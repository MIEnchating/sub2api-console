package business

import (
	"context"
	"database/sql"
	"errors"
	"slices"
)

// CommitAccountGroups saves confirmed membership and its audit in one transaction.
func (s *Store) CommitAccountGroups(ctx context.Context, accountID string, before, after []string, operation AccountOperation) error {
	return s.commitAccountMutation(ctx, accountID, operation, func(tx *sql.Tx, now string) error {
		if err := s.requireAccountGroupsUnlocked(ctx, tx, accountID); err != nil {
			return err
		}
		var manual bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM manual_priority_accounts WHERE account_id=?)`, accountID).Scan(&manual); err != nil {
			return err
		}
		if manual {
			return errors.New("账号处于手动控制，请先取消手动控制再切换分组")
		}
		current, err := pricingAccountChangeGroups(ctx, tx, accountID)
		if err != nil {
			return err
		}
		currentIDs := make([]string, 0, len(current))
		for _, group := range current {
			currentIDs = append(currentIDs, group.ID)
		}
		expected := slices.Clone(before)
		slices.Sort(currentIDs)
		slices.Sort(expected)
		if !slices.Equal(currentIDs, expected) {
			return errors.New("账号分组已变化，请刷新后重试")
		}
		ids := uniqueStableIDs(after)
		if len(ids) == 0 || len(ids) != len(after) {
			return errors.New("请选择有效且不重复的目标分组")
		}
		names, err := pricingGroupNames(ctx, tx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, ok := names[id]; !ok {
				return errors.New("目标分组已不存在，请同步目录后重试")
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=?`, accountID); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
				SELECT id,?,?,multiplier FROM accounts WHERE id=?`, names[id], id, accountID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(SELECT COUNT(*) FROM account_groups WHERE group_id=local_groups.remote_id OR (group_id IS NULL AND group_name=local_groups.name)),updated_at=?`, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE bindings SET local_group=?,updated_at=? WHERE local_account_id=?`, names[ids[0]], now, accountID); err != nil {
			return err
		}
		// Decisions made for the old membership must not remain pending for this account.
		if _, err := tx.ExecContext(ctx, `DELETE FROM routing_decisions WHERE account_id=?`, accountID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE accounts SET updated_at=? WHERE id=?`, now, accountID)
		return err
	})
}
