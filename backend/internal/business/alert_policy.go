package business

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

type AlertPolicy struct {
	Enabled                 bool     `json:"enabled"`
	ConfigurationEnabled    bool     `json:"configuration_enabled"`
	AuthEnabled             bool     `json:"auth_enabled"`
	RateSyncEnabled         bool     `json:"rate_sync_enabled"`
	BalanceEnabled          bool     `json:"balance_enabled"`
	ProbeEnabled            bool     `json:"probe_enabled"`
	RoutingBreakerEnabled   bool     `json:"routing_breaker_enabled"`
	RoutingDegradedEnabled  bool     `json:"routing_degraded_enabled"`
	RoutingSurvivorEnabled  bool     `json:"routing_survivor_enabled"`
	GroupUnavailableEnabled bool     `json:"group_unavailable_enabled"`
	GroupSurvivorEnabled    bool     `json:"group_survivor_enabled"`
	ApplyFailureEnabled     bool     `json:"apply_failure_enabled"`
	BalanceThresholds       []string `json:"balance_thresholds"`
	ProbeFailureStreak      int      `json:"probe_failure_streak"`
	ProbeRecoveryStreak     int      `json:"probe_recovery_streak"`
	ProbeGroups             []string `json:"probe_groups"`
	DeliveryEnabled         bool     `json:"delivery_enabled"`
	NotifyRecovery          bool     `json:"notify_recovery"`
	RepeatIntervalMinutes   int      `json:"repeat_interval_minutes"`
	StateChangeCooldown     int      `json:"state_change_cooldown_minutes"`
	MergeThreshold          int      `json:"merge_threshold"`
}

func DefaultAlertPolicy() AlertPolicy {
	return AlertPolicy{
		Enabled: true, ConfigurationEnabled: true, AuthEnabled: true, RateSyncEnabled: true,
		BalanceEnabled: true, ProbeEnabled: true, BalanceThresholds: []string{"20", "10", "5"},
		RoutingBreakerEnabled: true, RoutingDegradedEnabled: true, RoutingSurvivorEnabled: true,
		GroupUnavailableEnabled: true, GroupSurvivorEnabled: true, ApplyFailureEnabled: true,
		ProbeFailureStreak: 3, ProbeRecoveryStreak: 3, ProbeGroups: []string{}, DeliveryEnabled: true,
		NotifyRecovery: true, StateChangeCooldown: 30, MergeThreshold: 10,
	}
}

func (s *Store) AlertPolicy(ctx context.Context) (AlertPolicy, error) {
	document, err := s.readPolicyDocument(ctx, s.db, "alert-policy")
	if err != nil {
		return AlertPolicy{}, err
	}
	if document == nil {
		return DefaultAlertPolicy(), nil
	}
	return normalizeAlertPolicy(document, true)
}

func (s *Store) UpdateAlertPolicy(ctx context.Context, raw map[string]any) (AlertPolicy, error) {
	normalizedValue, err := normalizeJSONNumbers(raw)
	if err != nil {
		return AlertPolicy{}, err
	}
	policy, err := normalizeAlertPolicy(normalizedValue.(map[string]any), false)
	if err != nil {
		return AlertPolicy{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AlertPolicy{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "alert-policy", alertPolicyDocument(policy), now); err != nil {
		return AlertPolicy{}, err
	}
	if err := suppressDisabledAlertRules(ctx, tx, policy, now); err != nil {
		return AlertPolicy{}, err
	}
	if err := tx.Commit(); err != nil {
		return AlertPolicy{}, err
	}
	return policy, nil
}

func suppressDisabledAlertRules(ctx context.Context, tx *sql.Tx, policy AlertPolicy, now string) error {
	if !policy.Enabled {
		if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='suppressed',last_seen_at=?,delivery_status='告警总开关已关闭',last_error=NULL WHERE status='firing'`, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,delivery_status='告警总开关已关闭',last_error=NULL WHERE status='recovered'`, now)
		return err
	}
	disabledTypes := make([]string, 0, 11)
	if !policy.ConfigurationEnabled {
		disabledTypes = append(disabledTypes, "upstream.configuration")
	}
	if !policy.AuthEnabled {
		disabledTypes = append(disabledTypes, "upstream.auth")
	}
	if !policy.RateSyncEnabled {
		disabledTypes = append(disabledTypes, "upstream.rate_sync")
	}
	if !policy.BalanceEnabled {
		disabledTypes = append(disabledTypes, "upstream.balance")
	}
	if !policy.ProbeEnabled {
		disabledTypes = append(disabledTypes, "account.probe")
	}
	if !policy.RoutingBreakerEnabled {
		disabledTypes = append(disabledTypes, "account.routing_breaker")
	}
	if !policy.RoutingDegradedEnabled {
		disabledTypes = append(disabledTypes, "account.routing_degraded")
	}
	if !policy.RoutingSurvivorEnabled {
		disabledTypes = append(disabledTypes, "account.routing_survivor")
	}
	if !policy.GroupUnavailableEnabled {
		disabledTypes = append(disabledTypes, "group.routing_unavailable")
	}
	if !policy.GroupSurvivorEnabled {
		disabledTypes = append(disabledTypes, "group.routing_survivor")
	}
	if !policy.ApplyFailureEnabled {
		disabledTypes = append(disabledTypes, "routing.apply_failure")
	}
	for _, eventType := range disabledTypes {
		if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='suppressed',last_seen_at=?,delivery_status='规则已停用',last_error=NULL WHERE status='firing' AND event_type=?`, now, eventType); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,delivery_status='规则已停用',last_error=NULL WHERE status='recovered' AND event_type=?`, now, eventType); err != nil {
			return err
		}
	}
	return nil
}

