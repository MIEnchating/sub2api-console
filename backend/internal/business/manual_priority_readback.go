package business

import (
	"context"
	"errors"
	"time"
)

// Keep the reserved slot and original baseline: an automatic reordering must
// never turn into a fresh manual assignment or recapture the release values.
func (s *Store) CommitManualPriorityReadback(ctx context.Context, accountID string, reserved, before, priority int64, operation AccountOperation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT priority FROM manual_priority_accounts WHERE account_id=?`, accountID).Scan(&current); err != nil {
		return err
	}
	if current != reserved {
		return errors.New("手动控制位置已变化，请重新计算优先级")
	}
	result, err := tx.ExecContext(ctx, `UPDATE accounts SET priority=?,updated_at=? WHERE id=? AND priority=?`, priority, time.Now().UTC().Format(time.RFC3339Nano), accountID, before)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return errors.New("账号优先级已变化，请同步后重试")
	}
	if err := insertAccountOperation(ctx, tx, operation); err != nil {
		return err
	}
	return tx.Commit()
}
