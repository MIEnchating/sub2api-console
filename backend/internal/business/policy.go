package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PolicyGroupStrategy struct {
	ID                  *string  `json:"id"`
	Name                string   `json:"name"`
	Platforms           []string `json:"platforms"`
	Strategy            string   `json:"strategy"`
	StrategySource      string   `json:"strategy_source"`
	ParticipationStatus string   `json:"participation_status"`
	ParticipationReason *string  `json:"participation_reason"`
	AccountCount        int64    `json:"account_count"`
}

type PolicySnapshot struct {
	Available              bool                  `json:"available"`
	Source                 string                `json:"source"`
	Mode                   string                `json:"mode"`
	GlobalStrategy         *string               `json:"global_strategy"`
	GroupStrategies        []PolicyGroupStrategy `json:"group_strategies"`
	MissingRateFallback    *string               `json:"missing_rate_fallback"`
	ChangeThreshold        *string               `json:"change_threshold"`
	CooldownSeconds        *int                  `json:"cooldown_seconds"`
	AutoApply              map[string]any        `json:"auto_apply"`
	ExcludedGroupIDs       []string              `json:"excluded_group_ids"`
	TrafficEnabled         *bool                 `json:"traffic_enabled"`
	ProbeIntervalSeconds   *int                  `json:"probe_interval_seconds"`
	ProbeModel             *string               `json:"probe_model"`
	TrafficLookbackMinutes *int                  `json:"traffic_lookback_minutes"`
	MaxSamplesPerAccount   *int                  `json:"max_samples_per_account"`
	AdvancedPolicy         map[string]any        `json:"advanced_policy"`
	ConfigurationErrors    []string              `json:"configuration_errors"`
}

var policyPatchFields = map[string]struct{}{
	"mode":            {},
	"global_strategy": {}, "missing_rate_fallback": {}, "change_threshold": {}, "cooldown_seconds": {},
	"auto_apply": {}, "excluded_group_ids": {},
	"traffic_enabled": {}, "probe_interval_seconds": {}, "probe_model": {},
	"traffic_lookback_minutes": {}, "max_samples_per_account": {}, "advanced_policy": {}, "group_strategies": {},
}

var strategyAliases = map[string]string{
	"balanced": "balanced", "price_first": "price_first", "speed_first": "speed_first", "reliability": "reliability",
}

