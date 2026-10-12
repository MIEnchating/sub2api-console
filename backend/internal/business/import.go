package business

import (
	"context"
	"database/sql"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// RebuildImportedState derives server-only indexes and counters inside the
// offline import transaction, before the target may accept application writes.
func RebuildImportedState(ctx context.Context, tx *dbconn.Tx) error {
	for _, table := range []string{"runtime_events", "operation_audit"} {
		if _, err := tx.ExecContext(ctx, `UPDATE business_event_counters SET value=(SELECT COALESCE(MIN(source_id),0) FROM `+table+` WHERE source_id<0) WHERE name=?`, table); err != nil {
			return err
		}
	}
	var cursor int64
	first := true
	for {
		query := `SELECT id,observed_at FROM usage_records`
		args := []any{}
		if !first {
			query += ` WHERE id>?`
			args = append(args, cursor)
		}
		rows, err := tx.QueryContext(ctx, query+` ORDER BY id LIMIT 200`, args...)
		if err != nil {
			return err
		}
		type entry struct {
			id       int64
			observed sql.NullString
		}
		entries := []entry{}
		for rows.Next() {
			var value entry
			if err := rows.Scan(&value.id, &value.observed); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, value)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, value := range entries {
			var key []byte
			if stamp, err := time.Parse(time.RFC3339Nano, value.observed.String); value.observed.Valid && err == nil {
				key = routingRecoveryTimeKey(stamp)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE usage_records SET observed_key=? WHERE id=?`, key, value.id); err != nil {
				return err
			}
		}
		if len(entries) < 200 {
			return nil
		}
		cursor = entries[len(entries)-1].id
		first = false
	}
}
