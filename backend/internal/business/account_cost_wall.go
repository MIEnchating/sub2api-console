package business

import (
	"context"
	"errors"
	"time"
)

// SetAccountIgnoreCostWall changes only the local cost boundary exemption.
// Remote scheduling, manual controls and health evidence remain untouched.
func (s *Store) SetAccountIgnoreCostWall(ctx context.Context, accountID string, enabled bool, actor string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM accounts WHERE id=?`, accountID).Scan(&name); err != nil {
		return err
	}
	document, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return err
	}
	if document == nil {
		return errors.New("控制面策略记录不存在")
	}
	scope, ok := document["scope"].(map[string]any)
	if !ok {
		return errors.New("策略字段 scope 必须是对象")
	}
	scope = copyObject(scope)
	setAccountScopeValue(scope, "ignore_cost_wall_account_ids", accountID, enabled)
	document["scope"] = scope
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", document, now); err != nil {
		return err
	}
	action := "respect_cost_wall"
	if enabled {
		action = "ignore_cost_wall"
		if err := closeAccountCostTrafficAlerts(ctx, tx, accountID, now); err != nil {
			return err
		}
	}
	if err := recordAccountControlEvent(ctx, tx, accountID, name, action, actor, now); err != nil {
		return err
	}
	return tx.Commit()
}