// UpdatePolicy applies a partial control-plane patch. Omitted fields are
// preserved, while explicitly supplied empty arrays and objects clear their
// editable values.
func (s *Store) UpdatePolicy(ctx context.Context, rawPatch map[string]any, actor string) (PolicySnapshot, error) {
	normalizedValue, err := normalizeJSONNumbers(rawPatch)
	if err != nil {
		return PolicySnapshot{}, err
	}
	patch, ok := normalizedValue.(map[string]any)
	if !ok {
		return PolicySnapshot{}, fmt.Errorf("策略更新必须是对象")
	}
	for field := range patch {
		if _, allowed := policyPatchFields[field]; !allowed {
			return PolicySnapshot{}, fmt.Errorf("策略更新包含未知字段：%s", field)
		}
	}
	runtimeMode := ""
	if rawMode, present := patch["mode"]; present {
		parsedMode, ok := rawMode.(string)
		if !ok || !validMode(parsedMode) {
			return PolicySnapshot{}, fmt.Errorf("运行模式只能是监控模式或完全模式")
		}
		runtimeMode = parsedMode
		delete(patch, "mode")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PolicySnapshot{}, err
	}
	defer tx.Rollback()
	current, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return PolicySnapshot{}, err
	}
	if current == nil {
		return PolicySnapshot{}, fmt.Errorf("控制面策略记录不存在，无法执行局部更新")
	}
	updated, touchedGroupIDs, globalStrategy, err := normalizePolicyPatch(ctx, tx, current, patch)
	if err != nil {
		return PolicySnapshot{}, err
	}
	if err := validateManualPriorityCapacity(ctx, tx, updated); err != nil {
		return PolicySnapshot{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", updated, now); err != nil {
		return PolicySnapshot{}, err
	}
	if runtimeMode != "" {
		if err := updateRuntimeModeTx(ctx, tx, runtimeMode, now); err != nil {
			return PolicySnapshot{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET strategy=?,updated_at=? WHERE strategy_source='global_default'`, globalStrategy, now); err != nil {
		return PolicySnapshot{}, err
	}
	bindings, _ := updated["group_policy_bindings"].(map[string]any)
	for _, groupID := range touchedGroupIDs {
		strategy, source := globalStrategy, "global_default"
		if binding, found := bindings[groupID].(map[string]any); found {
			if rawStrategy, present := binding["strategy"]; present {
				strategy, err = normalizeStrategy(rawStrategy)
				if err != nil {
					return PolicySnapshot{}, err
				}
				source = "group_override"
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET strategy=?,strategy_source=?,updated_at=? WHERE remote_id=?`, strategy, source, now, groupID); err != nil {
			return PolicySnapshot{}, err
		}
	}
	eventPayload := map[string]any{"actor": strings.TrimSpace(actor), "strategy": globalStrategy}
	if runtimeMode != "" {
		eventPayload["mode"] = runtimeMode
	}
	payload, err := json.Marshal(eventPayload)
	if err != nil {
		return PolicySnapshot{}, err
	}
	var minimum sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&minimum); err != nil {
		return PolicySnapshot{}, err
	}
	sourceID := int64(-1)
	if minimum.Valid && minimum.Int64 <= -1 {
		sourceID = minimum.Int64 - 1
	}
	summary := "策略配置已更新"
	if strings.TrimSpace(actor) != "" {
		summary += "：" + strings.TrimSpace(actor)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json)
		VALUES(?,?,?,?,?,?)`, sourceID, "policy.updated", now, "succeeded", summary, string(payload)); err != nil {
		return PolicySnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return PolicySnapshot{}, err
	}
	return s.PolicySnapshot(ctx)
}

func normalizePolicyPatch(ctx context.Context, tx *sql.Tx, current, patch map[string]any) (map[string]any, []string, string, error) {
	updated := copyObject(current)
	sections := map[string]map[string]any{}
	for _, section := range []string{"selection", "weights", "scope", "probe", "traffic"} {
		value, present := updated[section]
		if !present {
			sections[section] = map[string]any{}
			updated[section] = sections[section]
			continue
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, nil, "", fmt.Errorf("策略字段 %s 必须是对象，不能静默回退默认配置", section)
		}
		sections[section] = copyObject(object)
		updated[section] = sections[section]
	}
	if rawAdvanced, present := patch["advanced_policy"]; present {
		advanced, ok := rawAdvanced.(map[string]any)
		if !ok {
			return nil, nil, "", fmt.Errorf("策略字段 advanced_policy 必须是对象")
		}
		for section := range advanced {
			if _, allowed := advancedPolicyCoreFields[section]; !allowed {
				return nil, nil, "", fmt.Errorf("高级策略包含未知分区：%s", section)
			}
		}
		for section, rawIncoming := range advanced {
			coreFields := advancedPolicyCoreFields[section]
			incoming, ok := rawIncoming.(map[string]any)
			if !ok {
				return nil, nil, "", fmt.Errorf("高级策略字段 %s 必须是对象", section)
			}
			for field := range incoming {
				if _, conflict := coreFields[field]; conflict {
					return nil, nil, "", fmt.Errorf("高级策略字段 %s 不能覆盖基础字段：%s", section, field)
				}
			}
			normalizedIncoming, err := validateAdvancedSection(section, incoming)
			if err != nil {
				return nil, nil, "", err
			}
			existingRaw, existingPresent := updated[section]
			existing, ok := existingRaw.(map[string]any)
			if !existingPresent {
				existing = map[string]any{}
				ok = true
			}
			if !ok {
				existing = map[string]any{}
			}
			replacement := map[string]any{}
			for field := range coreFields {
				if value, present := existing[field]; present {
					replacement[field] = value
				}
			}
			for field, value := range normalizedIncoming {
				replacement[field] = value
			}
			updated[section] = replacement
			if _, coreSection := sections[section]; coreSection {
				sections[section] = replacement
			}
		}
	}
	globalRaw, err := patchOrCurrent(patch, "global_strategy", current, []string{"selection.strategy"}, "balanced")
	if err != nil {
		return nil, nil, "", err
	}
	globalStrategy, err := normalizeStrategy(globalRaw)
	if err != nil {
		return nil, nil, "", fmt.Errorf("策略字段 global_strategy 无效")
	}
	updated["strategy"] = globalStrategy
	sections["selection"]["strategy"] = globalStrategy

	missingRaw, err := patchOrCurrent(patch, "missing_rate_fallback", current, []string{"weights.scheduling_missing_rate_fallback"}, "current_cost_wall")
	if err != nil {
		return nil, nil, "", err
	}
	missing := strings.TrimSpace(fmt.Sprint(missingRaw))
	if missing != "current_cost_wall" && missing != "fail_closed" && missing != "fail_open" {
		return nil, nil, "", fmt.Errorf("策略字段 missing_rate_fallback 无效")
	}
	thresholdRaw, err := patchOrCurrent(patch, "change_threshold", current, []string{"weights.change_threshold"}, "0.1")
	if err != nil {
		return nil, nil, "", err
	}
	threshold := strings.TrimSpace(fmt.Sprint(thresholdRaw))
	ratio, ok := new(big.Rat).SetString(threshold)
	if !ok || ratio.Sign() <= 0 || ratio.Cmp(big.NewRat(1, 1)) > 0 {
		return nil, nil, "", fmt.Errorf("策略字段 change_threshold 必须是大于 0 且不超过 1 的十进制数")
	}
	cooldownRaw, err := patchOrCurrent(patch, "cooldown_seconds", current, []string{"weights.cooldown_seconds"}, int64(60))
	if err != nil {
		return nil, nil, "", err
	}
	cooldown, err := boundedInteger("cooldown_seconds", cooldownRaw, 0, 86400)
	if err != nil {
		return nil, nil, "", err
	}
	sections["weights"]["scheduling_missing_rate_fallback"] = missing
	sections["weights"]["change_threshold"] = threshold
	sections["weights"]["cooldown_seconds"] = int64(cooldown)

	if rawAutoApply, present := patch["auto_apply"]; present {
		autoApply, ok := rawAutoApply.(map[string]any)
		if !ok {
			return nil, nil, "", fmt.Errorf("策略字段 auto_apply 必须是对象")
		}
		normalized := map[string]any{}
		if len(autoApply) > 0 {
			existing, existingOK := updated["auto_apply"].(map[string]any)
			if !existingOK && updated["auto_apply"] != nil {
				return nil, nil, "", fmt.Errorf("策略字段 auto_apply 必须是对象")
			}
			normalized = copyObject(existing)
		}
		for key, value := range autoApply {
			if _, allowed := autoApplyFields[key]; !allowed {
				return nil, nil, "", fmt.Errorf("策略字段 auto_apply.%s 未定义", key)
			}
			enabled, ok := value.(bool)
			if !ok {
				return nil, nil, "", fmt.Errorf("策略字段 auto_apply.%s 必须是布尔值", key)
			}
			normalized[key] = enabled
		}
		updated["auto_apply"] = normalized
	} else if value, present := updated["auto_apply"]; present {
		if _, ok := value.(map[string]any); !ok {
			return nil, nil, "", fmt.Errorf("策略字段 auto_apply 必须是对象")
		}
	} else {
		updated["auto_apply"] = map[string]any{}
	}

	for _, field := range []string{"excluded_group_ids"} {
		paths := []string{"scope." + field}
		raw, err := patchOrCurrent(patch, field, current, paths, []any{})
		if err != nil {
			return nil, nil, "", err
		}
		values, err := normalizedStringArray(field, raw)
		if err != nil {
			return nil, nil, "", err
		}
		sections["scope"][field] = values
	}
	trafficEnabledRaw, err := patchOrCurrent(patch, "traffic_enabled", current, []string{"traffic.enabled"}, true)
	if err != nil {
		return nil, nil, "", err
	}
	trafficEnabled, ok := trafficEnabledRaw.(bool)
	if !ok {
		return nil, nil, "", fmt.Errorf("策略字段 traffic_enabled 必须是布尔值")
	}
	sections["traffic"]["enabled"] = trafficEnabled
	probeRaw, err := patchOrCurrent(patch, "probe_interval_seconds", current, []string{"probe.interval_seconds"}, int64(300))
	if err != nil {
		return nil, nil, "", err
	}
	probeInterval, err := boundedInteger("probe_interval_seconds", probeRaw, 30, 86400)
	if err != nil {
		return nil, nil, "", err
	}
	sections["probe"]["interval_seconds"] = int64(probeInterval)
	if rawModel, present := patch["probe_model"]; present {
		model, ok := rawModel.(string)
		if !ok || len(model) > 256 {
			return nil, nil, "", fmt.Errorf("策略字段 probe_model 必须是长度不超过 256 的字符串")
		}
		sections["probe"]["model"] = strings.TrimSpace(model)
	}
	lookbackRaw, err := patchOrCurrent(patch, "traffic_lookback_minutes", current, []string{"traffic.lookback_minutes"}, int64(120))
	if err != nil {
		return nil, nil, "", err
	}
	lookback, err := boundedInteger("traffic_lookback_minutes", lookbackRaw, 1, 10080)
	if err != nil {
		return nil, nil, "", err
	}
	maxSamplesRaw, err := patchOrCurrent(patch, "max_samples_per_account", current, []string{"traffic.max_samples_per_account"}, int64(60))
	if err != nil {
		return nil, nil, "", err
	}
	maxSamples, err := boundedInteger("max_samples_per_account", maxSamplesRaw, 1, 200)
	if err != nil {
		return nil, nil, "", err
	}
	sections["traffic"]["lookback_minutes"] = int64(lookback)
	sections["traffic"]["max_samples_per_account"] = int64(maxSamples)

	touchedGroupIDs := []string{}
	if rawStrategies, present := patch["group_strategies"]; present {
		strategies, ok := rawStrategies.(map[string]any)
		if !ok {
			return nil, nil, "", fmt.Errorf("策略字段 group_strategies 必须是对象")
		}
		bindings, ok := updated["group_policy_bindings"].(map[string]any)
		if !ok && updated["group_policy_bindings"] != nil {
			return nil, nil, "", fmt.Errorf("策略字段 group_policy_bindings 必须是对象")
		}
		bindings = copyObject(bindings)
		for groupID, rawStrategy := range strategies {
			if !stableNumericID(groupID) {
				return nil, nil, "", fmt.Errorf("策略字段 group_strategies.%s 必须使用稳定数字 ID", groupID)
			}
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_groups WHERE remote_id=?`, groupID).Scan(&exists); err != nil {
				return nil, nil, "", err
			}
			if exists == 0 {
				return nil, nil, "", fmt.Errorf("策略字段 group_strategies.%s 必须是已登记分组的稳定 ID", groupID)
			}
			binding, _ := bindings[groupID].(map[string]any)
			binding = copyObject(binding)
			if rawStrategy == nil {
				delete(binding, "strategy")
			} else {
				strategy, err := normalizeStrategy(rawStrategy)
				if err != nil {
					return nil, nil, "", fmt.Errorf("策略字段 group_strategies.%s 无效", groupID)
				}
				binding["strategy"] = strategy
			}
			if len(binding) == 0 {
				delete(bindings, groupID)
			} else {
				bindings[groupID] = binding
			}
			touchedGroupIDs = append(touchedGroupIDs, groupID)
		}
		sort.Strings(touchedGroupIDs)
		updated["group_policy_bindings"] = bindings
	}
	if err := validatePolicyRelationships(updated); err != nil {
		return nil, nil, "", err
	}
	return updated, touchedGroupIDs, globalStrategy, nil
}

