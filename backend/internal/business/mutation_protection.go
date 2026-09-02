package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

type AccountMutationProtection struct {
	ManualPriority bool
	Paused         bool
	Excluded       bool
	ManualFused    bool
}

func (protection AccountMutationProtection) Protected() bool {
	return protection.ManualPriority || protection.Paused || protection.Excluded || protection.ManualFused
}

func (protection AccountMutationProtection) Reasons() []string {
	reasons := []string{}
	if protection.ManualPriority {
		reasons = append(reasons, "人工优先位")
	}
	if protection.Paused {
		reasons = append(reasons, "人工暂停")
	}
	if protection.Excluded {
		reasons = append(reasons, "人工排除")
	}
	if protection.ManualFused {
		reasons = append(reasons, "人工熔断")
	}
	return reasons
}

func (s *Store) AccountMutationProtection(ctx context.Context, accountID string) (AccountMutationProtection, error) {
	protections, err := s.AccountMutationProtections(ctx, []string{accountID})
	if err != nil {
		return AccountMutationProtection{}, err
	}
	return protections[accountID], nil
}

func (s *Store) AccountMutationProtections(ctx context.Context, accountIDs []string) (map[string]AccountMutationProtection, error) {
	requested := make(map[string]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		accountID = strings.TrimSpace(accountID)
		if !positiveNumericID(accountID) {
			return nil, errors.New("账号必须使用有效的稳定 ID")
		}
		requested[accountID] = struct{}{}
	}
	result := make(map[string]AccountMutationProtection, len(requested))
	if len(requested) == 0 {
		return result, nil
	}
	ordered := make([]string, 0, len(requested))
	for accountID := range requested {
		ordered = append(ordered, accountID)
	}
	sort.Strings(ordered)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	document, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, errors.New("控制面策略记录不存在")
	}
	scope := map[string]any{}
	if raw, present := document["scope"]; present {
		var ok bool
		scope, ok = raw.(map[string]any)
		if !ok {
			return nil, errors.New("策略字段 scope 必须是对象")
		}
	}
	paused := controlAccountIDs(scope["paused_account_ids"])
	excluded := controlAccountIDs(scope["excluded_account_ids"])
	fused := controlAccountIDs(scope["manual_fused_account_ids"])
	for _, accountID := range ordered {
		result[accountID] = AccountMutationProtection{
			Paused:      containsControlID(paused, accountID),
			Excluded:    containsControlID(excluded, accountID),
			ManualFused: containsControlID(fused, accountID),
		}
	}
	encodedAccountIDs, err := json.Marshal(ordered)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT account_id FROM manual_priority_accounts
		WHERE account_id IN (SELECT CAST(value AS TEXT) FROM json_each(?))`, string(encodedAccountIDs))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var accountID string
		if err := rows.Scan(&accountID); err != nil {
			rows.Close()
			return nil, err
		}
		protection := result[accountID]
		protection.ManualPriority = true
		result[accountID] = protection
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func containsControlID(values map[string]struct{}, accountID string) bool {
	_, found := values[accountID]
	return found
}
