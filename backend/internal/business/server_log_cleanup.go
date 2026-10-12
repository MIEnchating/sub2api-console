package business

import (
	"context"
	"database/sql"
	"time"
)

type serverLogTable struct {
	table, key, stamp, extra string
	count                    *int64
}

type serverLogCursor struct {
	key     any
	stamp   string
	present bool
}

func (s *Store) clearServerLogRecords(ctx context.Context, before *time.Time, report func(LogCleanupResult) error) (LogCleanupResult, error) {
	result := LogCleanupResult{}
	tables := []serverLogTable{
		{"run_records", "run_key", "updated_at", ` AND COALESCE(status,'') NOT IN ('queued','running','waiting_input')`, &result.Runs},
		{"runtime_events", "source_id", "created_at", "", &result.Events},
		{"operation_audit", "source_id", "created_at", "", &result.Changes},
	}
	for _, table := range tables {
		cursor := serverLogCursor{}
		for {
			count, next, exhausted, err := s.clearServerLogBatch(ctx, table, before, cursor)
			if err != nil {
				return result, err
			}
			*table.count += count
			if count > 0 && report != nil {
				if err := report(result); err != nil {
					return result, err
				}
			}
			if exhausted {
				break
			}
			cursor = next
		}
	}
	return result, nil
}

// Lock and inspect only a bounded batch of IDs and timestamps. The cursor
// advances over invalid timestamps too; unknown ages are retained.
func (s *Store) clearServerLogBatch(ctx context.Context, table serverLogTable, before *time.Time, cursor serverLogCursor) (int64, serverLogCursor, bool, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, cursor, false, err
	}
	defer tx.Rollback()
	query := `SELECT ` + table.key + `,` + table.stamp + ` FROM ` + table.table + ` WHERE 1=1` + table.extra
	args := []any{}
	if before != nil {
		upper := before.UTC().Add(48 * time.Hour)
		if upper.Year() >= 0 && upper.Year() <= 9999 {
			query += ` AND ` + table.stamp + `<?`
			args = append(args, upper.Format(time.RFC3339))
		}
	}
	if cursor.present {
		query += ` AND (` + table.stamp + `>? OR (` + table.stamp + `=? AND ` + table.key + `>?))`
		args = append(args, cursor.stamp, cursor.stamp, cursor.key)
	}
	query += ` ORDER BY ` + table.stamp + `,` + table.key + ` LIMIT ? FOR UPDATE`
	args = append(args, logCleanupBatchSize)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, cursor, false, err
	}
	items := []serverLogCursor{}
	for rows.Next() {
		item := serverLogCursor{present: true}
		if err := rows.Scan(&item.key, &item.stamp); err != nil {
			rows.Close()
			return 0, cursor, false, err
		}
		if key, ok := item.key.([]byte); ok {
			item.key = string(key)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, cursor, false, err
	}
	if err := rows.Close(); err != nil {
		return 0, cursor, false, err
	}
	var deleted int64
	for _, item := range items {
		if before != nil {
			at, err := time.Parse(time.RFC3339Nano, item.stamp)
			if err != nil || !at.Before(*before) {
				continue
			}
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM `+table.table+` WHERE `+table.key+`=? AND `+table.stamp+`=?`+table.extra, item.key, item.stamp)
		if err != nil {
			return 0, cursor, false, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, cursor, false, err
		}
		deleted += count
	}
	if err := tx.Commit(); err != nil {
		return 0, cursor, false, err
	}
	if len(items) > 0 {
		cursor = items[len(items)-1]
	}
	return deleted, cursor, len(items) < logCleanupBatchSize, nil
}