func validatePolicyRelationships(policy map[string]any) error {
	type integerRelation struct {
		leftSection, leftField    string
		rightSection, rightField  string
		leftDefault, rightDefault int
		message                   string
	}
	relations := []integerRelation{
		{"scoring", "short_window", "scoring", "long_window", 10, 60, "scoring.long_window 不能小于 scoring.short_window"},
		{"breaker", "http_failures", "breaker", "http_window", 3, 5, "breaker.http_failures 不能大于 breaker.http_window"},
		{"breaker", "latency_occurrences", "breaker", "latency_window", 5, 10, "breaker.latency_occurrences 不能大于 breaker.latency_window"},
		{"weights", "min_load_factor", "weights", "max_load_factor", 1, 100, "weights.min_load_factor 不能大于 weights.max_load_factor"},
		{"scaling", "min_per_account", "scaling", "max_per_account", 3, 250, "scaling.min_per_account 不能大于 scaling.max_per_account"},
		{"cleanup", "occurrences", "cleanup", "window", 3, 5, "cleanup.occurrences 不能大于 cleanup.window"},
	}
	for _, relation := range relations {
		left, err := effectivePolicyInteger(policy, relation.leftSection, relation.leftField, relation.leftDefault)
		if err != nil {
			return err
		}
		right, err := effectivePolicyInteger(policy, relation.rightSection, relation.rightField, relation.rightDefault)
		if err != nil {
			return err
		}
		if left > right {
			return fmt.Errorf("调度策略字段关系无效：%s", relation.message)
		}
	}
	return nil
}

