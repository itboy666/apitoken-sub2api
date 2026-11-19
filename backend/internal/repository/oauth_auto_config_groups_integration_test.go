//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func deletedGroupConfigFixture(t *testing.T, ids []int64) service.OAuthAutoConfig {
	t.Helper()
	ctx := context.Background()
	var previous string
	previousErr := integrationDB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=$1", service.SettingKeyOAuthAutoConfig).Scan(&previous)
	if previousErr != nil {
		require.ErrorIs(t, previousErr, sql.ErrNoRows)
	}
	t.Cleanup(func() {
		if previousErr == nil {
			_, err := integrationDB.ExecContext(ctx, "UPDATE settings SET value=$2 WHERE key=$1", service.SettingKeyOAuthAutoConfig, previous)
			require.NoError(t, err)
		} else {
			_, err := integrationDB.ExecContext(ctx, "DELETE FROM settings WHERE key=$1", service.SettingKeyOAuthAutoConfig)
			require.NoError(t, err)
		}
	})
	cfg := service.DefaultOAuthAutoConfig()
	cfg.Enabled, cfg.UpgradeEnabled = true, true
	cfg.GroupIDs, cfg.UpgradeGroupIDs = ids, ids
	cfg.Revision = "before-deletion"
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES($1,$2) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value", service.SettingKeyOAuthAutoConfig, string(raw))
	require.NoError(t, err)
	return cfg
}

func deletedGroupFixture(t *testing.T) *service.Group {
	t.Helper()
	g := mustCreateGroup(t, integrationEntClient, &service.Group{Name: fmt.Sprintf("deleted-config-%d", time.Now().UnixNano()), Platform: service.PlatformOpenAI})
	t.Cleanup(func() {
		_, err := integrationDB.Exec("DELETE FROM scheduler_outbox WHERE group_id=$1", g.ID)
		require.NoError(t, err)
		_, err = integrationDB.Exec("DELETE FROM groups WHERE id=$1", g.ID)
		require.NoError(t, err)
	})
	return g
}

func TestOAuthAutoConfigGroupDeletionPostgres(t *testing.T) {
	for _, mode := range []string{"direct", "cascade", "guarded"} {
		t.Run(mode, func(t *testing.T) {
			g, kept := deletedGroupFixture(t), deletedGroupFixture(t)
			deletedGroupConfigFixture(t, []int64{g.ID, kept.ID})
			repo := newGroupRepositoryWithSQL(integrationEntClient, integrationDB)
			remove := func(id int64) error {
				if mode == "direct" {
					return repo.Delete(t.Context(), id)
				}
				if mode == "guarded" {
					_, err := repo.DeleteCascadeIfEmpty(t.Context(), id)
					return err
				}
				_, err := repo.DeleteCascade(t.Context(), id)
				return err
			}
			require.NoError(t, remove(g.ID))
			cfg, err := service.GetOAuthAutoConfig(t.Context(), NewSettingRepository(integrationEntClient))
			require.NoError(t, err)
			require.Equal(t, []int64{kept.ID}, cfg.GroupIDs)
			require.Equal(t, []int64{kept.ID}, cfg.UpgradeGroupIDs)
			require.NotEqual(t, "before-deletion", cfg.Revision)
			require.NoError(t, remove(kept.ID))
			cfg, err = service.GetOAuthAutoConfig(t.Context(), NewSettingRepository(integrationEntClient))
			require.NoError(t, err)
			require.True(t, cfg.Enabled)
			require.Empty(t, cfg.GroupIDs)
			require.False(t, cfg.UpgradeEnabled)
		})
	}
}

func TestOAuthAutoConfigDeletionDoesNotDeadlockWorkerPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	g := deletedGroupFixture(t)
	deletedGroupConfigFixture(t, []int64{g.ID})
	a := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "deletion-worker-lock", Platform: service.PlatformOpenAI})
	t.Cleanup(func() {
		_, err := integrationDB.Exec("DELETE FROM accounts WHERE id=$1", a.ID)
		require.NoError(t, err)
	})
	_, err := integrationDB.ExecContext(ctx, "INSERT INTO account_groups(account_id,group_id,priority) VALUES($1,$2,50)", a.ID, g.ID)
	require.NoError(t, err)
	worker, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer worker.Rollback()
	var workerPID int
	require.NoError(t, worker.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&workerPID))
	var raw string
	require.NoError(t, worker.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=$1 FOR SHARE", service.SettingKeyOAuthAutoConfig).Scan(&raw))
	deleted := make(chan error, 1)
	go func() {
		_, err := newGroupRepositoryWithSQL(integrationEntClient, integrationDB).DeleteCascade(ctx, g.ID)
		deleted <- err
	}()
	require.Eventually(t, func() bool {
		var blocked bool
		err := integrationDB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", workerPID).Scan(&blocked)
		return err == nil && blocked
	}, 3*time.Second, 10*time.Millisecond)
	// Before the fix, deletion already held this account lock through the
	// membership trigger, producing the settings <-> account deadlock here.
	_, err = worker.ExecContext(ctx, "UPDATE accounts SET concurrency=4 WHERE id=$1", a.ID)
	require.NoError(t, err)
	require.NoError(t, worker.Commit())
	require.NoError(t, <-deleted)
	var concurrency int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT concurrency FROM accounts WHERE id=$1", a.ID).Scan(&concurrency))
	require.Equal(t, 4, concurrency)
}

func TestOAuthAutoConfigConcurrentDeleteRejectsSavePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	g := deletedGroupFixture(t)
	cfg := deletedGroupConfigFixture(t, []int64{g.ID})
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	defer tx.Rollback()
	var locked int64
	var deleterPID int
	require.NoError(t, scanSingleRow(ctx, tx.Client(), "SELECT pg_backend_pid()", nil, &deleterPID))
	require.NoError(t, scanSingleRow(ctx, tx.Client(), "SELECT id FROM groups WHERE id=$1 FOR UPDATE", []any{g.ID}, &locked))
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	saved := make(chan error, 1)
	go func() {
		saved <- (&settingRepository{client: integrationEntClient}).CompareAndSetOAuthAutoConfig(ctx, string(raw), cfg.Revision)
	}()
	require.Eventually(t, func() bool {
		var blocked bool
		err := integrationDB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", deleterPID).Scan(&blocked)
		return err == nil && blocked
	}, 3*time.Second, 10*time.Millisecond)
	_, err = newGroupRepositoryWithSQL(tx.Client(), tx).DeleteCascade(ctx, g.ID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.ErrorIs(t, <-saved, service.ErrGroupNotFound)
	loaded, err := service.GetOAuthAutoConfig(ctx, NewSettingRepository(integrationEntClient))
	require.NoError(t, err)
	require.Empty(t, loaded.GroupIDs)
	require.NotEqual(t, cfg.Revision, loaded.Revision)
}
