package routing

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Event string

const (
	EventHealthy       Event = "healthy"
	EventSlow          Event = "slow"
	EventUnknown       Event = "unknown_upstream_error"
	EventGateway       Event = "gateway_error"
	EventRateLimited   Event = "rate_limited_or_exhausted"
	EventProbeFailed   Event = "probe_failed"
	EventCredentialBad Event = "credential_invalid"
)

type Sample struct {
	Result        string
	FailureReason string
	Source        string
	LatencyP95    *string
	StatusCode    *int
	Payload       map[string]any
}

type Classified struct {
	Score       float64
	Event       Event
	Fatal       bool
	Failure     bool
	Gateway     bool
	RateLimited bool
}

type Health struct {
	ShortScore         float64
	LongScore          float64
	HealthScore        float64
	SampleCount        int
	LatestEvent        Event
	Fatal              bool
	FailureStreak      int
	RecoveryPassStreak int
	P50MS              *float64
	P95MS              *float64
	Events             []Event
	GatewayFailures    int
	RateLimited        int
}

type scoringConfig struct {
	shortWindow   int
	longWindow    int
	latestWeight  float64
	shortRatio    float64
	slowTTFBMS    int
	eventScores   map[Event]float64
	fatalPatterns []string
	quotaPatterns []string
	gatewayCodes  map[int]struct{}
}

var statusPattern = regexp.MustCompile(`\b([45][0-9]{2})\b`)

var defaultFatalPatterns = []string{
	"invalid api key", "unauthorized", "forbidden", "authentication", "account not found",
	"no api key", "no access token", "insufficient", "balance", "quota exceeded",
	"usage limit", "credit", "expired",
}

var defaultQuotaPatterns = []string{
	"usage limit", "usage_limit", "quota", "rate limit", "rate_limit",
	"insufficient", "balance", "credit", "billing", "too many requests",
	"exceeded your current", "resource_exhausted", "resource exhausted", "overloaded",
}

var authFailurePatterns = []string{
	"invalid api key", "invalid_api_key", "invalid key", "invalid token", "unauthorized",
	"authentication", "authentication_error", "forbidden", "permission denied", "access denied",
	"account not found", "no api key", "no access token", "revoked", "disabled key", "key not found",
}

func HealthScore(samples []Sample, policy map[string]any) (Health, error) {
	config, err := parseScoringConfig(policy)
	if err != nil {
		return Health{}, err
	}
	if len(samples) > config.longWindow {
		samples = samples[:config.longWindow]
	}
	if len(samples) == 0 {
		return Health{LatestEvent: EventUnknown, Events: []Event{}}, nil
	}
	classified := make([]Classified, len(samples))
	for index := range samples {
		classified[index] = classify(samples[index], config)
	}
	shortCount := min(len(classified), config.shortWindow)
	short := classified[:shortCount]
	shortValue := short[0].Score
	if len(short) > 1 {
		decaySum := 0.0
		for index := 0; index < len(short)-1; index++ {
			decaySum += math.Pow(0.5, float64(index))
		}
		shortValue = short[0].Score * config.latestWeight
		for index := 1; index < len(short); index++ {
			weight := (1 - config.latestWeight) * math.Pow(0.5, float64(index-1)) / decaySum
			shortValue += short[index].Score * weight
		}
	}
	longCount := min(len(classified), config.longWindow)
	longValue := 0.0
	for _, item := range classified[:longCount] {
		longValue += item.Score
	}
	longValue /= float64(longCount)
	final := clamp(shortValue*config.shortRatio+longValue*(1-config.shortRatio), 0, 100)
	if classified[0].Fatal {
		final = classified[0].Score
	}

	failureStreak := 0
	for _, item := range classified {
		if !item.Failure {
			break
		}
		failureStreak++
	}
	recoveryStreak := 0
	for _, item := range classified {
		if item.Failure {
			break
		}
		recoveryStreak++
	}
	latencies := make([]float64, 0, longCount)
	for index, item := range classified[:longCount] {
		if item.Failure {
			continue
		}
		if latency := latencyMS(samples[index]); latency != nil {
			latencies = append(latencies, *latency)
		}
	}
	var p50, p95 *float64
	if len(latencies) > 0 {
		sort.Float64s(latencies)
		p50Value := latencies[max(0, int(math.Ceil(0.50*float64(len(latencies))))-1)]
		p95Value := latencies[max(0, int(math.Ceil(0.95*float64(len(latencies))))-1)]
		p50, p95 = &p50Value, &p95Value
	}
	events := make([]Event, longCount)
	gatewayFailures, rateLimited := 0, 0
	for index, item := range classified[:longCount] {
		events[index] = item.Event
		if index < 5 && item.Gateway {
			gatewayFailures++
		}
		if index < 5 && item.RateLimited {
			rateLimited++
		}
	}
	return Health{
		ShortScore: round4(shortValue), LongScore: round4(longValue), HealthScore: round4(final),
		SampleCount: len(classified), LatestEvent: classified[0].Event, Fatal: classified[0].Fatal,
		FailureStreak: failureStreak, RecoveryPassStreak: recoveryStreak, P50MS: roundedPointer(p50), P95MS: roundedPointer(p95),
		Events: events, GatewayFailures: gatewayFailures, RateLimited: rateLimited,
	}, nil
}

