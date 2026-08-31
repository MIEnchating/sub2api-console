package routing

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func testPolicy() map[string]any {
	return map[string]any{
		"scoring": map[string]any{
			"short_window": int64(10), "long_window": int64(60), "latest_weight": 0.5,
			"short_ratio": 0.7, "slow_ttfb_ms": int64(5000),
			"event_scores": map[string]any{
				"perfect": int64(100), "slow_ttfb": int64(65), "upstream_unknown": int64(40),
				"gateway_error": int64(25), "quota_exhausted": int64(15), "probe_fail": int64(10), "fatal": int64(0),
			},
		},
		"classify": map[string]any{
			"fatal_patterns":       []any{"invalid api key", "unauthorized", "authentication"},
			"gateway_status_codes": []any{int64(429), int64(500), int64(502), int64(503), int64(504)},
		},
	}
}

func classifySampleForTest(sample Sample, policy map[string]any) (Classified, error) {
	config, err := parseScoringConfig(policy)
	if err != nil {
		return Classified{}, err
	}
	return classify(sample, config), nil
}

func TestClassificationPriorityAndScores(t *testing.T) {
	status401, status402, status429, status502 := 401, 402, 429, 502
	tests := []struct {
		name   string
		sample Sample
		event  Event
		score  float64
		fatal  bool
	}{
		{"credential beats quota text", Sample{Result: "失败", FailureReason: "quota unauthorized", StatusCode: &status401}, EventCredentialBad, 0, true},
		{"explicit quota status beats auth text", Sample{Result: "失败", FailureReason: "unauthorized", StatusCode: &status402}, EventRateLimited, 15, false},
		{"quota beats gateway list", Sample{Result: "失败", FailureReason: "rate limit", StatusCode: &status429}, EventRateLimited, 15, false},
		{"gateway", Sample{Result: "失败", StatusCode: &status502}, EventGateway, 25, false},
		{"probe failure", Sample{Result: "timeout"}, EventProbeFailed, 10, false},
		{"healthy", Sample{Result: "通过"}, EventHealthy, 100, false},
		{"unknown empty result", Sample{}, EventUnknown, 40, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := classifySampleForTest(test.sample, testPolicy())
			if err != nil {
				t.Fatal(err)
			}
			if result.Event != test.event || result.Score != test.score || result.Fatal != test.fatal {
				t.Fatalf("classification=%#v", result)
			}
		})
	}
}

