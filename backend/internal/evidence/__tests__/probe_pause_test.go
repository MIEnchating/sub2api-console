package evidence_test

import (
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestPausedProbesKeepTrafficCollectionWithoutFailureEvidence(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2"}, []string{"1"})
	fixture.now = time.Date(2026, 9, 16, 18, 0, 0, 0, time.UTC)
	fixture.policy["probe"].(map[string]any)["pause_window"] = map[string]any{
		"enabled": true, "start": "23:00", "end": "08:00", "timezone": "Asia/Shanghai",
	}
	plan, err := fixture.service.Plan(t.Context(), fixture.policy, nil, nil, fixture.now)
	if err != nil || len(plan.ProbeAccountIDs) != 0 {
		t.Fatalf("paused plan: %+v %v", plan, err)
	}
	result, err := fixture.service.Collect(t.Context(), fixture.policy, fixture.admin, evidence.Options{
		Now: fixture.now, ProbesAllowed: true, FetchTraffic: true, StrictFallback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TrafficPersisted != 1 || result.ProbesPersisted != 0 || len(fixture.probeIDs()) != 0 || len(result.SourceErrors) != 0 {
		t.Fatalf("pause must keep traffic without generating probe failures: %+v", result)
	}
	if result.FallbackReason == nil || !strings.Contains(*result.FallbackReason, "暂停时段") {
		t.Fatalf("missing pause reason: %+v", result)
	}
	plan, err = fixture.service.Plan(t.Context(), fixture.policy, nil, nil, fixture.now.Add(6*time.Hour))
	if err != nil || len(plan.ProbeAccountIDs) == 0 {
		t.Fatalf("end must resume planning: %+v %v", plan, err)
	}
}
