package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

type Repository interface {
	EvidenceTargets(context.Context, *string, *string) ([]business.EvidenceTarget, error)
	PersistTrafficSamples(context.Context, []business.TrafficSample) (int, error)
	PersistTrafficFetches(context.Context, []string, time.Time) error
}

type Admin interface {
	RequestDetails(context.Context, string, int, int) ([]map[string]any, error)
}

type ProbeRunner interface {
	RunNow(context.Context, probe.Request) (probe.RunSummary, error)
}

type Options struct {
	AccountID      *string
	GroupName      *string
	FetchTraffic   bool
	StrictFallback bool
	ProbesAllowed  bool
	Now            time.Time
}

type Result struct {
	RequestedSource       string   `json:"requested_source"`
	EffectiveSource       string   `json:"effective_source"`
	TrafficPersisted      int      `json:"traffic_persisted"`
	TrafficChecked        bool     `json:"traffic_checked"`
	ProbesPersisted       int      `json:"probes_persisted"`
	MalformedRows         int      `json:"malformed_rows"`
	MonitoredAccounts     int      `json:"monitored_accounts"`
	FallbackReason        *string  `json:"fallback_reason"`
	SourceErrors          []string `json:"source_errors"`
	TrafficDurationSecond float64  `json:"traffic_duration_seconds"`
	ProbeDurationSecond   float64  `json:"probe_duration_seconds"`
	MonitoringAvailable   *bool    `json:"monitoring_available"`
}

type Plan struct {
	RequestedSource string   `json:"requested_source"`
	ProbeAccountIDs []string `json:"probe_account_ids"`
}

type Service struct {
	repository Repository
	probes     ProbeRunner
}

type collectionPolicy struct {
	source             string
	lookbackMinutes    int
	maxSamples         int
	trafficRefresh     time.Duration
	trafficConcurrency int
	probeEnabled       bool
	probeInterval      time.Duration
	recoveryEnabled    bool
	recoveryInterval   time.Duration
	skipFreshTraffic   bool
	trafficFreshWindow time.Duration
	groupOverrides     map[string]probeGroupOverride
	managedGroupMode   string
	managedGroups      map[string]struct{}
	excludedGroups     map[string]struct{}
	excludedAccounts   map[string]struct{}
	accountTypes       map[string]struct{}
	platforms          map[string]struct{}
}

type probeGroupOverride struct {
	enabled         *bool
	probeEnabled    *bool
	probeInterval   *time.Duration
	recoveryEnabled *bool
}

func New(repository Repository, probes ProbeRunner) *Service {
	return &Service{repository: repository, probes: probes}
}