func effectivePolicyInteger(policy map[string]any, section, field string, fallback int) (int, error) {
	rawSection, present := policy[section]
	if !present {
		return fallback, nil
	}
	object, ok := rawSection.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("策略字段 %s 必须是对象", section)
	}
	raw, present := object[field]
	if !present {
		return fallback, nil
	}
	value, err := strictInteger(raw)
	if err != nil {
		return 0, fmt.Errorf("策略字段 %s.%s 必须是整数", section, field)
	}
	return value, nil
}

func patchOrCurrent(patch map[string]any, field string, current map[string]any, paths []string, fallback any) (any, error) {
	if value, present := patch[field]; present {
		if value == nil {
			return nil, fmt.Errorf("策略字段 %s 不能为 null", field)
		}
		return value, nil
	}
	for _, path := range paths {
		if value, present := lookupPolicyPath(current, path); present {
			if value == nil {
				return nil, fmt.Errorf("策略字段 %s 当前值不能为 null", field)
			}
			return value, nil
		}
	}
	return fallback, nil
}

func normalizeStrategy(value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("strategy must be string")
	}
	if normalized, found := strategyAliases[strings.ToLower(strings.TrimSpace(text))]; found {
		return normalized, nil
	}
	return "", fmt.Errorf("unknown strategy")
}

func boundedInteger(field string, value any, minimum, maximum int) (int, error) {
	parsed, err := strictInteger(value)
	if err != nil {
		return 0, fmt.Errorf("策略字段 %s 必须是整数", field)
	}
	if parsed < minimum {
		return 0, fmt.Errorf("策略字段 %s 不能小于 %d", field, minimum)
	}
	if parsed > maximum {
		return 0, fmt.Errorf("策略字段 %s 不能大于 %d", field, maximum)
	}
	return parsed, nil
}

func normalizedStringArray(field string, value any) ([]any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("策略字段 %s 必须是数组", field)
	}
	result := make([]any, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("策略字段 %s 只能包含非空字符串", field)
		}
		result = append(result, strings.TrimSpace(text))
	}
	return result, nil
}

func copyObject(value map[string]any) map[string]any {
	result := map[string]any{}
	for key, item := range value {
		result[key] = item
	}
	return result
}

func stableNumericID(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && parsed > 0
}

type advancedRule struct {
	kind      string
	minimum   float64
	maximum   float64
	allowed   map[string]struct{}
	maxLength int
}

