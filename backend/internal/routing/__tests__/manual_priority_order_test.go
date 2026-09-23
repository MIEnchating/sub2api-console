package routing_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"testing"
	"time"
)

func manualOrderFixture(t *testing.T) (map[string]any, []business.RoutingAccount, []business.RoutingSample, time.Time) {
	t.Helper()
	store := upstreamCapacityFixture(t)
	policy, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	policy["manual_priority"] = map[string]any{"reserved_max": 10, "latency_priority_enabled": true}
	policy["weights"].(map[string]any)["performance_min_samples"] = 2
	now := time.Now().UTC()
	enabled := true
	first, second := int64(1), int64(3)
	accounts := []business.RoutingAccount{
		{ID: "41", GroupName: "codex", Priority: &first, ManualPriority: &first, Schedulable: &enabled, Metadata: map[string]any{}},
		{ID: "42", GroupName: "codex", Priority: &second, ManualPriority: &second, Schedulable: &enabled, Metadata: map[string]any{}},
	}
	rows := []business.RoutingSample{}
	for _, id := range []string{"41", "42"} {
		latency := "2000"
		if id == "42" {
			latency = "500"
		}
		for _, key := range []string{"r1", "r2"} {
			rows = append(rows, business.RoutingSample{AccountID: id, EvidenceKey: id + key, Source: "traffic", Result: "通过", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), LatencyP95: &latency, Payload: map[string]any{"model": "same-model", "latency_metric": "first_token", "first_token_ms": latency}})
		}
	}
	return policy, accounts, rows, now
}
func TestManualOrderRanksFreshSameModelLatencyWithoutOtherFields(t *testing.T) {
	policy, accounts, rows, now := manualOrderFixture(t)
	changes, err := routing.PlanManualPriorityOrder(policy, accounts, rows, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].AccountID != "41" || changes[0].Priority != 3 || changes[1].Priority != 1 {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	if *accounts[0].ManualPriority != 1 || *accounts[1].Priority != 3 {
		t.Fatal("planning mutated caller accounts")
	}
}
func TestManualOrderKeepsPositionsWhenEvidenceOrAuthorizationIsMissing(t *testing.T) {
	for _, scenario := range []string{"disabled", "missing samples", "stale", "different models", "paused", "single account", "failure", "empty reply", "traffic disabled"} {
		t.Run(scenario, func(t *testing.T) {
			policy, accounts, rows, now := manualOrderFixture(t)
			switch scenario {
			case "disabled":
				policy["manual_priority"].(map[string]any)["latency_priority_enabled"] = false
			case "missing samples":
				rows = rows[:3]
			case "stale":
				now = now.Add(48 * time.Hour)
			case "different models":
				for i := 2; i < len(rows); i++ {
					rows[i].Payload["model"] = "other"
				}
			case "paused":
				accounts[1].Paused = true
			case "single account":
				accounts = accounts[:1]
			case "failure":
				for i := 2; i < len(rows); i++ {
					rows[i].Result = "失败"
				}
			case "empty reply":
				for i := 2; i < len(rows); i++ {
					rows[i].Payload["input_tokens"] = 0
					rows[i].Payload["output_tokens"] = 0
				}
			case "traffic disabled":
				policy["traffic"].(map[string]any)["enabled"] = false
			}
			changes, err := routing.PlanManualPriorityOrder(policy, accounts, rows, now)
			if err != nil || len(changes) != 0 {
				t.Fatalf("must keep positions: %+v %v", changes, err)
			}
		})
	}
}
func TestManualOrderAvoidsCollisionsAcrossMultipleGroups(t *testing.T) {
	policy, accounts, rows, now := manualOrderFixture(t)
	copy := accounts[1]
	copy.GroupName = "secondary"
	accounts = append(accounts, copy)
	priority := int64(1)
	accounts = append(accounts, business.RoutingAccount{ID: "43", GroupName: "secondary", ManualPriority: &priority, Priority: &priority})
	changes, err := routing.PlanManualPriorityOrder(policy, accounts, rows, now)
	if err != nil || len(changes) != 0 {
		t.Fatalf("must preserve occupied secondary slot: %+v %v", changes, err)
	}
}

func TestManualOrderPreservesFrozenEffectiveSlotAfterPreviousRound(t *testing.T) {
	policy, accounts, rows, now := manualOrderFixture(t)
	first, second := int64(3), int64(1)
	accounts[0].Priority, accounts[1].Priority = &first, &second
	accounts[1].Paused = true
	changes, err := routing.PlanManualPriorityOrder(policy, accounts, rows, now)
	if err != nil || len(changes) != 0 {
		t.Fatalf("frozen effective slots must stay unchanged: %+v %v", changes, err)
	}
}

func TestManualOrderPreservesEffectiveOrderForEqualLatency(t *testing.T) {
	policy, accounts, rows, now := manualOrderFixture(t)
	first, second := int64(3), int64(1)
	accounts[0].Priority, accounts[1].Priority = &first, &second
	for i := range rows {
		latency := "500"
		rows[i].LatencyP95 = &latency
		rows[i].Payload["first_token_ms"] = latency
	}
	changes, err := routing.PlanManualPriorityOrder(policy, accounts, rows, now)
	if err != nil || len(changes) != 0 {
		t.Fatalf("equal latency must not reset reservations: %+v %v", changes, err)
	}
}