func normalizeAlertPolicy(raw map[string]any, mergeDefaults bool) (AlertPolicy, error) {
	allowed := valueStringSet("enabled", "configuration_enabled", "auth_enabled", "rate_sync_enabled", "balance_enabled", "probe_enabled",
		"routing_breaker_enabled", "routing_degraded_enabled", "routing_survivor_enabled", "group_unavailable_enabled",
		"group_survivor_enabled", "apply_failure_enabled",
		"balance_thresholds", "balance_threshold", "probe_failure_streak", "probe_recovery_streak", "probe_groups", "delivery_enabled", "notify_recovery", "repeat_interval_minutes", "state_change_cooldown_minutes", "merge_threshold")
	for field := range raw {
		if _, ok := allowed[field]; !ok {
			return AlertPolicy{}, fmt.Errorf("告警策略包含未知字段：%s", field)
		}
	}
	document := alertPolicyDocument(DefaultAlertPolicy())
	if !mergeDefaults && len(raw) != len(alertPolicyDocument(DefaultAlertPolicy())) {
		return AlertPolicy{}, errors.New("告警策略必须提交全部字段")
	}
	for field, value := range raw {
		document[field] = value
	}
	if _, present := raw["balance_thresholds"]; !present {
		if legacy, legacyPresent := raw["balance_threshold"]; legacyPresent {
			document["balance_thresholds"] = []any{legacy}
		}
	}
	delete(document, "balance_threshold")
	booleanFields := []string{
		"enabled", "configuration_enabled", "auth_enabled", "rate_sync_enabled", "balance_enabled", "probe_enabled",
		"routing_breaker_enabled", "routing_degraded_enabled", "routing_survivor_enabled", "group_unavailable_enabled",
		"group_survivor_enabled", "apply_failure_enabled", "delivery_enabled", "notify_recovery",
	}
	booleans := map[string]bool{}
	for _, field := range booleanFields {
		value, ok := document[field].(bool)
		if !ok {
			return AlertPolicy{}, fmt.Errorf("%s 必须是布尔值", field)
		}
		booleans[field] = value
	}
	thresholdItems, ok := document["balance_thresholds"].([]any)
	if !ok || len(thresholdItems) < 1 || len(thresholdItems) > 20 {
		return AlertPolicy{}, errors.New("balance_thresholds 必须包含 1 到 20 个阈值")
	}
	thresholds := map[string]*big.Rat{}
	for _, rawThreshold := range thresholdItems {
		text := strings.TrimSpace(fmt.Sprint(rawThreshold))
		value, ok := new(big.Rat).SetString(text)
		if !ok || value.Sign() <= 0 || value.Cmp(big.NewRat(1_000_000_000_000, 1)) > 0 {
			return AlertPolicy{}, errors.New("balance_thresholds 必须全部是 0 到 1000000000000 之间的有效正数")
		}
		normalized := decimalRatText(value)
		thresholds[normalized] = value
	}
	thresholdTexts := make([]string, 0, len(thresholds))
	for text := range thresholds {
		thresholdTexts = append(thresholdTexts, text)
	}
	sort.Slice(thresholdTexts, func(i, j int) bool { return thresholds[thresholdTexts[i]].Cmp(thresholds[thresholdTexts[j]]) > 0 })
	groups, err := normalizedStringArray("probe_groups", document["probe_groups"])
	if err != nil || len(groups) > 1000 {
		return AlertPolicy{}, errors.New("probe_groups 只能包含最多 1000 个非空分组名称")
	}
	groupTexts := []string{}
	seenGroups := map[string]struct{}{}
	for _, rawGroup := range groups {
		group := rawGroup.(string)
		if len(group) > 255 {
			return AlertPolicy{}, errors.New("probe_groups 分组名称不能超过 255 个字符")
		}
		if _, seen := seenGroups[group]; !seen {
			seenGroups[group] = struct{}{}
			groupTexts = append(groupTexts, group)
		}
	}
	streak, err := boundedInteger("probe_failure_streak", document["probe_failure_streak"], 1, 100)
	if err != nil {
		return AlertPolicy{}, err
	}
	recoveryStreak, err := boundedInteger("probe_recovery_streak", document["probe_recovery_streak"], 1, 100)
	if err != nil {
		return AlertPolicy{}, err
	}
	repeat, err := boundedInteger("repeat_interval_minutes", document["repeat_interval_minutes"], 0, 10080)
	if err != nil {
		return AlertPolicy{}, err
	}
	cooldown, err := boundedInteger("state_change_cooldown_minutes", document["state_change_cooldown_minutes"], 0, 10080)
	if err != nil {
		return AlertPolicy{}, err
	}
	mergeThreshold, err := boundedInteger("merge_threshold", document["merge_threshold"], 2, 500)
	if err != nil {
		return AlertPolicy{}, err
	}
	return AlertPolicy{
		Enabled: booleans["enabled"], ConfigurationEnabled: booleans["configuration_enabled"], AuthEnabled: booleans["auth_enabled"],
		RateSyncEnabled: booleans["rate_sync_enabled"], BalanceEnabled: booleans["balance_enabled"], ProbeEnabled: booleans["probe_enabled"],
		RoutingBreakerEnabled: booleans["routing_breaker_enabled"], RoutingDegradedEnabled: booleans["routing_degraded_enabled"],
		RoutingSurvivorEnabled: booleans["routing_survivor_enabled"], GroupUnavailableEnabled: booleans["group_unavailable_enabled"],
		GroupSurvivorEnabled: booleans["group_survivor_enabled"], ApplyFailureEnabled: booleans["apply_failure_enabled"],
		BalanceThresholds: thresholdTexts, ProbeFailureStreak: streak, ProbeRecoveryStreak: recoveryStreak, ProbeGroups: groupTexts,
		DeliveryEnabled: booleans["delivery_enabled"], NotifyRecovery: booleans["notify_recovery"], RepeatIntervalMinutes: repeat,
		StateChangeCooldown: cooldown, MergeThreshold: mergeThreshold,
	}, nil
}