var advancedRules = map[string]map[string]advancedRule{
	"selection": {},
	"weights": {
		"enabled": {kind: "bool"}, "budget": {kind: "int", minimum: 1, maximum: 1_000_000},
		"gate_floor": {kind: "number", minimum: 0, maximum: 100}, "price_exp": {kind: "positive_number", maximum: 100},
		"speed_exp": {kind: "positive_number", maximum: 100}, "balanced_price_ratio": {kind: "ratio"},
		"min_load_factor": {kind: "int", minimum: 1, maximum: 1_000_000}, "max_load_factor": {kind: "int", minimum: 1, maximum: 1_000_000},
	},
	"manual_priority": {
		"reserved_max": {kind: "int", minimum: 1, maximum: 1000},
	},
	"scope": {
		"manage_all_accounts": {kind: "bool"},
		"managed_group_mode":  {kind: "enum", allowed: valueStringSet("all", "selected")},
		"managed_group_ids":   {kind: "strings"}, "excluded_group_ids": {kind: "strings"},
		"account_types": {kind: "strings"}, "platforms": {kind: "strings"},
		"paused_account_ids": {kind: "strings"}, "excluded_account_ids": {kind: "strings"}, "manual_fused_account_ids": {kind: "strings"},
	},
	"probe": {
		"enabled": {kind: "bool"}, "timeout_seconds": {kind: "int", minimum: 1, maximum: 86400},
		"concurrency": {kind: "int", minimum: 1, maximum: 32}, "prompt": {kind: "string", maxLength: 10000},
		"skip_when_traffic_fresh": {kind: "bool"}, "traffic_fresh_seconds": {kind: "int", minimum: 1, maximum: 86400},
		"retry_enabled": {kind: "bool"}, "retry_source": {kind: "enum", allowed: valueStringSet("fixed", "sub2api_pool")},
		"retry_count": {kind: "int", minimum: 0, maximum: 10}, "retry_status_codes": {kind: "status_codes"},
	},
	"traffic": {
		"refresh_seconds": {kind: "int", minimum: 1, maximum: 86400},
	},
	"scoring": {
		"short_window": {kind: "int", minimum: 1, maximum: 10000}, "long_window": {kind: "int", minimum: 1, maximum: 100000},
		"latest_weight": {kind: "ratio"}, "short_ratio": {kind: "ratio"},
		"slow_ttfb_ms": {kind: "int", minimum: 1, maximum: 3_600_000}, "event_scores": {kind: "event_scores"},
	},
	"breaker": {
		"enabled": {kind: "bool"}, "hard_fatal": {kind: "bool"}, "http_degrade_only": {kind: "bool"}, "latency_degrade_only": {kind: "bool"},
		"http_window": {kind: "int", minimum: 1, maximum: 10000}, "http_failures": {kind: "int", minimum: 1, maximum: 10000},
		"http_score_below": {kind: "number", minimum: 0, maximum: 100}, "latency_window": {kind: "int", minimum: 1, maximum: 10000},
		"latency_occurrences": {kind: "int", minimum: 1, maximum: 10000}, "latency_ttfb_ms": {kind: "int", minimum: 1, maximum: 3_600_000},
		"max_switch_per_round": {kind: "int", minimum: 1, maximum: 10000}, "fused_cooldown_seconds": {kind: "int", minimum: 0, maximum: 86400},
		"instant_status_codes": {kind: "status_codes"}, "min_pool_size": {kind: "int", minimum: 0, maximum: 10000},
		"min_pool_score": {kind: "number", minimum: 0, maximum: 100},
	},
	"degrade": {
		"enabled": {kind: "bool"}, "score_threshold": {kind: "number", minimum: 0, maximum: 100},
		"priority_step": {kind: "int", minimum: 1, maximum: 1_000_000}, "load_factor_ratio": {kind: "ratio"},
		"min_load_factor": {kind: "int", minimum: 1, maximum: 1_000_000},
	},
	"recovery": {
		"enabled": {kind: "bool"}, "probe_interval_seconds": {kind: "int", minimum: 1, maximum: 86400},
		"target_score": {kind: "number", minimum: 0, maximum: 100}, "success_count": {kind: "int", minimum: 1, maximum: 10000},
		"hold_seconds": {kind: "int", minimum: 0, maximum: 86400},
	},
	"scaling": {
		"enabled": {kind: "bool"}, "global_max_concurrency": {kind: "int", minimum: 1, maximum: 10_000_000},
		"min_per_account": {kind: "int", minimum: 1, maximum: 1_000_000}, "max_per_account": {kind: "int", minimum: 1, maximum: 1_000_000},
		"scale_up_ratio": {kind: "ratio"}, "step_up": {kind: "int", minimum: 1, maximum: 1_000_000},
		"step_down": {kind: "int", minimum: 1, maximum: 1_000_000}, "cooldown_seconds": {kind: "int", minimum: 0, maximum: 86400},
	},
	"cleanup": {
		"enabled": {kind: "bool"}, "action": {kind: "enum", allowed: valueStringSet("none", "pause", "disable", "delete")},
		"occurrences": {kind: "int", minimum: 1, maximum: 10000}, "window": {kind: "int", minimum: 1, maximum: 10000},
		"min_fused_minutes": {kind: "int", minimum: 0, maximum: 10080}, "max_per_round": {kind: "int", minimum: 1, maximum: 10000},
		"keep_last_in_group": {kind: "bool"}, "only_auth_errors": {kind: "bool"}, "trigger_status_codes": {kind: "status_codes"},
	},
	"upstream_multiplier": {
		"interval_seconds": {kind: "int", minimum: 30, maximum: 86400},
	},
	"price_management": {
		"enabled": {kind: "bool"}, "profit_margin": {kind: "number", minimum: 0, maximum: 0.99},
		"exchange_group_sets": {kind: "string_groups"}, "interval_seconds": {kind: "int", minimum: 30, maximum: 86400},
		"write_concurrency": {kind: "int", minimum: 1, maximum: 16},
	},
	"writeback": {
		"concurrency": {kind: "int", minimum: 1, maximum: 16}, "verification": {kind: "bool"},
	},
	"classify": {
		"fatal_patterns": {kind: "strings"}, "gateway_status_codes": {kind: "status_codes"},
	},
}