func ClassifySample(sample Sample, policy map[string]any) (Classified, error) {
	config, err := parseScoringConfig(policy)
	if err != nil {
		return Classified{}, err
	}
	return classify(sample, config), nil
}

func classify(sample Sample, config scoringConfig) Classified {
	result := normalizeReason(sample.Result)
	reason := normalizeReason(sample.FailureReason)
	text := strings.TrimSpace(result + " " + reason)
	status := sampleStatus(sample, text)
	success := result == "通过" || result == "passed" || result == "pass" || result == "success" ||
		result == "succeeded" || result == "healthy" || result == "ok"
	quotaStatus := status == 402 || status == 429
	quotaText := containsPattern(text, config.quotaPatterns) && !containsPattern(text, authFailurePatterns)
	quota := !success && (quotaStatus || quotaText)
	credential := status == 401 || status == 402 || status == 403 || containsPattern(text, config.fatalPatterns)
	_, gateway := config.gatewayCodes[status]
	failed := result == "失败" || result == "failed" || result == "error" || result == "timeout" ||
		result == "超时" || result == "probe failed" || result == "unhealthy" || reason != ""
	if quota {
		return Classified{Score: config.eventScores[EventRateLimited], Event: EventRateLimited, Failure: true, RateLimited: true}
	}
	if credential {
		return Classified{Score: config.eventScores[EventCredentialBad], Event: EventCredentialBad, Fatal: true, Failure: true}
	}
	if success {
		if latency := latencyMS(sample); latency != nil && *latency > float64(config.slowTTFBMS) {
			return Classified{Score: config.eventScores[EventSlow], Event: EventSlow}
		}
		return Classified{Score: config.eventScores[EventHealthy], Event: EventHealthy}
	}
	if gateway {
		return Classified{Score: config.eventScores[EventGateway], Event: EventGateway, Failure: true, Gateway: true}
	}
	if failed {
		if looksLikeNetworkFailure(text) {
			return Classified{Score: config.eventScores[EventProbeFailed], Event: EventProbeFailed, Failure: true}
		}
		return Classified{Score: config.eventScores[EventUnknown], Event: EventUnknown, Failure: true}
	}
	return Classified{Score: config.eventScores[EventUnknown], Event: EventUnknown, Failure: true}
}

