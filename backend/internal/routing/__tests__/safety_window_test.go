package routing_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestSafetyWindowsRemainIndependentOfScoringSampleLimit(t *testing.T) {
	for _, test := range []struct {
		name, result, state string
		policy              map[string]any
		payload             map[string]any
		cleanup             bool
	}{
		{
			name: "gateway window can fuse after three failures", result: "失败", state: "fused",
			policy:  map[string]any{"breaker": map[string]any{"min_pool_size": 0, "http_window": 3, "http_failures": 3, "http_degrade_only": false}},
			payload: map[string]any{"status_code": 502},
		},
		{
			name: "latency window can fuse after three slow successes", result: "通过", state: "fused",
			policy:  map[string]any{"breaker": map[string]any{"min_pool_size": 0, "latency_window": 3, "latency_occurrences": 3, "latency_degrade_only": false}},
			payload: map[string]any{"first_token_ms": 20000},
		},
		{
			name: "consecutive failures confirm degradation", result: "失败", state: "degraded",
			policy:  map[string]any{"breaker": map[string]any{"http_window": 5, "http_failures": 5, "transient_consecutive_failures": 3}},
			payload: map[string]any{},
		},
		{
			name: "cleanup window retains three credential failures", result: "失败", state: "fused", cleanup: true,
			policy: map[string]any{
				"breaker": map[string]any{"min_pool_size": 0},
				"cleanup": map[string]any{"enabled": true, "occurrences": 3, "window": 3, "min_fused_minutes": 0, "keep_last_in_group": false},
			},
			payload: map[string]any{"status_code": 401},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, _ := healthEvidenceStore(t)
			test.policy["scoring"] = map[string]any{"short_window": 1, "long_window": 1}
			if _, err := store.UpdatePolicy(ctx, map[string]any{"advanced_policy": test.policy}, "test"); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			var samples []business.TrafficSample
			for _, age := range []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute} {
				observed := now.Add(-age).Format(time.RFC3339Nano)
				samples = append(samples, business.TrafficSample{AccountID: "41", GroupName: "codex", Result: test.result, EvidenceKey: observed, ObservedAt: observed, Payload: test.payload})
			}
			if _, err := store.PersistTrafficSamples(ctx, samples); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(store).Calculate(ctx, routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions["41"]
			if decision.RoutingState != test.state || decision.EvidencePending || decision.SampleCount != 1 {
				t.Fatalf("scoring limit truncated independent safety evidence: %+v", decision)
			}
			if test.cleanup && (decision.CleanupAction == nil || *decision.CleanupAction != "pause") {
				t.Fatalf("scoring limit prevented confirmed cleanup: %+v", decision)
			}
		})
	}
}

func TestRecoverySuccessCountCanExceedScoringSampleLimit(t *testing.T) {
	ctx := context.Background()
	store, db := healthEvidenceStore(t)
	_, err := store.UpdatePolicy(ctx, map[string]any{"advanced_policy": map[string]any{
		"scoring":  map[string]any{"short_window": 1, "long_window": 1},
		"recovery": map[string]any{"success_count": 3},
	}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET schedulable=0,routing_state='fused' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, age := range []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute} {
		_, err = store.PersistProbeSamples(ctx, []business.ProbeSample{{AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: now.Add(-age).Format(time.RFC3339Nano)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := routing.NewService(store).Calculate(ctx, routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["41"]; decision.RoutingState != "healthy" || !decision.Schedulable || decision.SampleCount != 1 {
		t.Fatalf("scoring limit prevented recovery after three fresh successes: %+v", decision)
	}
}

func TestLargerSafetyWindowKeepsPerformanceWithinScoringWindow(t *testing.T) {
	ctx := context.Background()
	store, _ := healthEvidenceStore(t)
	_, err := store.UpdatePolicy(ctx, map[string]any{"advanced_policy": map[string]any{
		"scoring": map[string]any{"short_window": 1, "long_window": 1},
	}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var samples []business.TrafficSample
	for index, latency := range []int{100, 10000, 20000} {
		observed := now.Add(-time.Duration(index+1) * time.Minute).Format(time.RFC3339Nano)
		samples = append(samples, business.TrafficSample{
			AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: observed, ObservedAt: observed,
			Payload: map[string]any{"first_token_ms": latency},
		})
	}
	if _, err := store.PersistTrafficSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(ctx, routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.AccountDecisions["41"]
	if decision.TTFBP50MS == nil || decision.TTFBP95MS == nil || *decision.TTFBP50MS != 100 || *decision.TTFBP95MS != 100 {
		t.Fatalf("older safety samples changed the performance baseline: %+v", decision)
	}
}
