package taskstore

import (
	"context"
	"errors"
	"time"
)

func (s *Store) deleteServerHistory(ctx context.Context, skill string, selected []HistorySelection) error {
	ids := make([]string, len(selected))
	seen := map[string]bool{}
	for i, item := range selected {
		if item.ID == "" || seen[item.ID] {
			return errors.New("处理记录选择或版本无效，请刷新后重新选择")
		}
		if _, err := time.Parse(time.RFC3339Nano, item.UpdatedAt); err != nil {
			return errors.New("处理记录选择或版本无效，请刷新后重新选择")
		}
		ids[i] = item.ID
		seen[item.ID] = true
	}
	tx, err := s.beginTasksTx(ctx, ids)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range selected {
		var status, actual, owner string
		if err := tx.QueryRowContext(ctx, `SELECT status,updated_at,skill FROM tasks WHERE id=? FOR UPDATE`, item.ID).Scan(&status, &actual, &owner); err != nil {
			return err
		}
		version, err := time.Parse(time.RFC3339Nano, actual)
		expected, _ := time.Parse(time.RFC3339Nano, item.UpdatedAt)
		if err != nil || !version.Equal(expected) || activeTaskStatus(status) || owner != skill {
			return errors.New("处理记录已变化、仍在运行或不属于当前模块，请刷新后重新选择")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO deleted_tasks(id,deleted_at) VALUES(?,?)`, item.ID, time.Now().UTC().Format(storageTimeLayout)); err != nil {
			return err
		}
		for _, table := range []string{"task_recovery", "active_task_operations"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE task_id=?", item.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id=?`, item.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