func validateAdvancedSection(section string, values map[string]any) (map[string]any, error) {
	rules, knownSection := advancedRules[section]
	if !knownSection {
		return nil, fmt.Errorf("高级策略包含未知分区：%s", section)
	}
	result := map[string]any{}
	for field, value := range values {
		rule, knownField := rules[field]
		if !knownField {
			return nil, fmt.Errorf("高级策略字段 %s.%s 未定义", section, field)
		}
		path := section + "." + field
		normalized, err := validateAdvancedValue(path, value, rule)
		if err != nil {
			return nil, err
		}
		result[field] = normalized
	}
	return result, nil
}

func validateAdvancedValue(path string, value any, rule advancedRule) (any, error) {
	switch rule.kind {
	case "bool":
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("高级策略字段 %s 必须是布尔值", path)
		}
		return value, nil
	case "int":
		parsed, err := boundedInteger(path, value, int(rule.minimum), int(rule.maximum))
		if err != nil {
			return nil, err
		}
		return int64(parsed), nil
	case "number":
		parsed, ok := finiteNumber(value)
		if !ok {
			return nil, fmt.Errorf("高级策略字段 %s 必须是有限数值", path)
		}
		if parsed < rule.minimum || parsed > rule.maximum {
			return nil, fmt.Errorf("高级策略字段 %s 必须在 %s 到 %s 之间", path, formatNumber(rule.minimum), formatNumber(rule.maximum))
		}
		return parsed, nil
	case "positive_number":
		parsed, ok := finiteNumber(value)
		if !ok || parsed <= 0 || parsed > rule.maximum {
			return nil, fmt.Errorf("高级策略字段 %s 必须大于 0 且不超过 %s", path, formatNumber(rule.maximum))
		}
		return parsed, nil
	case "ratio":
		parsed, ok := finiteNumber(value)
		if !ok || parsed <= 0 || parsed > 1 {
			return nil, fmt.Errorf("高级策略字段 %s 必须大于 0 且不超过 1", path)
		}
		return parsed, nil
	case "string":
		text, ok := value.(string)
		if !ok || (rule.maxLength > 0 && len(text) > rule.maxLength) {
			return nil, fmt.Errorf("高级策略字段 %s 必须是长度不超过 %d 的字符串", path, rule.maxLength)
		}
		return text, nil
	case "enum":
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("高级策略字段 %s 必须是字符串", path)
		}
		if _, allowed := rule.allowed[text]; !allowed {
			return nil, fmt.Errorf("高级策略字段 %s 的选项无效", path)
		}
		return text, nil
	case "strings":
		return normalizedStringArray(path, value)
	case "string_groups":
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("高级策略字段 %s 必须是数组", path)
		}
		result := make([]any, 0, len(items))
		seen := map[string]struct{}{}
		for index, item := range items {
			group, err := normalizedStringArray(fmt.Sprintf("%s[%d]", path, index), item)
			if err != nil {
				return nil, err
			}
			if len(group) < 2 {
				return nil, fmt.Errorf("高级策略字段 %s[%d] 至少需要两个分组", path, index)
			}
			for _, rawID := range group {
				id := fmt.Sprint(rawID)
				if _, duplicate := seen[id]; duplicate {
					return nil, fmt.Errorf("高级策略字段 %s 中的分组 %s 不能重复", path, id)
				}
				seen[id] = struct{}{}
			}
			result = append(result, group)
		}
		return result, nil
	case "status_codes":
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("高级策略字段 %s 必须是数组", path)
		}
		result := make([]any, 0, len(items))
		seen := map[int]struct{}{}
		for _, item := range items {
			code, err := boundedInteger(path, item, 100, 599)
			if err != nil {
				return nil, err
			}
			if _, found := seen[code]; found {
				continue
			}
			seen[code] = struct{}{}
			result = append(result, int64(code))
		}
		if path == "classify.gateway_status_codes" && len(result) == 0 {
			return []any{int64(429), int64(500), int64(502), int64(503), int64(504)}, nil
		}
		return result, nil
	case "event_scores":
		scores, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("高级策略字段 %s 必须是对象", path)
		}
		allowed := valueStringSet("perfect", "slow_ttfb", "upstream_unknown", "gateway_error", "quota_exhausted", "probe_fail", "fatal")
		result := map[string]any{}
		for key, rawScore := range scores {
			if _, found := allowed[key]; !found {
				return nil, fmt.Errorf("高级策略字段 %s.%s 未定义", path, key)
			}
			minimum := 0
			if key == "quota_exhausted" {
				minimum = 1
			}
			score, ok := finiteNumber(rawScore)
			if !ok {
				return nil, fmt.Errorf("高级策略字段 %s.%s 必须是有限数值", path, key)
			}
			if score < float64(minimum) {
				return nil, fmt.Errorf("策略字段 %s.%s 不能小于 %d", path, key, minimum)
			}
			if score > 100 {
				return nil, fmt.Errorf("策略字段 %s.%s 不能大于 100", path, key)
			}
			result[key] = score
		}
		return result, nil
	default:
		return nil, fmt.Errorf("高级策略字段 %s 的类型规则缺失", path)
	}
}

