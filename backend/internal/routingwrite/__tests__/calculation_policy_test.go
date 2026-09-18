package routingwrite_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestCalculatedTargetsAreSkippedWhenGroupIsExcludedBeforeApply(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	calculated, err := routing.NewService(fixture.store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(calculated.AccountTargets) != 2 {
		t.Fatalf("missing calculated targets: %+v", calculated)
	}
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"excluded_group_ids": []any{"7"}}, "operator"); err != nil {
		t.Fatal(err)
	}
	applied, err := service.Apply(t.Context(), calculated.AccountTargets, "scheduler")
	if err != nil || applied.RemoteWrite || applied.Failed != 0 || len(applied.Results) != 2 {
		t.Fatalf("stale targets must be deferred without remote changes: %+v err=%v", applied, err)
	}
	for _, item := range applied.Results {
		if !item.Skipped || item.Error != nil || item.Reason == nil {
			t.Fatalf("missing explicit policy deferral: %+v", item)
		}
	}
}
