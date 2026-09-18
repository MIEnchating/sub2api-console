package evidence_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestPerformanceExplorationIncludesFreshTrafficReferencesWithinExistingBatch(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2", "3"}, []string{"1"})
	fixture.policy["probe"].(map[string]any)["performance_exploration_enabled"] = true
	result, err := fixture.service.Collect(context.Background(), fixture.policy, fixture.admin, evidence.Options{FetchTraffic: true, ProbesAllowed: true, ProbeBatchSize: 2, Now: fixture.now})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProbesPersisted != 2 || result.ProbesDeferred != 1 || !slices.Equal(fixture.probeIDs(), []string{"1", "2"}) {
		t.Fatalf("fresh reference missing or existing budget exceeded: result=%+v probed=%v", result, fixture.probeIDs())
	}
}

func TestPerformanceExplorationDisabledPreservesFreshTrafficSkip(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2"}, []string{"1"})
	fixture.policy["probe"].(map[string]any)["performance_exploration_enabled"] = false
	result, err := fixture.service.Collect(context.Background(), fixture.policy, fixture.admin, evidence.Options{FetchTraffic: true, ProbesAllowed: true, ProbeBatchSize: 1, Now: fixture.now})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProbesPersisted != 1 || !slices.Equal(fixture.probeIDs(), []string{"2"}) {
		t.Fatalf("disabled exploration changed skip behavior: result=%+v probed=%v", result, fixture.probeIDs())
	}
}

func TestPerformanceExplorationKeepsProbeIntervalAndColdAccountRotation(t *testing.T) {
	fixture := newProbeBatchFixture(t, []string{"1", "2", "3"}, []string{"1"})
	fixture.policy["probe"].(map[string]any)["performance_exploration_enabled"] = true
	fixture.seedProbe(t, "1", fixture.now.Add(-time.Minute))
	for _, now := range []time.Time{fixture.now, fixture.now.Add(time.Second)} {
		_, err := fixture.service.Collect(context.Background(), fixture.policy, fixture.admin, evidence.Options{FetchTraffic: true, ProbesAllowed: true, ProbeBatchSize: 1, Now: now})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(fixture.probeIDs(), []string{"2", "3"}) {
		t.Fatalf("references bypassed interval or starved cold accounts: probed=%v", fixture.probeIDs())
	}
}
