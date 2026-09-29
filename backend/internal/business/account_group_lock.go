package business

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

const groupLockedAccountIDs = "group_locked_account_ids"

var ErrAccountGroupsLocked = errors.New("账号分组已锁定，请先关闭锁定分组开关再切换")

// SetAccountGroupsLocked serializes with all Console group writers. The flag is
// local policy and is deliberately not editable through generic policy patches.
func (s *Store) SetAccountGroupsLocked(ctx context.Context, accountID string, enabled bool, actor string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	ctx, release, err := mutationguard.Acquire(ctx, s, mutationguard.Account(accountID))
	if err != nil {
		return err
	}
	defer release()
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
	scope := map[string]any{}
	if raw, present := document["scope"]; present {
		var ok bool
		scope, ok = raw.(map[string]any)
		if !ok {
			return errors.New("控制面范围配置不可读，请检查配置后重试")
		}
	}
	if document == nil {
		return errors.New("控制面范围配置不可读，请检查配置后重试")
	}
	current := containsControlID(controlAccountIDs(scope[groupLockedAccountIDs]), accountID)
	if current == enabled {
		return nil
	}
	groups, err := pricingAccountChangeGroups(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if enabled && len(groups) == 0 {
		return errors.New("账号尚未绑定有效分组，请先同步目录并选择分组")
	}
	scope = copyObject(scope)
	setAccountScopeValue(scope, groupLockedAccountIDs, accountID, enabled)
	document["scope"] = scope
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", document, now); err != nil {
		return err
	}
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	field := "groups_locked"
	if err := insertAccountOperation(ctx, tx, AccountOperation{
		OperationID: fmt.Sprintf("account-group-lock/%s/%d", accountID, time.Now().UnixNano()), OperationType: "account.groups_lock", State: "succeeded", Phase: "local", Actor: actor,
		ObjectID: accountID, ObjectName: &name, GroupNames: names, FieldName: &field,
		Before: map[string]any{"groups_locked": current}, After: map[string]any{"groups_locked": enabled, "groups": groups},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) groupLockedAccounts(ctx context.Context, queryer policyQueryer) (map[string]struct{}, error) {
	document, err := s.readPolicyDocument(ctx, queryer, "control-plane")
	if err != nil {
		return nil, err
	}
	scope := map[string]any{}
	if raw, present := document["scope"]; present {
		var ok bool
		scope, ok = raw.(map[string]any)
		if !ok {
			return nil, errors.New("控制面范围配置不可读，无法确认分组锁定状态")
		}
	}
	return controlAccountIDs(scope[groupLockedAccountIDs]), nil
}

func (s *Store) requireAccountGroupsUnlocked(ctx context.Context, queryer policyQueryer, accountID string) error {
	locked, err := s.groupLockedAccounts(ctx, queryer)
	if err != nil {
		return err
	}
	if containsControlID(locked, accountID) {
		return ErrAccountGroupsLocked
	}
	return nil
}
