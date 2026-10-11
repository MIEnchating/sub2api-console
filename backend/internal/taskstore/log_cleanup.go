package taskstore

import (
	"context"
	"time"
)

const logCleanupBatchSize = 2000

type logCleanupCursor struct {
	updatedAt string
	id        string
}

func (s *Store) ClearLogs(ctx context.Context, before *time.Time) (int64, int64, error) {
	return s.ClearLogsWithProgress(ctx, before, nil)
}

func (s *Store) ClearLogsWithProgress(ctx context.Context, before *time.Time, report func(int64, int64) error) (int64, int64, error) {
	var total, protected int64
	var cursor *logCleanupCursor
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE status IN ('queued','running','waiting_input')").Scan(&protected); err != nil {
		return 0, 0, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return total, protected, err
		}
		count, next, err := s.clearLogBatch(ctx, before, cursor)
		if err != nil {
			return total, protected, err
		}
		total += count
		cursor = next
		if count > 0 && report != nil {
			if err := report(total, protected); err != nil {
				return total, protected, err
			}
		}
		if count < logCleanupBatchSize {
			return total, protected, nil
		}
	}
}

func (s *Store) clearLogBatch(ctx context.Context, before *time.Time, cursor *logCleanupCursor) (int64, *logCleanupCursor, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, cursor, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS log_cleanup_ids(id TEXT PRIMARY KEY,updated_at TEXT NOT NULL) WITHOUT ROWID"); err != nil {
		return 0, cursor, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM log_cleanup_ids"); err != nil {
		return 0, cursor, err
	}
	query := "INSERT INTO log_cleanup_ids(id,updated_at) SELECT id,updated_at FROM tasks WHERE status NOT IN ('queued','running','waiting_input')"
	args := []any{}
	if before != nil {
		// Older databases can contain variable precision and timezone offsets.
		// Keep an indexed coarse bound, then compare exact instants. The 48h
		// margin also covers boundary offsets accepted by Go's RFC3339 parser.
		upper := before.UTC().Add(48 * time.Hour)
		if upper.Year() >= 0 && upper.Year() <= 9999 {
			query += " AND updated_at < ?"
			args = append(args, upper.Format(time.RFC3339))
			query += " AND console_history_time(updated_at) < ?"
			args = append(args, before.UTC().Format(storageTimeLayout))
		} else {
			query += " AND console_history_before(updated_at,?,?)"
			args = append(args, before.Unix(), before.Nanosecond())
		}
	}
	if cursor != nil {
		query += " AND (updated_at,id) > (?,?)"
		args = append(args, cursor.updatedAt, cursor.id)
	}
	query += " ORDER BY updated_at,id LIMIT ?"
	args = append(args, logCleanupBatchSize)
	selected, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, cursor, err
	}
	count, err := selected.RowsAffected()
	if err != nil {
		return 0, cursor, err
	}
	if count == 0 {
		return 0, cursor, nil
	}
	next := cursor
	if before != nil && count == logCleanupBatchSize {
		// Resume after the last committed candidate, so retained legacy dates are
		// not parsed again in every batch. Backfilled earlier history waits for the next cleanup.
		next = &logCleanupCursor{}
		if err := tx.QueryRowContext(ctx, `SELECT updated_at,id FROM log_cleanup_ids
			ORDER BY updated_at DESC,id DESC LIMIT 1`).Scan(&next.updatedAt, &next.id); err != nil {
			return 0, cursor, err
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO deleted_tasks(id,deleted_at) SELECT id,? FROM log_cleanup_ids", time.Now().UTC().Format(storageTimeLayout)); err != nil {
		return 0, cursor, err
	}
	for _, table := range []string{"task_recovery", "active_task_operations"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE task_id IN (SELECT id FROM log_cleanup_ids)"); err != nil {
			return 0, cursor, err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE id IN (SELECT id FROM log_cleanup_ids)"); err != nil {
		return 0, cursor, err
	}
	if err := tx.Commit(); err != nil {
		return 0, cursor, err
	}
	return count, next, nil
}