func finiteNumber(value any) (float64, bool) {
	var parsed float64
	switch item := value.(type) {
	case int:
		parsed = float64(item)
	case int64:
		parsed = float64(item)
	case float64:
		parsed = item
	default:
		return 0, false
	}
	return parsed, !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func valueStringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

var advancedPolicyCoreFields = map[string]map[string]struct{}{
	"selection":           {"strategy": {}},
	"weights":             {"scheduling_missing_rate_fallback": {}, "change_threshold": {}, "cooldown_seconds": {}},
	"manual_priority":     {},
	"scope":               {"excluded_group_ids": {}},
	"probe":               {"interval_seconds": {}, "model": {}},
	"traffic":             {"enabled": {}, "lookback_minutes": {}, "max_samples_per_account": {}},
	"scoring":             {},
	"breaker":             {},
	"degrade":             {},
	"recovery":            {},
	"scaling":             {},
	"cleanup":             {},
	"upstream_multiplier": {},
	"price_management":    {},
	"writeback":           {},
	"classify":            {},
}

var autoApplyFields = valueStringSet("schedulable", "priority", "load_factor", "concurrency")

func (s *Store) PolicySnapshot(ctx context.Context) (PolicySnapshot, error) {
	runtime, err := s.RuntimeSnapshot(ctx)
	if err != nil {
		return PolicySnapshot{}, err
	}
	document, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return PolicySnapshot{}, err
	}
	if document == nil {
		return PolicySnapshot{
			Available: false, Source: "控制面策略记录不存在", Mode: runtime.Mode, GroupStrategies: []PolicyGroupStrategy{},
			AdvancedPolicy: map[string]any{}, ConfigurationErrors: []string{"control-plane"},
		}, nil
	}
	result := PolicySnapshot{
		Available: true, Source: "当前控制面策略", Mode: runtime.Mode, GroupStrategies: []PolicyGroupStrategy{},
		AdvancedPolicy: map[string]any{}, ConfigurationErrors: []string{},
	}
	result.GlobalStrategy = policyString(document, &result.ConfigurationErrors, "global_strategy", "selection.strategy")
	if result.GlobalStrategy == nil && !pathPresent(document, "selection.strategy") {
		result.GlobalStrategy = stringPointer("balanced")
	}
	result.MissingRateFallback = policyString(document, &result.ConfigurationErrors, "missing_rate_fallback", "weights.scheduling_missing_rate_fallback")
	if result.MissingRateFallback == nil && !pathPresent(document, "weights.scheduling_missing_rate_fallback") {
		result.MissingRateFallback = stringPointer("current_cost_wall")
	}
	result.ChangeThreshold = policyString(document, &result.ConfigurationErrors, "change_threshold", "weights.change_threshold")
	if result.ChangeThreshold == nil && !pathPresent(document, "weights.change_threshold") {
		result.ChangeThreshold = stringPointer("0.1")
	}
	result.CooldownSeconds = policyInteger(document, &result.ConfigurationErrors, "weights.cooldown_seconds", "weights.cooldown_seconds")
	if result.CooldownSeconds == nil && !pathPresent(document, "weights.cooldown_seconds") {
		result.CooldownSeconds = intPointer(60)
	}
	result.TrafficEnabled = policyBool(document, &result.ConfigurationErrors, "traffic_enabled", "traffic.enabled")
	if result.TrafficEnabled == nil && !pathPresent(document, "traffic.enabled") {
		result.TrafficEnabled = boolPointer(true)
	}
	result.ProbeIntervalSeconds = policyInteger(document, &result.ConfigurationErrors, "probe_interval_seconds", "probe.interval_seconds")
	if result.ProbeIntervalSeconds == nil && !pathPresent(document, "probe.interval_seconds") {
		result.ProbeIntervalSeconds = intPointer(300)
	}
	result.TrafficLookbackMinutes = policyInteger(document, &result.ConfigurationErrors, "traffic_lookback_minutes", "traffic.lookback_minutes")
	if result.TrafficLookbackMinutes == nil && !pathPresent(document, "traffic.lookback_minutes") {
		result.TrafficLookbackMinutes = intPointer(120)
	}
	result.MaxSamplesPerAccount = policyInteger(document, &result.ConfigurationErrors, "max_samples_per_account", "traffic.max_samples_per_account")
	if result.MaxSamplesPerAccount == nil && !pathPresent(document, "traffic.max_samples_per_account") {
		result.MaxSamplesPerAccount = intPointer(60)
	}
	result.ProbeModel = policyString(document, &result.ConfigurationErrors, "probe_model", "probe.model")
	if result.ProbeModel == nil && !pathPresent(document, "probe.model") {
		result.ProbeModel = stringPointer("")
	}
	result.AutoApply = defaultAutoApply()
	if value, present := document["auto_apply"]; present {
		if object, ok := value.(map[string]any); ok {
			result.AutoApply = object
		} else {
			result.AutoApply = nil
			result.ConfigurationErrors = append(result.ConfigurationErrors, "auto_apply")
		}
	}
	result.ExcludedGroupIDs = policyStringList(document, &result.ConfigurationErrors, "excluded_group_ids", "scope.excluded_group_ids")
	defaults := initialControlPolicy()
	for section, core := range advancedPolicyCoreFields {
		raw, present := document[section]
		object := map[string]any{}
		if present {
			var ok bool
			object, ok = raw.(map[string]any)
			if !ok {
				result.ConfigurationErrors = append(result.ConfigurationErrors, section)
				result.AdvancedPolicy[section] = raw
				continue
			}
		}
		defaultObject, _ := defaults[section].(map[string]any)
		if defaultObject == nil && !present {
			continue
		}
		advanced := map[string]any{}
		for key := range advancedRules[section] {
			if _, retained := core[key]; retained {
				continue
			}
			value, found := object[key]
			if !found {
				value, found = defaultObject[key]
			}
			if found {
				advanced[key] = projectAdvancedSnapshotValue(section, key, value)
			}
		}
		if len(advanced) > 0 {
			result.AdvancedPolicy[section] = advanced
		}
	}
	groups, err := s.policyGroupStrategies(ctx, document)
	if err != nil {
		return PolicySnapshot{}, err
	}
	result.GroupStrategies = groups
	result.ConfigurationErrors = uniquePreservingOrder(result.ConfigurationErrors)
	return result, nil
}

