package business

import (
	"context"
	"errors"
	"time"
)

func (s *Store) CommitManualCostWallReadback(ctx context.Context, accountID string, reserved int64, before, enabled bool, state string, operation AccountOperation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE accounts SET schedulable=?,routing_state=?,updated_at=? WHERE id=? AND schedulable=? AND EXISTS(SELECT 1 FROM manual_priority_accounts WHERE account_id=? AND priority=?)`, enabled, state, time.Now().UTC().Format(time.RFC3339Nano), accountID, before, accountID, reserved)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.New("手动控制或调度状态已变化，请同步后重新检查成本墙")
	}
	if err := insertAccountOperation(ctx, tx, operation); err != nil {
		return err
	}
	return tx.Commit()
}
