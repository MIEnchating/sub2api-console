package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) clearServerLogs(ctx context.Context, before *time.Time, report func(int64, int64) error) (int64, int64, error) {
	var total, protected int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE status IN ('queued','running','waiting_input')`).Scan(&protected); err != nil {
		return 0, 0, err
	}
	var cursor *logCleanupCursor
	for {
		query := `SELECT id,updated_at FROM tasks WHERE status NOT IN ('queued','running','waiting_input')`
		args := []any{}
		if cursor != nil {
			query += ` AND (updated_at,id)>(?,?)`
			args = append(args, cursor.updatedAt, cursor.id)
		}
		if before != nil {
			upper := before.UTC().Add(48 * time.Hour)
			if upper.Year() >= 0 && upper.Year() <= 9999 {
				query += ` AND updated_at<?`
				args = append(args, upper.Format(time.RFC3339))
			}
		}
		query += ` ORDER BY updated_at,id LIMIT ?`
		args = append(args, logCleanupBatchSize)
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return total, protected, err
		}
		candidates := make([]logCleanupCursor, 0, logCleanupBatchSize)
		for rows.Next() {
			var item logCleanupCursor
			if err := rows.Scan(&item.id, &item.updatedAt); err != nil {
				rows.Close()
				return total, protected, err
			}
			candidates = append(candidates, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return total, protected, err
		}
		ids := make([]string, 0, len(candidates))
		for _, item := range candidates {
			if before != nil {
				stamp, err := time.Parse(time.RFC3339Nano, item.updatedAt)
				if err != nil || !stamp.Before(*before) {
					continue
				}
			}
			ids = append(ids, item.id)
		}
		changed, err := s.deleteServerLogCandidates(ctx, ids, before)
		total += changed
		if err != nil {
			return total, protected, err
		}
		if changed > 0 && report != nil {
			if err := report(total, protected); err != nil {
				return total, protected, err
			}
		}
		if len(candidates) < logCleanupBatchSize {
			return total, protected, nil
		}
		last := candidates[len(candidates)-1]
		cursor = &last
	}
}

func (s *Store) deleteServerLogCandidates(ctx context.Context, ids []string, before *time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := s.beginTasksTx(ctx, ids)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var changed int64
	for _, id := range ids {
		var status, updated string
		err := tx.QueryRowContext(ctx, `SELECT status,updated_at FROM tasks WHERE id=? FOR UPDATE`, id).Scan(&status, &updated)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if activeTaskStatus(status) {
			continue
		}
		if before != nil {
			stamp, err := time.Parse(time.RFC3339Nano, updated)
			if err != nil || !stamp.Before(*before) {
				continue
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO deleted_tasks(id,deleted_at) VALUES(?,?)`, id, time.Now().UTC().Format(storageTimeLayout)); err != nil {
			return 0, err
		}
		for _, table := range []string{"task_recovery", "active_task_operations"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE task_id=?", id); err != nil {
				return 0, err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id=?`, id); err != nil {
			return 0, err
		}
		changed++
	}
	return changed, tx.Commit()
}
