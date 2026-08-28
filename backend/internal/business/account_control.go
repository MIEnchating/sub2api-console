package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var accountControlFields = map[string]string{
	"pause": "paused_account_ids", "resume": "paused_account_ids",
	"exclude": "excluded_account_ids", "include": "excluded_account_ids",
	"fuse": "manual_fused_account_ids", "recover": "manual_fused_account_ids",
}

func (s *Store) SetAccountControl(ctx context.Context, accountID, action, actor string) (PolicySnapshot, error) {
	if !positiveNumericID(accountID) {
		return PolicySnapshot{}, errors.New("账号必须使用有效的稳定 ID")
	}
	field, valid := accountControlFields[action]
	if !valid {
		return PolicySnapshot{}, errors.New("账号控制 action 无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PolicySnapshot{}, err
	}
	defer tx.Rollback()
	var name string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM accounts WHERE id=?`, accountID).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PolicySnapshot{}, errors.New("账号不存在")
		}
		return PolicySnapshot{}, err
	}
	document, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return PolicySnapshot{}, err
	}
	if document == nil {
		return PolicySnapshot{}, errors.New("控制面策略记录不存在")
	}
	scope := map[string]any{}
	if raw, present := document["scope"]; present {
		var ok bool
		scope, ok = raw.(map[string]any)
		if !ok {
			return PolicySnapshot{}, errors.New("策略字段 scope 必须是对象")
		}
		scope = copyObject(scope)
	}
	values := controlAccountIDs(scope[field])
	enabled := action == "pause" || action == "exclude" || action == "fuse"
	if enabled {
		values[accountID] = struct{}{}
	} else {
		delete(values, accountID)
	}
	scope[field] = sortedControlAccountIDs(values)
	document["scope"] = scope
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", document, now); err != nil {
		return PolicySnapshot{}, err
	}
	if action == "pause" {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET paused=1,paused_reason='人工暂停',updated_at=? WHERE id=?`, now, accountID); err != nil {
			return PolicySnapshot{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO paused_accounts(account_id,reason,enabled,updated_at) VALUES(?,'人工暂停',1,?)
			ON CONFLICT(account_id) DO UPDATE SET reason='人工暂停',enabled=1,updated_at=excluded.updated_at`, accountID, now); err != nil {
			return PolicySnapshot{}, err
		}
	}
	if action == "resume" {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET paused=0,paused_reason=NULL,updated_at=? WHERE id=?`, now, accountID); err != nil {
			return PolicySnapshot{}, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM paused_accounts WHERE account_id=?`, accountID); err != nil {
			return PolicySnapshot{}, err
		}
	}
	if action == "recover" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM routing_decisions WHERE account_id=?`, accountID); err != nil {
			return PolicySnapshot{}, err
		}
	}
	if err := recordAccountControlEvent(ctx, tx, accountID, name, action, actor, now); err != nil {
		return PolicySnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return PolicySnapshot{}, err
	}
	return s.PolicySnapshot(ctx)
}

func (s *Store) SetAccountTestModel(ctx context.Context, accountID string, model *string, actor string) error {
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
	models := map[string]any{}
	if raw, present := document["account_test_models"]; present {
		var ok bool
		models, ok = raw.(map[string]any)
		if !ok {
			return errors.New("账号探测模型配置无效")
		}
		models = copyObject(models)
	}
	if model == nil || strings.TrimSpace(*model) == "" {
		delete(models, accountID)
	} else {
		value := strings.TrimSpace(*model)
		if len(value) > 256 {
			return errors.New("探测模型长度不能超过 256")
		}
		models[accountID] = value
	}
	document["account_test_models"] = models
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", document, now); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"account_id": accountID, "account_name": name, "model": model, "actor": actor})
	if err != nil {
		return err
	}
	var minimum sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&minimum); err != nil {
		return err
	}
	sourceID := int64(-1)
	if minimum.Valid && minimum.Int64 <= -1 {
		sourceID = minimum.Int64 - 1
	}
	summary := fmt.Sprintf("账号 %s（%s）探测模型已更新", name, accountID)
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json)
		VALUES(?,?,?,?,?,?)`, sourceID, "account.test_model", now, "succeeded", summary, string(payload)); err != nil {
		return err
	}
	return tx.Commit()
}

func controlAccountIDs(raw any) map[string]struct{} {
	result := map[string]struct{}{}
	values, _ := raw.([]any)
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if positiveNumericID(text) {
			result[text] = struct{}{}
		}
	}
	return result
}

func sortedControlAccountIDs(values map[string]struct{}) []any {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	result := make([]any, len(items))
	for index := range items {
		result[index] = items[index]
	}
	return result
}

func recordAccountControlEvent(ctx context.Context, tx *sql.Tx, accountID, name, action, actor, now string) error {
	payload, err := json.Marshal(map[string]any{"account_id": accountID, "account_name": name, "action": action, "actor": actor})
	if err != nil {
		return err
	}
	var minimum sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&minimum); err != nil {
		return err
	}
	sourceID := int64(-1)
	if minimum.Valid && minimum.Int64 <= -1 {
		sourceID = minimum.Int64 - 1
	}
	labels := map[string]string{"pause": "暂停", "resume": "恢复调度", "exclude": "排除", "include": "恢复管控", "fuse": "人工熔断", "recover": "解除熔断"}
	summary := fmt.Sprintf("账号 %s（%s）已执行%s", name, accountID, labels[action])
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json)
		VALUES(?,?,?,?,?,?)`, sourceID, "account.control", now, "succeeded", summary, string(payload))
	return err
}
