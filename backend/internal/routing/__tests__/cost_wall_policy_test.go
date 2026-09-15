package routing_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestCostWallPolicyDisabledRestoresSchedulingAndWeight(t *testing.T) {
	for _, paused := range []bool{false, true} {
		t.Run(map[bool]string{false: "cost blocked", true: "manually paused"}[paused], func(t *testing.T) {
			store, db := costWallStore(t, "1.25")
			if _, err := db.Exec(`UPDATE accounts SET schedulable=0,routing_state='cost_blocked',paused=? WHERE id='41'`, paused); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
				"cost_wall": map[string]any{"enabled": false}, "scope": map[string]any{"manage_all_accounts": false},
			}}, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions["41"]
			if paused {
				if decision.Schedulable || decision.RoutingState != "paused" {
					t.Fatalf("disabling cost wall must preserve manual pause: %+v", decision)
				}
			} else if !decision.Schedulable || decision.RoutingState == "cost_blocked" || decision.Weight <= 0 {
				t.Fatalf("disabled cost wall must restore scheduling and allocation: %+v", decision)
			}
		})
	}
}

func TestCostWallPolicyDisablingFallbackClosesPreviouslyEnabledSurvivor(t *testing.T) {
	store, db := costWallStore(t, "1")
	if _, err := db.Exec(`UPDATE accounts SET paused=1,schedulable=0 WHERE id='42'; UPDATE accounts SET routing_state='survivor' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"cost_wall": map[string]any{"fallback_enabled": false},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["41"]; decision.Schedulable || decision.RoutingState != "cost_blocked" || decision.Weight != 0 {
		t.Fatalf("disabled fallback must keep even the last account closed: %+v", decision)
	}
}

func TestCostWallPolicyDisabledDoesNotReopenAccountWithUnknownSchedulingStatus(t *testing.T) {
	store, db := costWallStore(t, "1.25")
	if _, err := db.Exec(`UPDATE accounts SET schedulable=NULL,routing_state='cost_blocked' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"cost_wall": map[string]any{"enabled": false},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["41"]; decision.Schedulable || decision.RoutingState != "unknown" {
		t.Fatalf("disabling cost wall must not bypass unknown remote scheduling status: %+v", decision)
	}
}

func TestCostWallPolicyProbeSwitchesApplyBeforeScheduling(t *testing.T) {
	for _, field := range []string{"enabled", "stop_auto_probe"} {
		t.Run(field, func(t *testing.T) {
			store, _ := costWallStore(t, "1")
			policy, err := store.ControlPolicy(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			policy["cost_wall"] = map[string]any{field: false}
			accounts, err := store.RoutingAccounts(t.Context(), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			blocked, err := routing.CostWallProbeBlocks(accounts, policy)
			if err != nil || blocked["41"] {
				t.Fatalf("%s=false must allow automatic probes: blocked=%v err=%v", field, blocked, err)
			}
		})
	}
}
