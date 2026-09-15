package routing_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestEmptyScoringHistoryPreservesFreshHealthEvidence(t *testing.T) {
	for _, test := range []struct {
		name, result, state string
		score               float64
	}{
		{name: "fresh success remains healthy", result: "通过", state: "healthy", score: 100},
		{name: "fresh failure remains degraded", result: "失败", state: "degraded", score: 40},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, _ := healthEvidenceStore(t)
			_, err := store.UpdatePolicy(ctx, map[string]any{"advanced_policy": map[string]any{
				"scoring": map[string]any{"history_window_minutes": 1},
			}}, "test")
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.PersistTrafficSamples(ctx, []business.TrafficSample{{
				AccountID: "41", GroupName: "codex", Result: test.result, EvidenceKey: "fresh",
				ObservedAt: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano),
				Payload:    map[string]any{},
			}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(store).Calculate(ctx, routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions["41"]
			if decision.HealthScore != test.score || decision.SampleCount != 1 || decision.RoutingState != test.state {
				t.Fatalf("empty history discarded fresh %s evidence: %+v", test.result, decision)
			}
		})
	}
}
