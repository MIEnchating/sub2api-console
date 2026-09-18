package routing_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestProjectHealthNeutralClientErrorDoesNotAdvanceScoredEvidenceTime(t *testing.T) {
	for _, hasScoredEvidence := range []bool{false, true} {
		t.Run(fmt.Sprintf("has_scored_evidence_%t", hasScoredEvidence), func(t *testing.T) {
			store, _ := healthEvidenceStore(t)
			now := time.Now().UTC()
			scoredAt := now.Add(-time.Minute).Format(time.RFC3339Nano)
			samples := []business.TrafficSample{{
				AccountID: "41", GroupName: "codex", Result: "失败", EvidenceKey: "client-error",
				ObservedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 400},
			}}
			if hasScoredEvidence {
				samples = append(samples, business.TrafficSample{
					AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: "success",
					ObservedAt: scoredAt, Payload: map[string]any{},
				})
			}
			if _, err := store.PersistTrafficSamples(t.Context(), samples); err != nil {
				t.Fatal(err)
			}
			projected, err := routing.NewService(store).ProjectHealth(t.Context(), routing.Scope{})
			if err != nil {
				t.Fatal(err)
			}
			health := projected["41"]
			if hasScoredEvidence {
				if health.HealthEvidenceAt == nil || *health.HealthEvidenceAt != scoredAt {
					t.Fatalf("neutral error advanced the scored evidence time: %+v", health)
				}
			} else if health.HealthEvidenceAt != nil || health.HealthScore != nil || health.SampleCount != 0 {
				t.Fatalf("neutral-only evidence acquired a scored timestamp: %+v", health)
			}
		})
	}
}
