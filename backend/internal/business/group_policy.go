package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var ErrGroupNotFound = errors.New("分组不存在")

var groupPolicyFields = map[string]struct{}{
	"enabled": {}, "strategy": {}, "min_pool_size": {}, "weight_budget": {}, "balanced_price_ratio": {},
	"breaker_enabled": {}, "recovery_enabled": {}, "weights_enabled": {}, "scaling_enabled": {},
	"probe_enabled": {}, "probe_interval_seconds": {}, "probe_model": {},
}

func (s *Store) UpdateGroupPolicy(ctx context.Context, groupID string, raw map[string]any, actor string) (GroupStatus, error) {
	if !stableNumericID(groupID) {
		return GroupStatus{}, fmt.Errorf("分组必须使用已登记的稳定数字 ID")
	}
	normalizedValue, err := normalizeJSONNumbers(raw)
	if err != nil {
		return GroupStatus{}, err
	}
	payload := normalizedValue.(map[string]any)
	for field := range payload {
		if _, allowed := groupPolicyFields[field]; !allowed {
			return GroupStatus{}, fmt.Errorf("分组策略包含未知字段：%s", field)
		}
	}
	missing := []string{}
	for field := range groupPolicyFields {
		if _, present := payload[field]; !present {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return GroupStatus{}, fmt.Errorf("分组策略缺少字段：%s", strings.Join(missing, ","))
	}
	binding, err := normalizeGroupPolicyBinding(payload)
	if err != nil {
		return GroupStatus{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GroupStatus{}, err
	}
	defer tx.Rollback()
	groupName, err := registeredGroup(ctx, tx, groupID)
	if err != nil {
		return GroupStatus{}, err
	}
	control, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return GroupStatus{}, err
	}
	if control == nil {
		return GroupStatus{}, fmt.Errorf("控制面策略不可用，无法保存分组策略")
	}
	bindings, ok := control["group_policy_bindings"].(map[string]any)
	if !ok && control["group_policy_bindings"] != nil {
		return GroupStatus{}, fmt.Errorf("策略字段 group_policy_bindings 必须是对象")
	}
	bindings = copyObject(bindings)
	bindings[groupID] = binding
	control["group_policy_bindings"] = bindings
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", control, now); err != nil {
		return GroupStatus{}, err
	}
	strategy := binding["strategy"].(string)
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET strategy=?,strategy_source='group_override',updated_at=? WHERE remote_id=?`, strategy, now, groupID); err != nil {
		return GroupStatus{}, err
	}
	if err := insertRuntimeEvent(ctx, tx, "group.policy.updated", "分组策略已更新："+groupName, map[string]any{"actor": strings.TrimSpace(actor), "group_id": groupID}, now); err != nil {
		return GroupStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return GroupStatus{}, err
	}
	return s.groupByID(ctx, groupID)
}

func (s *Store) ClearGroupPolicy(ctx context.Context, groupID string, actor string) (GroupStatus, error) {
	if !stableNumericID(groupID) {
		return GroupStatus{}, fmt.Errorf("分组必须使用已登记的稳定数字 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GroupStatus{}, err
	}
	defer tx.Rollback()
	groupName, err := registeredGroup(ctx, tx, groupID)
	if err != nil {
		return GroupStatus{}, err
	}
	control, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return GroupStatus{}, err
	}
	if control == nil {
		return GroupStatus{}, fmt.Errorf("控制面策略不可用，无法清除分组策略")
	}
	bindings, ok := control["group_policy_bindings"].(map[string]any)
	if !ok && control["group_policy_bindings"] != nil {
		return GroupStatus{}, fmt.Errorf("策略字段 group_policy_bindings 必须是对象")
	}
	bindings = copyObject(bindings)
	delete(bindings, groupID)
	control["group_policy_bindings"] = bindings
	strategy, err := effectiveGlobalStrategy(control)
	if err != nil {
		return GroupStatus{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", control, now); err != nil {
		return GroupStatus{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET strategy=?,strategy_source='global_default',updated_at=? WHERE remote_id=?`, strategy, now, groupID); err != nil {
		return GroupStatus{}, err
	}
	if err := insertRuntimeEvent(ctx, tx, "group.policy.cleared", "分组策略已回落到全局："+groupName, map[string]any{
		"actor": strings.TrimSpace(actor), "group_id": groupID,
	}, now); err != nil {
		return GroupStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return GroupStatus{}, err
	}
	return s.groupByID(ctx, groupID)
}

func effectiveGlobalStrategy(control map[string]any) (string, error) {
	selection, ok := control["selection"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("策略字段 selection 必须是对象")
	}
	strategy, err := normalizeStrategy(selection["strategy"])
	if err != nil {
		return "", fmt.Errorf("全局策略配置无效")
	}
	return strategy, nil
}