func alertPolicyDocument(policy AlertPolicy) map[string]any {
	thresholds := make([]any, len(policy.BalanceThresholds))
	for index, value := range policy.BalanceThresholds {
		thresholds[index] = value
	}
	groups := make([]any, len(policy.ProbeGroups))
	for index, value := range policy.ProbeGroups {
		groups[index] = value
	}
	return map[string]any{
		"enabled": policy.Enabled, "configuration_enabled": policy.ConfigurationEnabled, "auth_enabled": policy.AuthEnabled,
		"rate_sync_enabled": policy.RateSyncEnabled, "balance_enabled": policy.BalanceEnabled, "probe_enabled": policy.ProbeEnabled,
		"routing_breaker_enabled": policy.RoutingBreakerEnabled, "routing_degraded_enabled": policy.RoutingDegradedEnabled,
		"routing_survivor_enabled": policy.RoutingSurvivorEnabled, "group_unavailable_enabled": policy.GroupUnavailableEnabled,
		"group_survivor_enabled": policy.GroupSurvivorEnabled, "apply_failure_enabled": policy.ApplyFailureEnabled,
		"balance_thresholds": thresholds, "probe_failure_streak": int64(policy.ProbeFailureStreak), "probe_recovery_streak": int64(policy.ProbeRecoveryStreak), "probe_groups": groups,
		"delivery_enabled": policy.DeliveryEnabled, "notify_recovery": policy.NotifyRecovery, "repeat_interval_minutes": int64(policy.RepeatIntervalMinutes),
		"state_change_cooldown_minutes": int64(policy.StateChangeCooldown), "merge_threshold": int64(policy.MergeThreshold),
	}
}
