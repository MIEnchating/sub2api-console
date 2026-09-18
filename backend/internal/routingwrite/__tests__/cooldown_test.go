package routingwrite_test

import (
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestSuccessfulWriteWithoutOptionalReadbackStartsScalingCooldown(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(100), 4, 5, true)
	for _, state := range fixture.states {
		state["rate_multiplier"] = "0.5"
	}
	if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{{"id": json.Number("7"), "name": "capacity-group"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
		"cooldown_seconds": 600,
		"auto_apply":       map[string]any{"priority": false, "load_factor": false},
		"advanced_policy": map[string]any{
			"scaling":   map[string]any{"enabled": true, "cooldown_seconds": 600},
			"writeback": map[string]any{"verification": false},
			"cost_wall": map[string]any{"enabled": false},
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	calculator := routing.NewService(fixture.store)
	first, err := calculator.Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := service.Apply(t.Context(), first.AccountTargets, "scheduler")
	if err != nil || initial.Failed != 0 || initial.Changed != 2 {
		t.Fatalf("initial scaling failed: %+v err=%v", initial, err)
	}
	audit, err := fixture.store.AuditEvents(t.Context(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range audit {
		if event.State != "succeeded" || event.RemoteConfirmed == nil || !*event.RemoteConfirmed || event.ReadbackConfirmed == nil || *event.ReadbackConfirmed {
			t.Fatalf("optional verification must remain disabled: %+v", event)
		}
	}
	second, err := calculator.Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range second.AccountTargets {
		if !target.ScalingCooldown || target.Concurrency != nil {
			t.Fatalf("successful remote write must start the configured cooldown: %+v", target)
		}
	}
	applied, err := service.Apply(t.Context(), second.AccountTargets, "scheduler")
	if err != nil || applied.RemoteWrite || applied.Changed != 0 || applied.Failed != 0 {
		t.Fatalf("the next round must preserve concurrency during cooldown: %+v err=%v", applied, err)
	}
}

func TestUnconfirmedOrFailedWriteDoesNotStartCooldown(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		state     string
		confirmed bool
	}{
		{name: "unconfirmed success", state: "succeeded"},
		{name: "failed confirmed write", state: "failed", confirmed: true},
		{name: "skipped target", state: "skipped"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
			if _, err := routing.NewService(fixture.store).Calculate(t.Context(), routing.Scope{}, true); err != nil {
				t.Fatal(err)
			}
			field := "concurrency"
			if err := fixture.store.RecordAccountOperation(t.Context(), business.AccountOperation{
				OperationID: "cooldown-boundary", OperationType: "routing.writeback", ObjectID: "41",
				State: scenario.state, Phase: "remote-write", RemoteConfirmed: scenario.confirmed, FieldName: &field,
			}); err != nil {
				t.Fatal(err)
			}
			rows, err := fixture.store.PreviousRoutingDecisions(t.Context(), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if !row.LastApplyAt.IsZero() || !row.LastScalingWriteAt.IsZero() {
					t.Fatalf("an unconfirmed operation must not suppress subsequent scheduling: %+v", row)
				}
			}
		})
	}
}
