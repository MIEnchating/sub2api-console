package business

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// Only local event namespaces allocate negative IDs. Source imports use their
// original positive IDs. The counter update and event insert commit together.
func nextLocalEventID(ctx context.Context, tx *dbconn.Tx, table string) (int64, error) {
	if table != "runtime_events" && table != "operation_audit" {
		return 0, errors.New("无效的本地事件命名空间")
	}
	if tx.Driver() == dbconn.SQLite {
		var minimum sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM `+table+` WHERE source_id<0`).Scan(&minimum); err != nil {
			return 0, err
		}
		if minimum.Valid {
			if minimum.Int64 == math.MinInt64 {
				return 0, errors.New("本地事件 ID 已耗尽")
			}
			return minimum.Int64 - 1, nil
		}
		return -1, nil
	}
	// Counters are initialized with the schema and reconciled during imports.
	// Reading the event table in INSERT SELECT would acquire shared index locks
	// in MySQL and deadlock with other writers waiting on the same counter.
	if _, err := tx.ExecContext(ctx, `UPDATE business_event_counters SET value=value-1 WHERE name=?`, table); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT value FROM business_event_counters WHERE name=? FOR UPDATE`, table).Scan(&id)
	return id, err
}

// Server counters stay locked until commit, so a writer receives its own last
// ID even when another transaction has inserted a different event concurrently.
func lastLocalEventIDSQL(tx *dbconn.Tx, table string) string {
	if tx.Driver() == dbconn.SQLite {
		return `SELECT MIN(source_id) FROM ` + table + ` WHERE source_id < 0`
	}
	return `SELECT value FROM business_event_counters WHERE name='` + table + `' FOR UPDATE`
}
