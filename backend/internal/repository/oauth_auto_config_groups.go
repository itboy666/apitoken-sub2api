package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

// removeOAuthAutoConfigGroup runs inside the group deletion transaction, after
// the group row is locked. It shares the writer's advisory and setting locks so
// deletion also serializes with the first save and concurrent group deletions.
func removeOAuthAutoConfigGroup(ctx context.Context, exec sqlExecutor, groupID int64) error {
	if _, err := exec.ExecContext(ctx, "SELECT pg_advisory_xact_lock(681705431)"); err != nil {
		return err
	}
	var raw string
	err := scanSingleRow(ctx, exec, "SELECT value FROM settings WHERE key=$1 FOR UPDATE", []any{service.SettingKeyOAuthAutoConfig}, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil || fields == nil {
		// Preserve a malformed legacy value; deletion must not replace it.
		return nil
	}
	changed := false
	for _, key := range []string{"group_ids", "upgrade_group_ids"} {
		var ids []int64
		if err := json.Unmarshal(fields[key], &ids); err != nil || !slices.Contains(ids, groupID) {
			continue
		}
		remaining := make([]int64, 0, len(ids))
		for _, id := range ids {
			if id != groupID {
				remaining = append(remaining, id)
			}
		}
		fields[key], _ = json.Marshal(remaining)
		if key == "upgrade_group_ids" && len(remaining) == 0 {
			fields["upgrade_enabled"] = json.RawMessage("false")
		}
		// Keep initial enabled=true even with no destinations. Import requires
		// an explicit valid group then; disabling would allow default binding.
		changed = true
	}
	if !changed {
		return nil
	}
	fields["revision"], _ = json.Marshal(uuid.NewString())
	fields["updated_at"], _ = json.Marshal(time.Now().UTC())
	value, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, "UPDATE settings SET value=$2,updated_at=NOW() WHERE key=$1", service.SettingKeyOAuthAutoConfig, string(value))
	return err
}