func (s *Store) SetGroupExcluded(ctx context.Context, groupID string, excluded bool, actor string) (GroupStatus, error) {
	if !stableNumericID(groupID) {
		return GroupStatus{}, fmt.Errorf("分组必须使用已登记的稳定数字 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GroupStatus{}, err
	}
	defer tx.Rollback()
	groupName, err := registeredGroup(ctx, tx, groupID)
	if err != nil {
		return GroupStatus{}, err
	}
	control, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return GroupStatus{}, err
	}
	if control == nil {
		return GroupStatus{}, fmt.Errorf("控制面策略不可用，无法修改分组管控范围")
	}
	scope, ok := control["scope"].(map[string]any)
	if !ok {
		return GroupStatus{}, fmt.Errorf("策略字段 scope 必须是对象")
	}
	scope = copyObject(scope)
	excludedIDs, err := normalizedStringArray("scope.excluded_group_ids", defaultArray(scope["excluded_group_ids"]))
	if err != nil {
		return GroupStatus{}, err
	}
	if excluded {
		excludedIDs = appendUniqueFold(excludedIDs, groupID)
	} else {
		excludedIDs = removeTextFold(excludedIDs, groupID)
	}
	scope["excluded_group_ids"] = excludedIDs
	control["scope"] = scope
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", control, now); err != nil {
		return GroupStatus{}, err
	}
	action := "排除"
	if !excluded {
		action = "恢复管控"
	}
	if err := insertRuntimeEvent(ctx, tx, "group.scope.updated", "分组已"+action+"："+groupName, map[string]any{
		"actor": strings.TrimSpace(actor), "group_id": groupID, "excluded": excluded,
	}, now); err != nil {
		return GroupStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return GroupStatus{}, err
	}
	return s.groupByID(ctx, groupID)
}

func normalizeGroupPolicyBinding(payload map[string]any) (map[string]any, error) {
	result := map[string]any{}
	for _, field := range []string{"enabled", "breaker_enabled", "recovery_enabled", "weights_enabled", "scaling_enabled", "probe_enabled"} {
		value, ok := payload[field].(bool)
		if !ok {
			return nil, fmt.Errorf("分组策略字段 %s 必须是布尔值", field)
		}
		result[field] = value
	}
	strategy, err := normalizeStrategy(payload["strategy"])
	if err != nil {
		return nil, fmt.Errorf("分组策略字段 strategy 无效")
	}
	result["strategy"] = strategy
	for field, bounds := range map[string][2]int{
		"min_pool_size": {0, 10000}, "weight_budget": {1, 1_000_000}, "probe_interval_seconds": {30, 86400},
	} {
		value, err := boundedInteger(field, payload[field], bounds[0], bounds[1])
		if err != nil {
			return nil, err
		}
		result[field] = int64(value)
	}
	ratio, ok := finiteNumber(payload["balanced_price_ratio"])
	if !ok || ratio < 0 || ratio > 1 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return nil, fmt.Errorf("分组策略字段 balanced_price_ratio 必须在 0 到 1 之间")
	}
	result["balanced_price_ratio"] = ratio
	model := payload["probe_model"]
	if model != nil {
		text, ok := model.(string)
		if !ok || len(text) > 256 {
			return nil, fmt.Errorf("分组策略字段 probe_model 必须是长度不超过 256 的字符串或 null")
		}
		text = strings.TrimSpace(text)
		if text == "" {
			model = nil
		} else {
			model = text
		}
	}
	result["probe_model"] = model
	return result, nil
}

func registeredGroup(ctx context.Context, tx *sql.Tx, groupID string) (string, error) {
	var name string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM local_groups WHERE remote_id=?`, groupID).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w：分组 ID %s", ErrGroupNotFound, groupID)
		}
		return "", err
	}
	return name, nil
}

func insertRuntimeEvent(ctx context.Context, tx *sql.Tx, eventType, summary string, payload map[string]any, now string) error {
	return insertRuntimeEventWithStatus(ctx, tx, eventType, "succeeded", summary, payload, now)
}

func (s *Store) RecordRuntimeEvent(ctx context.Context, eventType, status, summary string, payload map[string]any) (int64, error) {
	if strings.TrimSpace(eventType) == "" || strings.TrimSpace(status) == "" || strings.TrimSpace(summary) == "" {
		return 0, errors.New("运行事件类型、状态和摘要不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertRuntimeEventWithStatus(ctx, tx, eventType, status, summary, payload, now); err != nil {
		return 0, err
	}
	var eventID int64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&eventID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return eventID, nil
}

func insertRuntimeEventWithStatus(ctx context.Context, tx *sql.Tx, eventType, status, summary string, payload map[string]any, now string) error {
	encoded, err := json.Marshal(payload)
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
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json)
		VALUES(?,?,?,?,?,?)`, sourceID, eventType, now, status, summary, string(encoded))
	return err
}

func defaultArray(value any) any {
	if value == nil {
		return []any{}
	}
	return value
}

func appendUniqueFold(values []any, text string) []any {
	for _, value := range values {
		if strings.EqualFold(fmt.Sprint(value), text) {
			return values
		}
	}
	return append(values, text)
}

func removeTextFold(values []any, text string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		if !strings.EqualFold(fmt.Sprint(value), text) {
			result = append(result, value)
		}
	}
	return result
}

func (s *Store) groupByID(ctx context.Context, groupID string) (GroupStatus, error) {
	groups, err := s.Groups(ctx)
	if err != nil {
		return GroupStatus{}, err
	}
	for _, group := range groups {
		if group.ID != nil && *group.ID == groupID {
			return group, nil
		}
	}
	return GroupStatus{}, fmt.Errorf("%w：分组 ID %s", ErrGroupNotFound, groupID)
}
