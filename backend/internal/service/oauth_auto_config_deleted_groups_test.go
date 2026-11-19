package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type deletedAutoConfigGroups struct {
	GroupRepository
	groups map[int64]*Group
	err    error
}

func (r deletedAutoConfigGroups) GetByID(_ context.Context, id int64) (*Group, error) {
	if r.err != nil {
		return nil, r.err
	}
	if g := r.groups[id]; g != nil {
		return g, nil
	}
	return nil, fmt.Errorf("group lookup: %w", ErrGroupNotFound)
}

func TestOAuthAutoConfigDeletedGroupRecovery(t *testing.T) {
	active := func(id int64) *Group { return &Group{ID: id, Platform: PlatformOpenAI, Status: StatusActive} }
	failure := errors.New("database temporarily unavailable")
	for _, tt := range []struct {
		name       string
		configured []int64
		explicit   []int64
		groups     map[int64]*Group
		lookupErr  error
		want       []int64
		errText    string
	}{
		{"legacy deleted references", []int64{5, 3, 8, 11}, []int64{99}, map[int64]*Group{8: active(8), 11: active(11)}, nil, []int64{8, 11}, ""},
		{"all deleted explicit selection", []int64{5, 3}, []int64{11}, map[int64]*Group{11: active(11)}, nil, []int64{11}, ""},
		{"cleaned empty policy explicit selection", []int64{}, []int64{11}, map[int64]*Group{11: active(11)}, nil, []int64{11}, ""},
		{"all deleted requires selection", []int64{5, 3}, nil, nil, nil, nil, "AUTO_CONFIG_GROUPS_REQUIRED"},
		{"cleaned empty policy requires selection", []int64{}, nil, nil, nil, nil, "AUTO_CONFIG_GROUPS_REQUIRED"},
		{"explicit deleted selection stays strict", []int64{5}, []int64{3, 11}, map[int64]*Group{11: active(11)}, nil, nil, "GROUP_NOT_FOUND"},
		{"disabled target is not deleted", []int64{8, 11}, nil, map[int64]*Group{8: {ID: 8, Platform: PlatformOpenAI, Status: StatusDisabled}, 11: active(11)}, nil, nil, "AUTO_CONFIG_GROUP_INVALID"},
		{"explicit cross platform rejected", []int64{5}, []int64{11}, map[int64]*Group{11: {ID: 11, Platform: PlatformAnthropic, Status: StatusActive}}, nil, nil, "AUTO_CONFIG_GROUP_INVALID"},
		{"database failure is not deletion", []int64{5}, []int64{11}, nil, failure, nil, failure.Error()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultOAuthAutoConfig()
			cfg.Enabled, cfg.GroupIDs, cfg.Revision = true, tt.configured, "policy-v1"
			cfg.Concurrency = 7
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			repo := &accountOpsSettingsStub{raw: string(raw)}
			svc := &adminServiceImpl{settingService: NewSettingService(repo, nil), groupRepo: deletedAutoConfigGroups{groups: tt.groups, err: tt.lookupErr}}
			in := &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: tt.explicit, Concurrency: 3}
			err = svc.ApplyOAuthAutoConfig(t.Context(), in)
			if tt.errText != "" {
				require.ErrorContains(t, err, tt.errText)
				require.Equal(t, tt.explicit, in.GroupIDs, "failed resolution must not overwrite explicit groups")
				require.Equal(t, 3, in.Concurrency)
				if tt.lookupErr != nil {
					require.ErrorIs(t, err, tt.lookupErr)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, in.GroupIDs)
			require.Equal(t, 7, in.Concurrency)
			require.Equal(t, "policy-v1", in.Extra["auto_config_initial_revision"])
		})
	}
}

func TestOAuthAutoConfigEmptyInitialTargetsRemainReadableButCannotBeSaved(t *testing.T) {
	cfg := DefaultOAuthAutoConfig()
	cfg.Enabled = true
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	loaded, err := GetOAuthAutoConfig(t.Context(), &accountOpsSettingsStub{raw: string(raw)})
	require.NoError(t, err)
	require.True(t, loaded.Enabled)
	require.Empty(t, loaded.GroupIDs)
	require.ErrorContains(t, ValidateOAuthAutoConfig(loaded), "select initial account groups")
}