func (s *Store) policyGroupStrategies(ctx context.Context, control map[string]any) ([]PolicyGroupStrategy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name,remote_id,platform,strategy,strategy_source,account_count
		FROM local_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PolicyGroupStrategy, 0)
	for rows.Next() {
		var item PolicyGroupStrategy
		var remoteID, platform, strategy, strategySource sql.NullString
		if err := rows.Scan(&item.Name, &remoteID, &platform, &strategy, &strategySource, &item.AccountCount); err != nil {
			return nil, err
		}
		item.ID = nullString(remoteID)
		item.Platforms = []string{}
		if platform.Valid && strings.TrimSpace(platform.String) != "" {
			item.Platforms = []string{platform.String}
		}
		item.Strategy, item.StrategySource = groupStrategy(control, item.ID, nullString(strategy), nullString(strategySource))
		item.ParticipationStatus, item.ParticipationReason = groupParticipation(control, item.ID, item.Name)
		result = append(result, item)
	}
	return result, rows.Err()
}

func projectAdvancedSnapshotValue(_ string, _ string, value any) any { return value }

func lookupPolicyPath(document map[string]any, path string) (any, bool) {
	var current any = document
	parts := strings.Split(path, ".")
	for index, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, index > 0
		}
		value, present := object[part]
		if !present {
			return nil, false
		}
		current = value
	}
	return current, true
}

func pathPresent(document map[string]any, path string) bool {
	_, present := lookupPolicyPath(document, path)
	return present
}

func policyString(document map[string]any, errorsFound *[]string, field string, paths ...string) *string {
	for _, path := range paths {
		value, present := lookupPolicyPath(document, path)
		if !present {
			continue
		}
		if value == nil {
			*errorsFound = append(*errorsFound, field)
			return nil
		}
		return stringPointer(fmt.Sprint(value))
	}
	return nil
}

func policyInteger(document map[string]any, errorsFound *[]string, field string, paths ...string) *int {
	for _, path := range paths {
		value, present := lookupPolicyPath(document, path)
		if !present {
			continue
		}
		parsed, err := strictInteger(value)
		if err != nil {
			*errorsFound = append(*errorsFound, field)
			return nil
		}
		return &parsed
	}
	return nil
}

func policyBool(document map[string]any, errorsFound *[]string, field string, paths ...string) *bool {
	for _, path := range paths {
		value, present := lookupPolicyPath(document, path)
		if !present {
			continue
		}
		parsed, ok := value.(bool)
		if !ok {
			*errorsFound = append(*errorsFound, field)
			return nil
		}
		return &parsed
	}
	return nil
}

func policyStringList(document map[string]any, errorsFound *[]string, field string, paths ...string) []string {
	for _, path := range paths {
		value, present := lookupPolicyPath(document, path)
		if !present {
			continue
		}
		values, ok := value.([]any)
		if !ok {
			*errorsFound = append(*errorsFound, field)
			return nil
		}
		result := make([]string, 0, len(values))
		for _, item := range values {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				*errorsFound = append(*errorsFound, field)
				return nil
			}
			result = append(result, strings.TrimSpace(text))
		}
		return result
	}
	return []string{}
}

func defaultAutoApply() map[string]any {
	return map[string]any{"schedulable": true, "priority": true, "load_factor": true, "concurrency": false}
}

func uniquePreservingOrder(values []string) []string {
	seen, result := map[string]struct{}{}, []string{}
	for _, value := range values {
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func intPointer(value int) *int {
	result := value
	return &result
}

func boolPointer(value bool) *bool {
	result := value
	return &result
}