func looksLikeNetworkFailure(value string) bool {
	for _, marker := range []string{
		"timeout", "deadline exceeded", "connection refused", "connection reset",
		"no such host", "eof", "broken pipe", "network is unreachable", "tls", "dial tcp", "canceled",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func parseScoringConfig(policy map[string]any) (scoringConfig, error) {
	scoring, err := optionalObject(policy, "scoring")
	if err != nil {
		return scoringConfig{}, err
	}
	classifySection, err := optionalObject(policy, "classify")
	if err != nil {
		return scoringConfig{}, err
	}
	if _, present := scoring["events"]; present {
		return scoringConfig{}, errors.New("scoring.events 已废弃，请使用 scoring.event_scores")
	}
	if _, present := scoring["error_classification"]; present {
		return scoringConfig{}, errors.New("scoring.error_classification 已废弃，请使用 classify")
	}
	shortWindow, err := integerField(scoring, "short_window", 10, 1, 10000)
	if err != nil {
		return scoringConfig{}, err
	}
	longWindow, err := integerField(scoring, "long_window", 60, 1, 100000)
	if err != nil {
		return scoringConfig{}, err
	}
	if longWindow < shortWindow {
		return scoringConfig{}, errors.New("scoring.long_window 不能小于 scoring.short_window")
	}
	latestWeight, err := numberField(scoring, "latest_weight", 0.5, math.SmallestNonzeroFloat64, 1)
	if err != nil {
		return scoringConfig{}, err
	}
	shortRatio, err := numberField(scoring, "short_ratio", 0.7, math.SmallestNonzeroFloat64, 1)
	if err != nil {
		return scoringConfig{}, err
	}
	slowTTFB, err := integerField(scoring, "slow_ttfb_ms", 5000, 1, 3_600_000)
	if err != nil {
		return scoringConfig{}, err
	}
	eventScores, err := configuredEventScores(scoring)
	if err != nil {
		return scoringConfig{}, err
	}
	fatalPatterns := append([]string{}, defaultFatalPatterns...)
	if raw, present := classifySection["fatal_patterns"]; present {
		fatalPatterns, err = stringList(raw, "classify.fatal_patterns")
		if err != nil {
			return scoringConfig{}, err
		}
	}
	quotaPatterns := append([]string{}, defaultQuotaPatterns...)
	gatewayCodes := map[int]struct{}{429: {}, 500: {}, 502: {}, 503: {}, 504: {}}
	if raw, present := classifySection["gateway_status_codes"]; present {
		gatewayCodes, err = statusCodes(raw)
		if err != nil {
			return scoringConfig{}, err
		}
		if len(gatewayCodes) == 0 {
			gatewayCodes = map[int]struct{}{429: {}, 500: {}, 502: {}, 503: {}, 504: {}}
		}
	}
	return scoringConfig{
		shortWindow: shortWindow, longWindow: longWindow, latestWeight: latestWeight, shortRatio: shortRatio,
		slowTTFBMS: slowTTFB, eventScores: eventScores,
		fatalPatterns: fatalPatterns, quotaPatterns: quotaPatterns, gatewayCodes: gatewayCodes,
	}, nil
}

func configuredEventScores(scoring map[string]any) (map[Event]float64, error) {
	defaults := map[Event]float64{
		EventHealthy: 100, EventSlow: 65, EventUnknown: 40, EventGateway: 25,
		EventRateLimited: 15, EventProbeFailed: 10, EventCredentialBad: 0,
	}
	raw, present := scoring["event_scores"]
	if !present {
		return defaults, nil
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("scoring.event_scores 必须是对象")
	}
	fields := map[Event]string{
		EventHealthy: "perfect", EventSlow: "slow_ttfb", EventUnknown: "upstream_unknown", EventGateway: "gateway_error",
		EventRateLimited: "quota_exhausted", EventProbeFailed: "probe_fail", EventCredentialBad: "fatal",
	}
	for event, key := range fields {
		value, found := values[key]
		if !found {
			continue
		}
		parsed, err := strictNumber(value)
		minimum := 0.0
		if event == EventRateLimited {
			minimum = 1
		}
		if err != nil || parsed < minimum || parsed > 100 {
			return nil, fmt.Errorf("scoring.event_scores.%s 必须在 %g 到 100 之间", key, minimum)
		}
		defaults[event] = parsed
	}
	return defaults, nil
}

func sampleStatus(sample Sample, text string) int {
	if sample.StatusCode != nil {
		return *sample.StatusCode
	}
	if sample.Payload != nil {
		if raw, present := sample.Payload["status_code"]; present {
			value, err := strictInteger(raw)
			if err == nil {
				return value
			}
			return 0
		}
	}
	match := statusPattern.FindStringSubmatch(text)
	if len(match) == 2 {
		value, _ := strconv.Atoi(match[1])
		return value
	}
	return 0
}

func latencyMS(sample Sample) *float64 {
	if sample.LatencyP95 == nil || strings.TrimSpace(*sample.LatencyP95) == "" {
		return nil
	}
	source := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(sample.Source)), "_", "-")
	trustedProbe := source == "active-probe" || source == "probe"
	metric, ok := sample.Payload["latency_metric"].(string)
	metric = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(metric)), "-", "_")
	if !trustedProbe && (!ok || (metric != "first_token" && metric != "ttfb")) {
		return nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(*sample.LatencyP95), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return nil
	}
	if sample.Payload != nil {
		if raw, present := sample.Payload["latency_unit"]; present {
			unit, ok := raw.(string)
			if !ok {
				return nil
			}
			switch strings.ToLower(strings.TrimSpace(unit)) {
			case "ms", "millisecond", "milliseconds":
				return &value
			case "s", "second", "seconds":
				value *= 1000
				return &value
			default:
				return nil
			}
		}
	}
	if source == "active-probe" || source == "probe" || source == "traffic" || source == "logs" {
		return &value
	}
	if value <= 100 {
		value *= 1000
	}
	return &value
}

