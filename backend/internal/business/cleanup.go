package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type AccountDeleteScope struct {
	BindingID     int64
	UpstreamID    string
	UpstreamKeyID string
}

type accountDeleteScopeQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) MarkCleanupPaused(ctx context.Context, accountID, reason string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE accounts SET paused=1,paused_reason=?,routing_state='paused',updated_at=? WHERE id=?`, reason, now, accountID)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO paused_accounts(account_id,reason,enabled,updated_at) VALUES(?,?,1,?)
		ON CONFLICT(account_id) DO UPDATE SET reason=excluded.reason,enabled=1,updated_at=excluded.updated_at`, accountID, reason, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkCleanupDisabled(ctx context.Context, accountID string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id=?`, accountID).Scan(&raw); err != nil {
		return err
	}
	metadata, err := decodeJSONObject(raw)
	if err != nil {
		return errors.New("账号元数据损坏，无法记录自动停用")
	}
	metadata["status"] = "inactive"
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET schedulable=0,routing_state='disabled',metadata_json=?,updated_at=? WHERE id=?`, string(encoded), now, accountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteAccountProjection(ctx context.Context, accountID string, operation AccountOperation) error {
	return s.deleteAccountProjection(ctx, accountID, nil, operation)
}

func (s *Store) ConfirmAccountDeleteScope(ctx context.Context, accountID string, scope AccountDeleteScope) error {
	if err := validateAccountDeleteScope(accountID, scope); err != nil {
		return err
	}
	return confirmAccountDeleteScope(ctx, s.db, accountID, scope)
}

func (s *Store) ReconcileDeletedUpstreamKeyProjection(
	ctx context.Context,
	accountID string,
	scope AccountDeleteScope,
) error {
	if err := validateAccountDeleteScope(accountID, scope); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := confirmAccountDeleteScope(ctx, tx, accountID, scope); err != nil {
		return err
	}
	if err := deleteUpstreamKeyProjection(ctx, tx, scope); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteAccountProjectionWithScope(
	ctx context.Context,
	accountID string,
	scope AccountDeleteScope,
	operation AccountOperation,
) error {
	if err := validateAccountDeleteScope(accountID, scope); err != nil {
		return err
	}
	return s.deleteAccountProjection(ctx, accountID, &scope, operation)
}

func (s *Store) deleteAccountProjection(
	ctx context.Context,
	accountID string,
	scope *AccountDeleteScope,
	operation AccountOperation,
) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE id=?`, accountID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return sql.ErrNoRows
	}
	if scope != nil {
		if err := confirmAccountDeleteScope(ctx, tx, accountID, *scope); err != nil {
			return err
		}
		if err := deleteUpstreamKeyProjection(ctx, tx, *scope); err != nil {
			return err
		}
	}
	for _, table := range []string{"account_groups", "health_samples", "routing_decisions", "account_health_evaluations", "paused_accounts", "manual_priority_accounts", "routing_baselines", "cleanup_states"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE account_id=?", accountID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM bindings WHERE local_account_id=?`, accountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE id=?`, accountID); err != nil {
		return err
	}
	control, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return err
	}
	if control != nil {
		removeAccountPolicyReferences(control, accountID)
		if err := s.writePolicyDocument(ctx, tx, "control-plane", control, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if err := insertAccountOperation(ctx, tx, operation); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(
		SELECT COUNT(*) FROM account_groups WHERE group_name=local_groups.name
	),updated_at=?`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func deleteUpstreamKeyProjection(ctx context.Context, tx *sql.Tx, scope AccountDeleteScope) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_keys WHERE key_id=? AND host IN (
		SELECT host FROM upstream_identity_hosts WHERE upstream_id=?
	)`, scope.UpstreamKeyID, scope.UpstreamID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM upstream_catalog_entities
		WHERE upstream_id=? AND entity_kind='key' AND entity_id=?`, scope.UpstreamID, scope.UpstreamKeyID)
	return err
}

func validateAccountDeleteScope(accountID string, scope AccountDeleteScope) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	scope.UpstreamID = strings.TrimSpace(scope.UpstreamID)
	scope.UpstreamKeyID = strings.TrimSpace(scope.UpstreamKeyID)
	if scope.BindingID <= 0 || scope.UpstreamID == "" || scope.UpstreamKeyID == "" {
		return errors.New("账号删除范围缺少稳定 Binding ID、上游身份 ID 或 Key ID")
	}
	return nil
}

func confirmAccountDeleteScope(
	ctx context.Context,
	queryer accountDeleteScopeQueryer,
	accountID string,
	scope AccountDeleteScope,
) error {
	var exact int
	if err := queryer.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id
		WHERE b.id=? AND b.local_account_id=? AND bi.upstream_id=? AND bi.upstream_key_id=?
	)`, scope.BindingID, accountID, scope.UpstreamID, scope.UpstreamKeyID).Scan(&exact); err != nil {
		return err
	}
	if exact != 1 {
		return errors.New("账号绑定的稳定上游身份或 Key ID 已变化，请重新确认")
	}
	var shared int
	if err := queryer.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM binding_identities bi JOIN bindings b ON b.id=bi.binding_id
		WHERE bi.upstream_id=? AND bi.upstream_key_id=?
			AND (bi.binding_id<>? OR b.local_account_id<>?)
		UNION ALL
		SELECT 1 FROM onboarding_pending pending
		LEFT JOIN upstream_identity_hosts host ON host.host=pending.upstream_host
		WHERE pending.upstream_key_id=? AND (
			pending.upstream_id=? OR (TRIM(pending.upstream_id)='' AND host.upstream_id=?)
		)
	)`, scope.UpstreamID, scope.UpstreamKeyID, scope.BindingID, accountID,
		scope.UpstreamKeyID, scope.UpstreamID, scope.UpstreamID).Scan(&shared); err != nil {
		return err
	}
	if shared == 1 {
		return errors.New("上游 Key 仍被其他账号绑定或开户待续引用，拒绝删除共享 Key")
	}
	return nil
}

func removeAccountPolicyReferences(document map[string]any, accountID string) {
	if models, ok := document["account_test_models"].(map[string]any); ok {
		delete(models, accountID)
	}
	scope, ok := document["scope"].(map[string]any)
	if !ok {
		return
	}
	for _, field := range []string{"paused_account_ids", "excluded_account_ids", "manual_fused_account_ids"} {
		switch values := scope[field].(type) {
		case []any:
			filtered := make([]any, 0, len(values))
			for _, value := range values {
				if text, ok := value.(string); ok && strings.TrimSpace(text) == accountID {
					continue
				}
				filtered = append(filtered, value)
			}
			scope[field] = filtered
		case []string:
			filtered := make([]string, 0, len(values))
			for _, value := range values {
				if strings.TrimSpace(value) != accountID {
					filtered = append(filtered, value)
				}
			}
			scope[field] = filtered
		}
	}
}
