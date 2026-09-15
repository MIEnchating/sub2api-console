package taskstore

import (
	"context"
	"errors"
	"strings"
	"time"
)

// HistorySelection pins the version shown in a destructive history confirmation.
type HistorySelection struct {
	ID        string `json:"id"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Store) DeleteTerminalBySkill(ctx context.Context, skill string, selected []HistorySelection) error {
	if strings.TrimSpace(skill) == "" || len(selected) == 0 || len(selected) > 500 {
		return errors.New("请选择 1～500 条已结束的处理记录")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	seen := make(map[string]bool, len(selected))
	for _, item := range selected {
		updated, err := time.Parse(time.RFC3339Nano, item.UpdatedAt)
		if err != nil || item.ID == "" || seen[item.ID] {
			return errors.New("处理记录选择或版本无效，请刷新后重新选择")
		}
		seen[item.ID] = true
		result, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id=? AND skill=? AND updated_at=? AND status IN ('succeeded','partial','failed','cancelled')`, item.ID, skill, updated.UTC().Format(storageTimeLayout))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return errors.New("处理记录已变化、仍在运行或不属于当前模块，请刷新后重新选择")
		}
		// Terminal tasks may still finish bounded cleanup such as SMS-order
		// finalization. Their later writes must not resurrect deleted history.
		if _, err := tx.ExecContext(ctx, `INSERT INTO deleted_tasks(id,deleted_at) VALUES(?,?)`, item.ID, time.Now().UTC().Format(storageTimeLayout)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