func TestPercentilesIgnoreZeroLatencySamples(t *testing.T) {
	zero, oneHundred := "0", "100"
	health, err := HealthScore([]Sample{
		{Result: "通过", Source: "traffic", LatencyP95: &zero, Payload: map[string]any{"latency_metric": "first_token", "latency_unit": "ms"}},
		{Result: "通过", Source: "traffic", LatencyP95: &oneHundred, Payload: map[string]any{"latency_metric": "first_token", "latency_unit": "ms"}},
	}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.P50MS == nil || *health.P50MS != 100 || health.P95MS == nil || *health.P95MS != 100 {
		t.Fatalf("zero latency must not enter percentiles: %#v", health)
	}
}

func TestActiveProbeFirstContentLatencyEntersFirstTokenPercentiles(t *testing.T) {
	firstToken, legacyFirstToken := "1250", "1500"
	health, err := HealthScore([]Sample{
		{
			Result: "通过", Source: "active-probe", LatencyP95: &firstToken,
			Payload: map[string]any{
				"latency_metric": "first_token", "latency_source": "account_test.first_content", "latency_unit": "ms",
			},
		},
		{Result: "通过", Source: "active-probe", LatencyP95: &legacyFirstToken},
	}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.LatestEvent != EventHealthy || health.P50MS == nil || *health.P50MS != 1250 || health.P95MS == nil || *health.P95MS != 1500 {
		t.Fatalf("主动探针首个内容事件耗时没有进入首字延迟：%#v", health)
	}
}

func TestTrafficRequestDurationEntersCombinedLatencyPercentiles(t *testing.T) {
	duration := "195843"
	health, err := HealthScore([]Sample{{
		Result: "通过", Source: "traffic", LatencyP95: &duration,
		Payload: map[string]any{
			"latency_metric": "ttfb", "latency_source": "operations.duration_ms", "latency_unit": "ms",
		},
	}}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.LatestEvent != EventSlow || health.P50MS == nil || *health.P50MS != 195843 || health.P95MS == nil || *health.P95MS != 195843 {
		t.Fatalf("真实流量总耗时没有进入综合延迟：%#v", health)
	}
}

func TestCompleteProbeResponseDoesNotEnterCombinedLatency(t *testing.T) {
	duration := "195843"
	health, err := HealthScore([]Sample{{
		Result: "通过", Source: "active-probe", LatencyP95: &duration,
		Payload: map[string]any{
			"latency_metric": "total_duration", "latency_source": "account_test.complete_response", "latency_unit": "ms",
		},
	}}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.LatestEvent != EventHealthy || health.P50MS != nil || health.P95MS != nil {
		t.Fatalf("完整探针响应耗时不应重复进入综合延迟：%#v", health)
	}
}

func TestCombinedLatencyMixesProbeFirstContentAndTrafficDuration(t *testing.T) {
	firstContent, requestDuration := "1250", "9000"
	health, err := HealthScore([]Sample{
		{Result: "通过", Source: "active-probe", LatencyP95: &firstContent, Payload: map[string]any{
			"latency_metric": "first_token", "latency_source": "account_test.first_content", "latency_unit": "ms",
		}},
		{Result: "通过", Source: "traffic", LatencyP95: &requestDuration, Payload: map[string]any{
			"latency_metric": "request_duration", "latency_source": "operations.duration_ms", "latency_unit": "ms",
		}},
	}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.P50MS == nil || *health.P50MS != 1250 || health.P95MS == nil || *health.P95MS != 9000 {
		t.Fatalf("combined latency=%#v", health)
	}
}

func TestQuotaRemainsRecoverableWithLegacyFatalPatterns(t *testing.T) {
	policy := testPolicy()
	policy["classify"].(map[string]any)["fatal_patterns"] = []any{
		"invalid api key", "unauthorized", "insufficient", "balance", "quota exceeded", "credit",
	}
	status402 := 402
	tests := []struct {
		name  string
		input Sample
		want  Event
		fatal bool
	}{
		{"balance shortage", Sample{Result: "失败", FailureReason: "insufficient balance"}, EventRateLimited, false},
		{"billing status", Sample{Result: "失败", FailureReason: "credit required", StatusCode: &status402}, EventRateLimited, false},
		{"auth still wins", Sample{Result: "失败", FailureReason: "unauthorized: credit exhausted"}, EventCredentialBad, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := classifySampleForTest(test.input, policy)
			if err != nil {
				t.Fatal(err)
			}
			if got.Event != test.want || got.Fatal != test.fatal {
				t.Fatalf("classification=%#v", got)
			}
		})
	}
}

func TestHealthUsesLatestWeightGeometricDecayAndLongAverage(t *testing.T) {
	policy := testPolicy()
	policy["scoring"].(map[string]any)["short_window"] = int64(3)
	policy["scoring"].(map[string]any)["long_window"] = int64(4)
	samples := []Sample{{Result: "通过"}, {Result: "失败"}, {Result: "通过"}, {Result: "失败"}}

	health, err := HealthScore(samples, policy)
	if err != nil {
		t.Fatal(err)
	}
	// Ordinary upstream failures score 40; only network/timeout failures score 10.
	// Short = 100*0.5 + 40*(0.5/1.5) + 100*(0.25/1.5) = 80.
	// Long = 70. Final = 80*0.7 + 70*0.3 = 77.
	if health.ShortScore != 80 || health.LongScore != 70 || health.HealthScore != 77 {
		t.Fatalf("health=%#v", health)
	}
	if health.FailureStreak != 0 || health.RecoveryPassStreak != 1 || health.SampleCount != 4 {
		t.Fatalf("streaks=%#v", health)
	}
}

func TestLatestFatalIsOneVoteVeto(t *testing.T) {
	status := 401
	health, err := HealthScore([]Sample{{Result: "失败", StatusCode: &status}, {Result: "通过"}, {Result: "通过"}}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !health.Fatal || health.HealthScore != 0 || health.LatestEvent != EventCredentialBad {
		t.Fatalf("health=%#v", health)
	}
}

func TestLatestFatalUsesConfiguredEventScore(t *testing.T) {
	policy := testPolicy()
	policy["scoring"].(map[string]any)["event_scores"].(map[string]any)["fatal"] = 7.5
	status := 401
	health, err := HealthScore([]Sample{{Result: "失败", StatusCode: &status}, {Result: "通过"}}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !health.Fatal || health.HealthScore != 7.5 || health.LatestEvent != EventCredentialBad {
		t.Fatalf("configured fatal score did not become the final score: %#v", health)
	}
}

func TestSuccessfulSamplesSkipQuotaAndGatewayClassification(t *testing.T) {
	for _, status := range []int{429, 502} {
		classified, err := classifySampleForTest(Sample{
			Result: "通过", FailureReason: "rate limit", StatusCode: &status,
		}, testPolicy())
		if err != nil {
			t.Fatal(err)
		}
		if classified.Event != EventHealthy || classified.Failure || classified.Fatal {
			t.Fatalf("successful status %d was classified as an error: %#v", status, classified)
		}
	}
}

func TestNoSamplesScoreZeroAndSingleSampleKeepsFullScore(t *testing.T) {
	empty, err := HealthScore(nil, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if empty.HealthScore != 0 || empty.LatestEvent != EventUnknown || len(empty.Events) != 0 {
		t.Fatalf("empty=%#v", empty)
	}
	single, err := HealthScore([]Sample{{Result: "通过"}}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if single.ShortScore != 100 || single.LongScore != 100 || single.HealthScore != 100 {
		t.Fatalf("single=%#v", single)
	}
}

func TestHealthStrictlyLimitsScoringToLongWindow(t *testing.T) {
	policy := testPolicy()
	policy["scoring"].(map[string]any)["long_window"] = int64(60)
	samples := make([]Sample, 63)
	for index := range samples {
		samples[index] = Sample{Result: "通过"}
	}
	for index := 60; index < len(samples); index++ {
		samples[index] = Sample{Result: "失败"}
	}

	health, err := HealthScore(samples, policy)
	if err != nil {
		t.Fatal(err)
	}
	if health.SampleCount != 60 || health.LongScore != 100 || health.HealthScore != 100 {
		t.Fatalf("health must use exactly the latest long window: %#v", health)
	}
}

func TestHealthUsesAvailableSamplesWhenLongWindowIsNotFull(t *testing.T) {
	policy := testPolicy()
	policy["scoring"].(map[string]any)["long_window"] = int64(60)
	samples := make([]Sample, 8)
	for index := range samples {
		samples[index] = Sample{Result: "通过"}
	}

	health, err := HealthScore(samples, policy)
	if err != nil {
		t.Fatal(err)
	}
	if health.SampleCount != 8 || health.ShortScore != 100 || health.LongScore != 100 || health.HealthScore != 100 {
		t.Fatalf("an incomplete window must use its actual samples without padding: %#v", health)
	}
}

func TestLatencyUnitsAndPercentiles(t *testing.T) {
	values := []string{"100", "200", "300", "400"}
	samples := make([]Sample, len(values))
	for index := range values {
		samples[index] = Sample{
			Result: "通过", Source: "traffic", LatencyP95: &values[index],
			Payload: map[string]any{"latency_unit": "ms", "latency_metric": "first_token"},
		}
	}
	health, err := HealthScore(samples, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.P50MS == nil || *health.P50MS != 200 || health.P95MS == nil || *health.P95MS != 400 {
		t.Fatalf("latencies=%#v", health)
	}
}

func TestTrafficDurationIsClassifiedAsSlowCombinedLatency(t *testing.T) {
	duration := "195843"
	health, err := HealthScore([]Sample{{
		Result: "通过", Source: "traffic", LatencyP95: &duration,
		Payload: map[string]any{
			"latency_unit": "ms", "latency_metric": "request_duration",
			"latency_source": "operations.duration_ms", "duration_ms": duration,
		},
	}}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.LatestEvent != EventSlow || health.HealthScore != 65 || health.P95MS == nil || *health.P95MS != 195843 {
		t.Fatalf("真实流量总耗时没有参与综合延迟评分：%#v", health)
	}
}

func TestExplicitTrafficFirstTokenIsClassifiedAsSlow(t *testing.T) {
	firstToken := "9000"
	health, err := HealthScore([]Sample{{
		Result: "通过", Source: "traffic", LatencyP95: &firstToken,
		Payload: map[string]any{"latency_unit": "ms", "latency_metric": "first_token"},
	}}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.LatestEvent != EventSlow || health.HealthScore != 65 || health.P95MS == nil || *health.P95MS != 9000 {
		t.Fatalf("明确的首字延迟没有参与评分：%#v", health)
	}
}

func TestAccountStateLatencyIsNotClassifiedAsFirstToken(t *testing.T) {
	latency := "15.4"
	health, err := HealthScore([]Sample{{
		Result: "未取到日志", Source: "account-state", LatencyP95: &latency,
		Payload: map[string]any{},
	}}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.P95MS != nil {
		t.Fatalf("账号状态耗时被错误当成首字延迟：%#v", health)
	}
}

func TestMalformedPolicyFailsClosed(t *testing.T) {
	tests := []map[string]any{
		{"scoring": nil},
		{"scoring": map[string]any{"latest_weight": 2.0}},
		{"scoring": map[string]any{"short_window": json.Number("1.5")}},
		{"classify": map[string]any{"gateway_status_codes": []any{true}}},
		{"classify": map[string]any{"fatal_patterns": []any{"ok", nil}}},
	}
	for _, policy := range tests {
		if _, err := HealthScore([]Sample{{Result: "通过"}}, policy); err == nil {
			t.Fatalf("policy should fail: %#v", policy)
		}
	}
}

func TestQuotaScoreZeroIsRejectedByExecutionConsumer(t *testing.T) {
	policy := testPolicy()
	policy["scoring"].(map[string]any)["event_scores"].(map[string]any)["quota_exhausted"] = int64(0)
	_, err := HealthScore([]Sample{{Result: "通过"}}, policy)
	if err == nil || !strings.Contains(err.Error(), "quota_exhausted") {
		t.Fatalf("execution consumer accepted a fatal quota score: %v", err)
	}
}

func TestScoringConsumerRejectsShortWindowAboveLongWindow(t *testing.T) {
	policy := testPolicy()
	policy["scoring"].(map[string]any)["short_window"] = int64(61)
	policy["scoring"].(map[string]any)["long_window"] = int64(60)
	_, err := HealthScore([]Sample{{Result: "通过"}}, policy)
	if err == nil || !strings.Contains(err.Error(), "long_window") {
		t.Fatalf("execution consumer accepted inverted scoring windows: %v", err)
	}
}

func TestScoringConsumerRejectsRetiredPolicyFields(t *testing.T) {
	for _, field := range []string{"events", "error_classification"} {
		policy := testPolicy()
		policy["scoring"].(map[string]any)[field] = map[string]any{}
		if _, err := HealthScore([]Sample{{Result: "通过"}}, policy); err == nil || !strings.Contains(err.Error(), "已废弃") {
			t.Fatalf("retired scoring field %s was accepted: %v", field, err)
		}
	}
}

func TestEmptyGatewayStatusCodesUseGuardianDefaults(t *testing.T) {
	policy := testPolicy()
	policy["classify"].(map[string]any)["gateway_status_codes"] = []any{}
	status := 500
	health, err := HealthScore([]Sample{{Result: "失败", StatusCode: &status}}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if health.LatestEvent != EventGateway {
		t.Fatalf("500 was not classified by the normalized Guardian defaults: %#v", health)
	}
}

func TestHealthOutputContainsOnlyFiniteNumbers(t *testing.T) {
	health, err := HealthScore([]Sample{{Result: "通过"}}, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []float64{health.ShortScore, health.LongScore, health.HealthScore} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("non-finite health: %#v", health)
		}
	}
	if _, err := json.Marshal(health); err != nil || strings.Contains(string(mustJSON(health)), "Infinity") {
		t.Fatalf("health is not strict JSON: %v", err)
	}
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
