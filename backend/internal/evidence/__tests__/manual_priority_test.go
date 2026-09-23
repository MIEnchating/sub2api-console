package evidence_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"testing"
)

func TestManualPriorityLatencyOptionCollectsTrafficButNeverPlansFallbackProbe(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1"}, nil)
	if _, err := fixture.store.AssignManualPriority(t.Context(), "1", 1, "100", 100, true, "test"); err != nil {
		t.Fatal(err)
	}
	targets, err := fixture.store.EvidenceTargets(t.Context(), nil, nil)
	if err != nil || len(targets) != 0 {
		t.Fatalf("disabled option collected manual account: %v %v", targets, err)
	}
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"manual_priority": map[string]any{"latency_priority_enabled": true}}}, "test"); err != nil {
		t.Fatal(err)
	}
	targets, err = fixture.store.EvidenceTargets(t.Context(), nil, nil)
	if err != nil || len(targets) != 1 || !targets[0].ManualPriority {
		t.Fatalf("manual traffic target missing: %v %v", targets, err)
	}
	policy, err := fixture.store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := fixture.service.Plan(t.Context(), policy, nil, nil, fixture.now)
	if err != nil || len(plan.ProbeAccountIDs) != 0 {
		t.Fatalf("manual account entered probe plan: %v %v", plan, err)
	}
	result, err := fixture.service.Collect(t.Context(), policy, nil, evidence.Options{ProbesAllowed: true, FetchTraffic: true, Now: fixture.now})
	if err != nil || result.ProbesPersisted != 0 || len(fixture.probeIDs()) != 0 {
		t.Fatalf("manual account probed: %v %v", result, err)
	}
}
