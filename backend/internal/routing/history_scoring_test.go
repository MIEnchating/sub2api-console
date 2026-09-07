package routing

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestHistoricalProbeScoringUsesIndependentWindow(t *testing.T) {
	for _, tc := range []struct {
		name               string
		history, freshness int
		count              int
		short, long, score float64
	}{
		{"default history retains older probes", 1440, 900, 3, 70, 60, 67},
		{"shorter history excludes older probes", 20, 900, 2, 70, 70, 70},
		{"freshness extension does not change history", 1440, 3600, 3, 70, 60, 67},
		{"expired evidence cannot activate historical score", 1440, 300, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 9, 7, 16, 9, 0, 0, time.UTC)
			policy := routingPolicy()
			policy["probe"] = map[string]any{"freshness_seconds": tc.freshness}
			policy["scoring"].(map[string]any)["history_window_minutes"] = tc.history
			policy["scoring"].(map[string]any)["short_window"] = 2
			schedulable := true
			repository := &routingRepositoryStub{policy: policy, accounts: []business.RoutingAccount{{ID: "14", GroupName: "codex", Schedulable: &schedulable, Metadata: map[string]any{}}}}
			for i, age := range []time.Duration{9 * time.Minute, 19 * time.Minute, 33 * time.Minute} {
				result := "失败"
				if i == 1 {
					result = "通过"
				}
				repository.samples = append(repository.samples, business.RoutingSample{AccountID: "14", GroupName: "codex", Source: "active-probe", Result: result, ObservedAt: now.Add(-age).Format(time.RFC3339Nano)})
			}
			service := NewService(repository)
			service.now = func() time.Time { return now }
			result, err := service.Calculate(context.Background(), Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions["14"]
			if decision.SampleCount != tc.count || decision.ShortScore != tc.short || decision.LongScore != tc.long || decision.HealthScore != tc.score {
				t.Fatalf("history score/count mismatch: %+v", decision)
			}
			if tc.count == 0 && decision.RoutingState != "unknown" {
				t.Fatalf("stale history established health: %+v", decision)
			}
		})
	}
}

func TestHistoricalScoresDoNotExtendCurrentSafetyStreaks(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 9, 0, 0, time.UTC)
	for _, result := range []string{"通过", "失败"} {
		t.Run(result, func(t *testing.T) {
			var rows []business.RoutingSample
			for _, age := range []time.Duration{9 * time.Minute, 19 * time.Minute, 33 * time.Minute} {
				rows = append(rows, business.RoutingSample{Source: "active-probe", Result: result, ObservedAt: now.Add(-age).Format(time.RFC3339Nano)})
			}
			current, _ := selectRoutingEvidence(rows, now, 2*time.Hour, 15*time.Minute, 60)
			history := selectScoringHistory(rows, current, now, 24*time.Hour, 60)
			health, err := scoreRoutingHealth(current, history, testPolicy())
			if err != nil {
				t.Fatal(err)
			}
			if health.SampleCount != 3 || len(health.Events) != 1 {
				t.Fatalf("historical scores leaked into current evidence: %+v", health)
			}
			if result == "通过" && (health.RecoveryPassStreak != 1 || health.FailureStreak != 0) {
				t.Fatalf("stale successes extended recovery: %+v", health)
			}
			if result == "失败" && (health.FailureStreak != 1 || health.RecoveryPassStreak != 0) {
				t.Fatalf("stale failures extended failure streak: %+v", health)
			}
		})
	}
}

func TestScoringHistoryKeepsCurrentTrafficPreferenceAndPerformanceFresh(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 9, 0, 0, time.UTC)
	for _, tc := range []struct {
		name             string
		latestTrafficAge time.Duration
		count            int
	}{
		{"fresh traffic uses retained traffic history", 10 * time.Minute, 3},
		{"expired traffic leaves probe history eligible", 3 * time.Hour, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := []business.RoutingSample{
				{Source: "active-probe", Result: "通过", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)},
				{Source: "active-probe", Result: "失败", ObservedAt: now.Add(-20 * time.Minute).Format(time.RFC3339Nano)},
				{Source: "traffic", Result: "通过", ObservedAt: now.Add(-tc.latestTrafficAge).Format(time.RFC3339Nano)},
				{Source: "traffic", Result: "失败", ObservedAt: now.Add(-4 * time.Hour).Format(time.RFC3339Nano)},
			}
			current, performance := selectRoutingEvidence(rows, now, 2*time.Hour, 15*time.Minute, 60)
			history := selectScoringHistory(rows, current, now, 24*time.Hour, 60)
			health, err := scoreRoutingHealth(current, history, testPolicy())
			if err != nil {
				t.Fatal(err)
			}
			if health.SampleCount != tc.count {
				t.Fatalf("wrong history source: %+v", history)
			}
			if tc.latestTrafficAge > 2*time.Hour && len(performance) != 0 {
				t.Fatalf("stale traffic entered performance: %+v", performance)
			}
			if tc.latestTrafficAge < 2*time.Hour && len(performance) != 1 {
				t.Fatalf("history entered performance: %+v", performance)
			}
		})
	}
}