func (s *Service) Plan(ctx context.Context, policy map[string]any, accountID, groupName *string, now time.Time) (Plan, error) {
	configured, err := parsePolicy(policy)
	if err != nil {
		return Plan{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	targets, err := s.repository.EvidenceTargets(ctx, accountID, groupName)
	if err != nil {
		return Plan{}, err
	}
	targets = filterEvidenceTargets(targets, configured)
	return Plan{
		RequestedSource: configured.source,
		ProbeAccountIDs: dueProbeAccounts(targets, configured, now.UTC(), false, map[string]struct{}{}),
	}, nil
}

func (s *Service) Collect(ctx context.Context, policy map[string]any, admin Admin, options Options) (Result, error) {
	configured, err := parsePolicy(policy)
	if err != nil {
		return Result{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	targets, err := s.repository.EvidenceTargets(ctx, options.AccountID, options.GroupName)
	if err != nil {
		return Result{}, err
	}
	targets = filterEvidenceTargets(targets, configured)
	byAccount := groupTargets(targets)
	result := Result{
		RequestedSource: configured.source, EffectiveSource: configured.source,
		TrafficChecked:    configured.source == "traffic" && options.FetchTraffic,
		MonitoredAccounts: len(byAccount), SourceErrors: []string{},
	}
	monitoringUnavailable := false
	trafficForScope := false
	sourceErrorAccounts := map[string]struct{}{}
	trafficSamples := []business.TrafficSample{}
	successfulFetches := []string{}
	if result.TrafficChecked {
		started := time.Now()
		if admin == nil {
			monitoringUnavailable = true
			result.MonitoringAvailable = boolPointer(false)
			result.SourceErrors = append(result.SourceErrors, "未配置 Sub2API 管理目标")
		} else {
			accountIDs := sortedAccountIDs(byAccount)
			dueIDs := make([]string, 0, len(accountIDs))
			for _, accountID := range accountIDs {
				if !trafficFetchDue(byAccount[accountID], now, configured.trafficRefresh) {
					continue
				}
				dueIDs = append(dueIDs, accountID)
			}
			for _, outcome := range fetchTrafficAccounts(ctx, admin, dueIDs, configured.lookbackMinutes, configured.maxSamples, configured.trafficConcurrency) {
				accountID, rows, readErr := outcome.accountID, outcome.rows, outcome.err
				var disabled *adminclient.MonitoringDisabled
				if errors.As(readErr, &disabled) {
					monitoringUnavailable = true
					result.MonitoringAvailable = boolPointer(false)
					result.SourceErrors = append(result.SourceErrors, disabled.Error())
					continue
				}
				if readErr != nil {
					result.SourceErrors = append(result.SourceErrors, "账号 "+accountID+"："+safeError(readErr))
					sourceErrorAccounts[accountID] = struct{}{}
					continue
				}
				if !monitoringUnavailable {
					result.MonitoringAvailable = boolPointer(true)
				}
				successfulFetches = append(successfulFetches, accountID)
				converted, malformed := convertTrafficRows(accountID, byAccount[accountID], rows, now.Add(-time.Duration(configured.lookbackMinutes)*time.Minute), configured.maxSamples)
				result.MalformedRows += malformed
				if len(converted) > 0 {
					trafficForScope = true
					trafficSamples = append(trafficSamples, converted...)
				}
			}
		}
		result.TrafficDurationSecond = time.Since(started).Seconds()
	}
	// All remote calls above finish before the local write transaction begins.
	if err := s.repository.PersistTrafficFetches(ctx, successfulFetches, now); err != nil {
		return Result{}, err
	}
	if len(trafficSamples) > 0 {
		result.TrafficPersisted, err = s.repository.PersistTrafficSamples(ctx, trafficSamples)
		if err != nil {
			return Result{}, err
		}
		applyFreshTraffic(targets, trafficSamples)
	}
	dueAccounts := dueProbeAccounts(targets, configured, now, monitoringUnavailable, sourceErrorAccounts)
	fallbackRequired := len(dueAccounts) > 0
	if fallbackRequired {
		reason := "没有新鲜真实流量，按探测间隔补充主动探测"
		if monitoringUnavailable {
			reason = "运维监控不可用，降级为主动探测"
		}
		result.FallbackReason = &reason
		if !options.ProbesAllowed {
			result.SourceErrors = append(result.SourceErrors, "需要主动探测回退，但主动探测未开启")
		} else if s.probes == nil {
			result.SourceErrors = append(result.SourceErrors, "需要主动探测回退，但探测执行器不可用")
		} else {
			started := time.Now()
			summary, probeErr := s.probes.RunNow(ctx, probe.Request{AccountIDs: dueAccounts, GroupName: options.GroupName, Automatic: true})
			result.ProbeDurationSecond = time.Since(started).Seconds()
			if probeErr != nil {
				result.SourceErrors = append(result.SourceErrors, safeError(probeErr))
			} else {
				result.ProbesPersisted = summary.Persisted
			}
		}
	}
	freshTraffic := trafficForScope || hasFreshTraffic(targets, now, time.Duration(configured.lookbackMinutes)*time.Minute)
	switch {
	case configured.source == "active_probe":
		result.EffectiveSource = "active_probe"
	case result.ProbesPersisted > 0 && freshTraffic:
		result.EffectiveSource = "traffic+active_probe"
	case result.ProbesPersisted > 0:
		result.EffectiveSource = "active_probe"
	default:
		result.EffectiveSource = "traffic"
	}
	result.SourceErrors = uniqueStrings(result.SourceErrors)
	usable := trafficForScope || result.ProbesPersisted > 0 || freshTraffic || len(byAccount) == 0
	if options.StrictFallback && len(result.SourceErrors) > 0 && ((fallbackRequired && result.ProbesPersisted == 0) || !usable) {
		return Result{}, errors.New(strings.Join(result.SourceErrors, "；"))
	}
	return result, nil
}

func boolPointer(value bool) *bool {
	result := value
	return &result
}

func parsePolicy(policy map[string]any) (collectionPolicy, error) {
	traffic, err := object(policy, "traffic")
	if err != nil {
		return collectionPolicy{}, err
	}
	probePolicy, err := object(policy, "probe")
	if err != nil {
		return collectionPolicy{}, err
	}
	trafficEnabled, err := optionalBool(traffic, "enabled", true)
	if err != nil {
		return collectionPolicy{}, errors.New("traffic.enabled 配置无效")
	}
	source := "active_probe"
	if trafficEnabled {
		source = "traffic"
	}
	lookback, err := positiveInteger(traffic, "lookback_minutes", 120)
	if err != nil || lookback > 10080 {
		return collectionPolicy{}, errors.New("traffic.lookback_minutes 配置无效")
	}
	maxSamples, err := positiveInteger(traffic, "max_samples_per_account", 60)
	if err != nil || maxSamples > 200 {
		return collectionPolicy{}, errors.New("traffic.max_samples_per_account 配置无效")
	}
	trafficRefresh, err := positiveInteger(traffic, "refresh_seconds", 60)
	if err != nil || trafficRefresh > 86400 {
		return collectionPolicy{}, errors.New("traffic.refresh_seconds 配置无效")
	}
	trafficConcurrency, err := positiveInteger(probePolicy, "concurrency", 4)
	if err != nil || trafficConcurrency > 32 {
		return collectionPolicy{}, errors.New("probe.concurrency 配置无效")
	}
	probeEnabled, err := optionalBool(probePolicy, "enabled", true)
	if err != nil {
		return collectionPolicy{}, err
	}
	probeInterval, err := positiveInteger(probePolicy, "interval_seconds", 300)
	if err != nil || probeInterval < 30 || probeInterval > 86400 {
		return collectionPolicy{}, errors.New("probe.interval_seconds 配置无效")
	}
	skipFresh, err := optionalBool(probePolicy, "skip_when_traffic_fresh", true)
	if err != nil {
		return collectionPolicy{}, err
	}
	freshSeconds, err := positiveInteger(probePolicy, "traffic_fresh_seconds", 180)
	if err != nil || freshSeconds < 1 || freshSeconds > 86400 {
		return collectionPolicy{}, errors.New("probe.traffic_fresh_seconds 配置无效")
	}
	recoveryPolicy, err := object(policy, "recovery")
	if err != nil {
		return collectionPolicy{}, err
	}
	recoveryEnabled, err := optionalBool(recoveryPolicy, "enabled", true)
	if err != nil {
		return collectionPolicy{}, errors.New("recovery.enabled 配置无效")
	}
	recoveryInterval, err := positiveInteger(recoveryPolicy, "probe_interval_seconds", 180)
	if err != nil || recoveryInterval < 1 || recoveryInterval > 86400 {
		return collectionPolicy{}, errors.New("recovery.probe_interval_seconds 配置无效")
	}
	groupOverrides, err := parseProbeGroupOverrides(policy)
	if err != nil {
		return collectionPolicy{}, err
	}
	scope, err := object(policy, "scope")
	if err != nil {
		return collectionPolicy{}, err
	}
	managedGroupMode, _ := scope["managed_group_mode"].(string)
	managedGroupMode = strings.ToLower(strings.TrimSpace(managedGroupMode))
	if managedGroupMode == "" {
		managedGroupMode = "all"
	}
	if managedGroupMode != "all" && managedGroupMode != "selected" {
		return collectionPolicy{}, errors.New("scope.managed_group_mode 配置无效")
	}
	managedGroups, err := normalizedStringSet(scope, "managed_group_ids")
	if err != nil {
		return collectionPolicy{}, err
	}
	excludedGroups, err := normalizedStringSet(scope, "excluded_group_ids")
	if err != nil {
		return collectionPolicy{}, err
	}
	excludedAccounts, err := normalizedStringSet(scope, "excluded_account_ids")
	if err != nil {
		return collectionPolicy{}, err
	}
	accountTypes, err := normalizedStringSet(scope, "account_types")
	if err != nil {
		return collectionPolicy{}, err
	}
	platforms, err := normalizedStringSet(scope, "platforms")
	if err != nil {
		return collectionPolicy{}, err
	}
	return collectionPolicy{
		source: source, lookbackMinutes: lookback, maxSamples: maxSamples,
		trafficRefresh: time.Duration(trafficRefresh) * time.Second, trafficConcurrency: trafficConcurrency, probeEnabled: probeEnabled,
		probeInterval: time.Duration(probeInterval) * time.Second, skipFreshTraffic: skipFresh,
		recoveryEnabled: recoveryEnabled, recoveryInterval: time.Duration(recoveryInterval) * time.Second,
		trafficFreshWindow: time.Duration(freshSeconds) * time.Second, groupOverrides: groupOverrides,
		managedGroupMode: managedGroupMode, managedGroups: managedGroups, excludedGroups: excludedGroups,
		excludedAccounts: excludedAccounts, accountTypes: accountTypes, platforms: platforms,
	}, nil
}

type trafficFetchOutcome struct {
	accountID string
	rows      []map[string]any
	err       error
}

func fetchTrafficAccounts(ctx context.Context, admin Admin, accountIDs []string, lookbackMinutes, maxSamples, concurrency int) []trafficFetchOutcome {
	result := make([]trafficFetchOutcome, len(accountIDs))
	if len(accountIDs) == 0 {
		return result
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(accountIDs) {
		concurrency = len(accountIDs)
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				accountID := accountIDs[index]
				rows, err := admin.RequestDetails(ctx, accountID, lookbackMinutes, maxSamples)
				result[index] = trafficFetchOutcome{accountID: accountID, rows: rows, err: err}
			}
		}()
	}
	for index := range accountIDs {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			for pending := index; pending < len(accountIDs); pending++ {
				result[pending] = trafficFetchOutcome{accountID: accountIDs[pending], err: ctx.Err()}
			}
			return result
		}
	}
	close(jobs)
	workers.Wait()
	return result
}

func filterEvidenceTargets(targets []business.EvidenceTarget, policy collectionPolicy) []business.EvidenceTarget {
	result := make([]business.EvidenceTarget, 0, len(targets))
	for _, target := range targets {
		if _, excluded := policy.excludedAccounts[normalizedEvidenceValue(target.AccountID)]; excluded {
			continue
		}
		if len(policy.accountTypes) > 0 {
			if _, managed := policy.accountTypes[normalizedEvidenceValue(target.AccountType)]; !managed {
				continue
			}
		}
		if len(policy.platforms) > 0 {
			if _, managed := policy.platforms[normalizedEvidenceValue(target.Platform)]; !managed {
				continue
			}
		}
		keys := evidenceGroupKeys(target)
		if evidenceSetsOverlap(keys, policy.excludedGroups) {
			continue
		}
		if policy.managedGroupMode == "selected" && !evidenceSetsOverlap(keys, policy.managedGroups) {
			continue
		}
		if target.GroupID != nil {
			if override, found := policy.groupOverrides[strings.TrimSpace(*target.GroupID)]; found && override.enabled != nil && !*override.enabled {
				continue
			}
		}
		result = append(result, target)
	}
	return result
}

func normalizedStringSet(source map[string]any, key string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	raw, present := source[key]
	if !present {
		return result, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("scope.%s 配置无效", key)
	}
	for _, item := range items {
		value, ok := item.(string)
		value = normalizedEvidenceValue(value)
		if !ok || value == "" {
			return nil, fmt.Errorf("scope.%s 配置无效", key)
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func normalizedEvidenceValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func evidenceGroupKeys(target business.EvidenceTarget) map[string]struct{} {
	result := map[string]struct{}{normalizedEvidenceValue(target.GroupName): {}}
	if target.GroupID != nil {
		result[normalizedEvidenceValue(*target.GroupID)] = struct{}{}
	}
	return result
}

func evidenceSetsOverlap(left, right map[string]struct{}) bool {
	for value := range left {
		if _, found := right[value]; found {
			return true
		}
	}
	return false
}

func parseProbeGroupOverrides(policy map[string]any) (map[string]probeGroupOverride, error) {
	bindings, err := object(policy, "group_policy_bindings")
	if err != nil {
		return nil, err
	}
	result := make(map[string]probeGroupOverride, len(bindings))
	for groupID, raw := range bindings {
		binding, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("group_policy_bindings.%s 配置无效", groupID)
		}
		override := probeGroupOverride{}
		for field, target := range map[string]**bool{
			"enabled": &override.enabled, "probe_enabled": &override.probeEnabled, "recovery_enabled": &override.recoveryEnabled,
		} {
			if rawValue, present := binding[field]; present {
				value, ok := rawValue.(bool)
				if !ok {
					return nil, fmt.Errorf("group_policy_bindings.%s.%s 配置无效", groupID, field)
				}
				copy := value
				*target = &copy
			}
		}
		if rawInterval, present := binding["probe_interval_seconds"]; present {
			seconds, err := strictInteger(rawInterval)
			if err != nil || seconds < 30 || seconds > 86400 {
				return nil, fmt.Errorf("group_policy_bindings.%s.probe_interval_seconds 配置无效", groupID)
			}
			interval := time.Duration(seconds) * time.Second
			override.probeInterval = &interval
		}
		result[strings.TrimSpace(groupID)] = override
	}
	return result, nil
}

func convertTrafficRows(accountID string, memberships []business.EvidenceTarget, rows []map[string]any, cutoff time.Time, limit int) ([]business.TrafficSample, int) {
	ordered := append([]map[string]any{}, rows...)
	sort.SliceStable(ordered, func(left, right int) bool { return rowTime(ordered[left]).After(rowTime(ordered[right])) })
	result := []business.TrafficSample{}
	malformed, accepted := 0, 0
	for _, row := range ordered {
		if accepted >= limit {
			break
		}
		if textValue(row["account_id"]) != accountID {
			malformed++
			continue
		}
		requestID := strings.TrimSpace(textValue(row["request_id"]))
		if requestID == "" {
			malformed++
			continue
		}
		observed := rowTime(row)
		if observed.IsZero() || observed.Before(cutoff) {
			malformed++
			continue
		}
		kind, valid := rowKind(row)
		if !valid {
			malformed++
			continue
		}
		membership, found := primaryEvidenceMembership(memberships)
		if !found {
			malformed++
			continue
		}
		duration, valid := optionalDecimalText(row["duration_ms"])
		if !valid {
			malformed++
			continue
		}
		firstToken, valid := optionalDecimalText(row["first_token_ms"])
		if !valid {
			malformed++
			continue
		}
		accepted++
		var failureReason *string
		if kind == "error" {
			reason := safeText(row["message"])
			if reason == "" {
				reason = "请求失败"
			}
			failureReason = &reason
		}
		resultText := "通过"
		if kind == "error" {
			resultText = "失败"
		}
		payload := map[string]any{
			"request_id": requestID, "status_code": safeScalar(row["status_code"]), "phase": safeScalar(row["phase"]),
		}
		if duration != nil {
			payload["duration_ms"] = *duration
			payload["duration_unit"] = "ms"
			payload["latency_metric"] = "request_duration"
			payload["latency_source"] = "operations.duration_ms"
			payload["latency_unit"] = "ms"
		}
		if firstToken != nil {
			payload["first_token_ms"] = *firstToken
			payload["first_token_unit"] = "ms"
			payload["first_token_source"] = "operations.first_token_ms"
		}
		latency := duration
		if latency == nil && firstToken != nil {
			latency = firstToken
			payload["latency_metric"] = "first_token"
			payload["latency_source"] = "operations.first_token_ms"
			payload["latency_unit"] = "ms"
		}
		result = append(result, business.TrafficSample{
			AccountID: accountID, GroupName: membership.GroupName, Result: resultText,
			LatencyP50: latency, LatencyP95: latency, LatencyP99: latency, SampleCount: 1, Attempts: 1,
			FailureReason: failureReason, ObservedAt: observed.Format(time.RFC3339Nano), EvidenceKey: requestID,
			Payload: payload,
		})
	}
	return result, malformed
}

func primaryEvidenceMembership(memberships []business.EvidenceTarget) (business.EvidenceTarget, bool) {
	if len(memberships) == 0 {
		return business.EvidenceTarget{}, false
	}
	result := memberships[0]
	for _, candidate := range memberships[1:] {
		if evidenceMembershipLess(candidate, result) {
			result = candidate
		}
	}
	return result, true
}

func evidenceMembershipLess(left, right business.EvidenceTarget) bool {
	leftKey, rightKey := left.GroupName, right.GroupName
	if left.GroupID != nil && strings.TrimSpace(*left.GroupID) != "" {
		leftKey = strings.TrimSpace(*left.GroupID)
	}
	if right.GroupID != nil && strings.TrimSpace(*right.GroupID) != "" {
		rightKey = strings.TrimSpace(*right.GroupID)
	}
	leftID, leftErr := strconv.ParseUint(leftKey, 10, 64)
	rightID, rightErr := strconv.ParseUint(rightKey, 10, 64)
	if leftErr == nil && rightErr == nil && leftID != rightID {
		return leftID < rightID
	}
	if leftKey != rightKey {
		return leftKey < rightKey
	}
	return left.GroupName < right.GroupName
}

func dueProbeAccounts(targets []business.EvidenceTarget, policy collectionPolicy, now time.Time, monitoringUnavailable bool, forced map[string]struct{}) []string {
	byAccount := groupTargets(targets)
	result := []string{}
	for _, accountID := range sortedAccountIDs(byAccount) {
		memberships := byAccount[accountID]
		_, forcedAccount := forced[accountID]
		primary, found := primaryEvidenceMembership(memberships)
		if found && membershipProbeDue(primary, policy, now, monitoringUnavailable, forcedAccount) {
			result = append(result, accountID)
		}
	}
	return result
}

func membershipProbeDue(target business.EvidenceTarget, policy collectionPolicy, now time.Time, monitoringUnavailable, forced bool) bool {
	probeEnabled := policy.probeEnabled
	probeInterval := policy.probeInterval
	recoveryEnabled := policy.recoveryEnabled
	if target.GroupID != nil {
		if override, found := policy.groupOverrides[strings.TrimSpace(*target.GroupID)]; found {
			if override.enabled != nil && !*override.enabled {
				return false
			}
			if override.probeEnabled != nil {
				probeEnabled = *override.probeEnabled
			}
			if override.probeInterval != nil {
				probeInterval = *override.probeInterval
			}
			if override.recoveryEnabled != nil {
				recoveryEnabled = *override.recoveryEnabled
			}
		}
	}
	// Recovery follows the state that is actually active in Sub2API. A desired
	// decision may have failed to write and cannot put a healthy account into
	// the recovery path.
	state := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(target.EffectiveState)), "-", "_")
	fused := state == "fused" || state == "hard_open" || state == "soft_open"
	interval := probeInterval
	if fused {
		if !recoveryEnabled {
			return false
		}
		interval = policy.recoveryInterval
	} else if !probeEnabled {
		return false
	}
	if target.ProbeAt != nil && now.Sub(target.ProbeAt.UTC()) < interval {
		return false
	}
	if fused {
		return true
	}
	latestSample := latestTime(target.TrafficAt, target.ProbeAt)
	trafficFresh := latestSample != nil && now.Sub(latestSample.UTC()) <= policy.trafficFreshWindow
	return policy.source == "active_probe" || monitoringUnavailable || forced || (policy.source == "traffic" && (!policy.skipFreshTraffic || !trafficFresh))
}

func trafficFetchDue(targets []business.EvidenceTarget, now time.Time, interval time.Duration) bool {
	for _, target := range targets {
		if target.TrafficFetchAt != nil && now.Sub(target.TrafficFetchAt.UTC()) < interval {
			return false
		}
	}
	return true
}

func latestTime(values ...*time.Time) *time.Time {
	var latest *time.Time
	for _, value := range values {
		if value == nil || latest != nil && !value.After(*latest) {
			continue
		}
		copy := value.UTC()
		latest = &copy
	}
	return latest
}

func groupTargets(targets []business.EvidenceTarget) map[string][]business.EvidenceTarget {
	result := map[string][]business.EvidenceTarget{}
	for _, target := range targets {
		result[target.AccountID] = append(result[target.AccountID], target)
	}
	return result
}

func sortedAccountIDs(values map[string][]business.EvidenceTarget) []string {
	result := make([]string, 0, len(values))
	for accountID := range values {
		result = append(result, accountID)
	}
	sort.Slice(result, func(left, right int) bool {
		leftValue, leftErr := strconv.ParseUint(result[left], 10, 64)
		rightValue, rightErr := strconv.ParseUint(result[right], 10, 64)
		if leftErr == nil && rightErr == nil {
			return leftValue < rightValue
		}
		return result[left] < result[right]
	})
	return result
}

func rowGroups(memberships []business.EvidenceTarget, row map[string]any) []business.EvidenceTarget {
	raw, present := row["group_id"]
	if !present || raw == nil {
		return memberships
	}
	groupID := strings.TrimSpace(textValue(raw))
	if groupID == "" {
		return nil
	}
	result := []business.EvidenceTarget{}
	for _, membership := range memberships {
		if membership.GroupID != nil && *membership.GroupID == groupID {
			result = append(result, membership)
		}
	}
	return result
}

func rowKind(row map[string]any) (string, bool) {
	if raw, present := row["kind"]; present {
		value, ok := raw.(string)
		value = strings.ToLower(strings.TrimSpace(value))
		return value, ok && (value == "success" || value == "error")
	}
	if raw, present := row["status_code"]; present {
		status, err := strictInteger(raw)
		if err != nil {
			return "", false
		}
		if status >= 400 {
			return "error", true
		}
		return "success", true
	}
	return "success", true
}

func rowTime(row map[string]any) time.Time {
	value, ok := row["created_at"].(string)
	if !ok {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func optionalDecimalText(raw any) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	if _, bad := raw.(bool); bad {
		return nil, false
	}
	text := strings.TrimSpace(textValue(raw))
	if text == "" {
		return nil, false
	}
	if _, err := strconv.ParseFloat(text, 64); err != nil {
		return nil, false
	}
	return &text, true
}

func applyFreshTraffic(targets []business.EvidenceTarget, samples []business.TrafficSample) {
	latest := map[string]time.Time{}
	for _, sample := range samples {
		observed, err := time.Parse(time.RFC3339Nano, sample.ObservedAt)
		if err != nil {
			continue
		}
		key := sample.AccountID + "\x00" + sample.GroupName
		if previous, found := latest[key]; !found || observed.After(previous) {
			latest[key] = observed
		}
	}
	for index := range targets {
		if observed, found := latest[targets[index].AccountID+"\x00"+targets[index].GroupName]; found {
			copy := observed.UTC()
			targets[index].TrafficAt = &copy
		}
	}
}

func hasFreshTraffic(targets []business.EvidenceTarget, now time.Time, window time.Duration) bool {
	for _, target := range targets {
		if target.TrafficAt != nil && now.Sub(*target.TrafficAt) <= window {
			return true
		}
	}
	return false
}

func object(source map[string]any, key string) (map[string]any, error) {
	raw, present := source[key]
	if !present {
		return map[string]any{}, nil
	}
	result, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New(key + " 配置无效")
	}
	return result, nil
}

func positiveInteger(source map[string]any, key string, fallback int) (int, error) {
	value, err := integerValue(source, key, fallback)
	if err != nil || value < 1 {
		return 0, errors.New(strings.TrimSuffix(key, "_minutes") + " 配置无效")
	}
	return value, nil
}

func integerValue(source map[string]any, key string, fallback int) (int, error) {
	raw, present := source[key]
	if !present {
		return fallback, nil
	}
	return strictInteger(raw)
}

func strictInteger(raw any) (int, error) {
	if raw == nil {
		return 0, errors.New("不是整数")
	}
	switch value := raw.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case json.Number:
		return strconv.Atoi(value.String())
	case float64:
		if value != float64(int(value)) {
			return 0, errors.New("不是整数")
		}
		return int(value), nil
	default:
		return 0, errors.New("不是整数")
	}
}

func optionalBool(source map[string]any, key string, fallback bool) (bool, error) {
	raw, present := source[key]
	if !present {
		return fallback, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, errors.New(key + " 配置无效")
	}
	return value, nil
}

func safeScalar(raw any) any {
	switch raw.(type) {
	case nil, string, bool, json.Number, int, int64, float64:
		return raw
	default:
		return nil
	}
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func safeText(value any) string {
	text := textValue(value)
	text = redact.Secrets(text)
	runes := []rune(text)
	if len(runes) > 500 {
		return string(runes[:500])
	}
	return text
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return safeText(err.Error())
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
