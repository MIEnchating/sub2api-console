package routing_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestHealthEvaluationUsesConfiguredScoringInProjectionAndDecisions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	repository := placementFixture(placementAccount("41", "1", 20), placementAccount("42", "1", 21))
	repository.policy["scoring"] = map[string]any{"event_scores": map[string]any{"perfect": 92, "fatal": 7}}
	repository.policy["classify"] = map[string]any{"fatal_patterns": []any{"tenant credential rejected"}}
	repository.samples = []business.RoutingSample{
		{AccountID: "41", GroupName: "codex", Source: "traffic", Result: "通过", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)},
		{AccountID: "42", GroupName: "codex", Source: "active-probe", Result: "失败", FailureReason: "tenant credential rejected", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)},
	}
	service := routing.NewService(repository, routing.WithClock(func() time.Time { return now }))
	for _, scores := range []struct{ perfect, fatal float64 }{{92, 7}, {86, 3}} {
		// A later evaluation must observe changed policy rather than a global cache.
		repository.policy["scoring"] = map[string]any{"event_scores": map[string]any{"perfect": scores.perfect, "fatal": scores.fatal}}
		projected, err := service.ProjectHealth(t.Context(), routing.Scope{})
		if err != nil {
			t.Fatal(err)
		}
		calculated, err := service.Calculate(t.Context(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		for id, expected := range map[string]float64{"41": scores.perfect, "42": scores.fatal} {
			projection := projected[id]
			decision := calculated.AccountDecisions[id]
			if projection.HealthScore == nil || *projection.HealthScore != expected || decision.HealthScore != expected || decision.SampleCount != 1 {
				t.Fatalf("account %s lost configured score %g: projection=%+v decision=%+v", id, expected, projection, decision)
			}
		}
		if calculated.AccountDecisions["42"].LatestEvent != routing.EventCredentialBad {
			t.Fatal("custom fatal classification was lost")
		}
	}
}

func TestHealthEvaluationRejectsInvalidScoringEvenWithoutAccounts(t *testing.T) {
	t.Parallel()
	repository := placementFixture()
	repository.policy["scoring"] = map[string]any{"short_window": 0}
	service := routing.NewService(repository)
	if _, err := service.ProjectHealth(t.Context(), routing.Scope{}); err == nil {
		t.Fatal("projection accepted invalid scoring")
	}
	if _, err := service.Calculate(t.Context(), routing.Scope{}, true); err == nil {
		t.Fatal("calculation accepted invalid scoring")
	}
}

func BenchmarkProjectHealthBatch(b *testing.B) {
	for _, source := range []string{"traffic", "active-probe"} {
		b.Run(source, func(b *testing.B) {
			now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
			repository := placementFixture()
			for index := range 1000 {
				id := strconv.Itoa(1000 + index)
				repository.accounts = append(repository.accounts, placementAccount(id, "1", 20))
				for sample := range 5 {
					repository.samples = append(repository.samples, business.RoutingSample{
						AccountID: id, GroupName: "codex", Source: source, Result: "通过",
						ObservedAt: now.Add(-time.Duration(sample+1) * time.Minute).Format(time.RFC3339Nano),
						Payload:    map[string]any{"input_tokens": 1, "output_tokens": 1},
					})
				}
			}
			service := routing.NewService(repository, routing.WithClock(func() time.Time { return now }))
			b.ReportAllocs()
			for b.Loop() {
				result, err := service.ProjectHealth(b.Context(), routing.Scope{})
				if err != nil || len(result) != 1000 {
					b.Fatalf("projection failed: count=%d err=%v", len(result), err)
				}
			}
		})
	}
}