func TestHistoricalSuccessCannotDiluteFreshFatalProbe(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 9, 0, 0, time.UTC)
	rows := []business.RoutingSample{
		{Source: "active-probe", Result: "失败", FailureReason: "invalid api key", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)},
		{Source: "active-probe", Result: "通过", ObservedAt: now.Add(-20 * time.Minute).Format(time.RFC3339Nano)},
	}
	current, _ := selectRoutingEvidence(rows, now, 2*time.Hour, 15*time.Minute, 60)
	history := selectScoringHistory(rows, current, now, 24*time.Hour, 60)
	health, err := scoreRoutingHealth(current, history, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !health.Fatal || health.HealthScore != 0 || health.SampleCount != 2 {
		t.Fatalf("historical success diluted fatal evidence: %+v", health)
	}
	current, _ = selectRoutingEvidence(rows, now.Add(time.Hour), 2*time.Hour, 15*time.Minute, 60)
	history = selectScoringHistory(rows, current, now.Add(time.Hour), 24*time.Hour, 60)
	health, err = scoreRoutingHealth(current, history, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if health.Fatal || health.SampleCount != 0 {
		t.Fatalf("stale fatal evidence was reapplied: %+v", health)
	}
}

func TestEvidenceWindowDefaultsDoNotFollowGlobalProbeInterval(t *testing.T) {
	policy := routingPolicy()
	policy["probe"] = map[string]any{"interval_seconds": 3600}
	config, err := parseEngineConfig(policy)
	if err != nil {
		t.Fatal(err)
	}
	if config.probeMaxAge != 15*time.Minute || config.historyMaxAge != 24*time.Hour {
		t.Fatalf("window defaults depend on interval: %+v", config)
	}
}

func TestFusedAccountScoresOnlyFreshRecoveryProbes(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 9, 0, 0, time.UTC)
	policy := routingPolicy()
	stopped := false
	repository := &routingRepositoryStub{
		policy:   policy,
		accounts: []business.RoutingAccount{{ID: "14", GroupName: "codex", EffectiveState: "fused", Schedulable: &stopped, Metadata: map[string]any{}}},
		samples: []business.RoutingSample{
			{AccountID: "14", GroupName: "codex", Source: "active-probe", Result: "通过", ObservedAt: now.Add(-9 * time.Minute).Format(time.RFC3339Nano)},
			{AccountID: "14", GroupName: "codex", Source: "active-probe", Result: "通过", ObservedAt: now.Add(-19 * time.Minute).Format(time.RFC3339Nano)},
			{AccountID: "14", GroupName: "codex", Source: "active-probe", Result: "失败", FailureReason: "invalid api key", ObservedAt: now.Add(-33 * time.Minute).Format(time.RFC3339Nano)},
		},
	}
	service := NewService(repository)
	service.now = func() time.Time { return now }
	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.AccountDecisions["14"]
	if decision.HealthScore != 100 || decision.SampleCount != 1 {
		t.Fatalf("pre-fuse history changed recovery score: %+v", decision)
	}
	if decision.RoutingState != "fused" || decision.Recovery == nil || decision.Recovery.Ready {
		t.Fatalf("stale success allowed recovery: %+v", decision)
	}
}

func TestDecisionPersistsActualShortAndLongSampleCounts(t *testing.T) {
	for _, count := range []int{0, 1, 58} {
		item := &candidate{health: Health{SampleCount: count}}
		decision := publicDecision(item, "codex", engineConfig{shortWindow: 10}, time.Now())
		payload := decisionWrite(decision).Payload
		if payload["short_sample_count"] != min(10, count) || payload["long_sample_count"] != count {
			t.Fatalf("count %d: wrong persisted counts: %#v", count, payload)
		}
	}
}
