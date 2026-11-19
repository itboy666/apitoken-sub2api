//go:build unit

package repository

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type capturedOAuthConfig struct{ raw string }

func (c *capturedOAuthConfig) Match(v driver.Value) bool {
	value, ok := v.(string)
	c.raw = value
	return ok
}

func TestDeletedOAuthAutoConfigGroupCleanup(t *testing.T) {
	for _, tt := range []struct {
		name    string
		raw     string
		changed bool
		initial []int64
		upgrade []int64
	}{
		{"both selections", `{"enabled":true,"group_ids":[8,3,11],"upgrade_enabled":true,"upgrade_group_ids":[3,8],"revision":"old","unknown":{"keep":true}}`, true, []int64{8, 11}, []int64{8}},
		{"last group", `{"enabled":true,"group_ids":[3],"upgrade_enabled":true,"upgrade_group_ids":[3],"revision":"old","unknown":{"keep":true}}`, true, []int64{}, []int64{}},
		{"unrelated group", `{"group_ids":[8],"upgrade_group_ids":[11],"revision":"old"}`, false, nil, nil},
		{"malformed legacy value", `{bad json`, false, nil, nil},
		{"malformed legacy selections", `{"group_ids":"broken"}`, false, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT value FROM settings.*FOR UPDATE").WithArgs(service.SettingKeyOAuthAutoConfig).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(tt.raw))
			captured := &capturedOAuthConfig{}
			if tt.changed {
				mock.ExpectExec("UPDATE settings SET value").WithArgs(service.SettingKeyOAuthAutoConfig, captured).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			require.NoError(t, removeOAuthAutoConfigGroup(t.Context(), db, 3))
			require.NoError(t, mock.ExpectationsWereMet())
			if tt.changed {
				var cfg service.OAuthAutoConfig
				require.NoError(t, json.Unmarshal([]byte(captured.raw), &cfg))
				require.Equal(t, tt.initial, cfg.GroupIDs)
				require.Equal(t, tt.upgrade, cfg.UpgradeGroupIDs)
				require.True(t, cfg.Enabled, "empty initial policy must not enable implicit default binding")
				require.Equal(t, len(tt.upgrade) > 0, cfg.UpgradeEnabled)
				require.NotEmpty(t, cfg.Revision)
				require.NotEqual(t, "old", cfg.Revision, "stale browser saves must conflict")
				require.False(t, cfg.UpdatedAt.IsZero())
				require.Contains(t, captured.raw, `"unknown":{"keep":true}`)
			}
		})
	}
}

func TestDeletedOAuthAutoConfigGroupCleanupPropagatesDatabaseFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	failure := errors.New("settings write unavailable")
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT value FROM settings.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"group_ids":[3]}`))
	mock.ExpectExec("UPDATE settings SET value").WillReturnError(failure)
	require.ErrorIs(t, removeOAuthAutoConfigGroup(t.Context(), db, 3), failure)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOAuthAutoConfigCASRejectsDeletedGroupAfterServiceValidation(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "upgrade"}[upgrade], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			cfg := service.DefaultOAuthAutoConfig()
			if upgrade {
				cfg.UpgradeEnabled, cfg.UpgradeGroupIDs = true, []int64{3}
			} else {
				cfg.Enabled, cfg.GroupIDs = true, []int64{3}
			}
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectQuery("account_group_live_group_lock.*FOR SHARE").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectRollback()
			err = (&settingRepository{client: client}).CompareAndSetOAuthAutoConfig(t.Context(), string(raw), "old")
			require.ErrorIs(t, err, service.ErrGroupNotFound)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestOAuthAutoConfigCASRejectsRevisionBeforeDeletion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	cfg := service.DefaultOAuthAutoConfig()
	cfg.Enabled, cfg.GroupIDs = true, []int64{8}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("account_group_live_group_lock.*FOR SHARE").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"revision":"after-deletion"}`))
	mock.ExpectRollback()
	err = (&settingRepository{client: client}).CompareAndSetOAuthAutoConfig(t.Context(), string(raw), "before-deletion")
	require.ErrorContains(t, err, "AUTO_CONFIG_CONFLICT")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupDeletionAndOAuthCleanupShareTransaction(t *testing.T) {
	for _, mode := range []string{"direct", "cascade", "guarded"} {
		for _, fail := range []bool{false, true} {
			name := mode + map[bool]string{false: "/commit", true: "/cleanup failure rolls back deletion"}[fail]
			t.Run(name, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
				repo := newGroupRepositoryWithSQL(client, db)
				mock.ExpectBegin()
				if mode == "direct" {
					mock.ExpectExec(`UPDATE "groups"`).WillReturnResult(sqlmock.NewResult(0, 1))
				} else {
					mock.ExpectQuery("SELECT id, subscription_type FROM groups.*FOR UPDATE").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"id", "subscription_type"}).AddRow(3, "standard"))
					if mode == "guarded" {
						mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
					}
				}
				// Policy must be locked before account_groups deletion: its trigger
				// locks accounts, while background workers take settings -> accounts.
				mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery("SELECT value FROM settings.*FOR UPDATE").WithArgs(service.SettingKeyOAuthAutoConfig).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"enabled":true,"group_ids":[3,8],"revision":"old"}`))
				write := mock.ExpectExec("UPDATE settings SET value")
				failure := errors.New("settings write unavailable")
				if fail {
					write.WillReturnError(failure)
					mock.ExpectRollback()
				} else {
					write.WillReturnResult(sqlmock.NewResult(0, 1))
					if mode != "direct" {
						mock.ExpectExec("DELETE FROM user_allowed_groups").WillReturnResult(sqlmock.NewResult(0, 0))
						mock.ExpectExec("DELETE FROM account_groups").WillReturnResult(sqlmock.NewResult(0, 0))
						mock.ExpectExec("UPDATE composite_model_routes").WillReturnResult(sqlmock.NewResult(0, 0))
						mock.ExpectExec("DELETE FROM api_key_groups").WillReturnResult(sqlmock.NewResult(0, 0))
						mock.ExpectExec(`UPDATE "groups"`).WillReturnResult(sqlmock.NewResult(0, 1))
						mock.ExpectQuery("SELECT value FROM settings WHERE key = .*FOR UPDATE").WithArgs(service.SettingKeyContentModerationConfig).WillReturnRows(sqlmock.NewRows([]string{"value"}))
					}
					mock.ExpectCommit()
					mock.ExpectExec("INSERT INTO scheduler_outbox").WillReturnResult(sqlmock.NewResult(0, 1))
				}
				switch mode {
				case "direct":
					err = repo.Delete(t.Context(), 3)
				case "cascade":
					_, err = repo.DeleteCascade(t.Context(), 3)
				default:
					_, err = repo.DeleteCascadeIfEmpty(t.Context(), 3)
				}
				if fail {
					require.ErrorIs(t, err, failure)
				} else {
					require.NoError(t, err)
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}