func optionalObject(source map[string]any, key string) (map[string]any, error) {
	raw, present := source[key]
	if !present {
		return map[string]any{}, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New(key + " 必须是对象")
	}
	return value, nil
}

func integerField(source map[string]any, key string, fallback, minimum, maximum int) (int, error) {
	raw, present := source[key]
	if !present {
		return fallback, nil
	}
	value, err := strictInteger(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, errors.New("scoring." + key + " 配置无效")
	}
	return value, nil
}

func numberField(source map[string]any, key string, fallback, minimum, maximum float64) (float64, error) {
	raw, present := source[key]
	if !present {
		return fallback, nil
	}
	value, err := strictNumber(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, errors.New("scoring." + key + " 配置无效")
	}
	return value, nil
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
		parsed, err := strconv.Atoi(value.String())
		return parsed, err
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) {
			return 0, errors.New("不是整数")
		}
		return int(value), nil
	default:
		return 0, errors.New("不是整数")
	}
}

func strictNumber(raw any) (float64, error) {
	if raw == nil {
		return 0, errors.New("不是数字")
	}
	var value float64
	var err error
	switch current := raw.(type) {
	case int:
		value = float64(current)
	case int64:
		value = float64(current)
	case float64:
		value = current
	case json.Number:
		value, err = current.Float64()
	default:
		return 0, errors.New("不是数字")
	}
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("不是有限数字")
	}
	return value, nil
}

func stringList(raw any, field string) ([]string, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, errors.New(field + " 必须是数组")
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, errors.New(field + " 只能包含非空字符串")
		}
		result = append(result, strings.ToLower(strings.TrimSpace(value)))
	}
	return result, nil
}

func statusCodes(raw any) (map[int]struct{}, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, errors.New("classify.gateway_status_codes 必须是数组")
	}
	result := map[int]struct{}{}
	for _, item := range items {
		value, err := strictInteger(item)
		if err != nil || value < 100 || value > 599 {
			return nil, errors.New("classify.gateway_status_codes 包含无效状态码")
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func containsPattern(text string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func normalizeReason(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	return strings.ReplaceAll(value, "-", " ")
}

func round4(value float64) float64           { return math.Round(value*10000) / 10000 }
func clamp(value, low, high float64) float64 { return math.Max(low, math.Min(high, value)) }

func roundedPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	result := round4(*value)
	return &result
}
