package routing_test

import (
	"fmt"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestSavedDegradationPolicyAtSupportedMaximumStillCalculatesTargets(t *testing.T) {
	for _, field := range []string{"priority_step", "min_load_factor"} {
		t.Run(field, func(t *testing.T) {
			store, db := controlScopeStore(t)
			_, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at)
				VALUES('41','managed','1',1,'{}','now');
				INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','standard','7')`)
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.UpdatePolicy(t.Context(), map[string]any{
				"advanced_policy": map[string]any{"degrade": map[string]any{field: int64(1_000_000)}},
			}, "policy-test")
			if err != nil {
				t.Fatalf("supported policy could not be saved: %v", err)
			}

			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatalf("saved policy stopped routing calculation: %v", err)
			}
			if target, found := result.AccountTargets["41"]; !found || target.Schedulable == nil || !*target.Schedulable {
				t.Fatalf("saved policy did not produce the managed account target: %+v", result.AccountTargets)
			}
		})
	}
}

func TestBalancedPolicyWithZeroPriceShareCanBeSavedAndExecuted(t *testing.T) {
	store, db := controlScopeStore(t)
	_, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at)
		VALUES('41','free','0',1,'{}','now'),('42','paid','1',1,'{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','standard','7'),('42','standard','7')`)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.UpdatePolicy(t.Context(), map[string]any{
		"global_strategy": "balanced",
		"advanced_policy": map[string]any{"weights": map[string]any{"balanced_price_ratio": 0}},
	}, "policy-test")
	if err != nil {
		t.Fatalf("zero price share, supported by the editor and group overrides, was rejected: %v", err)
	}
	weights, ok := snapshot.AdvancedPolicy["weights"].(map[string]any)
	if !ok || weights["balanced_price_ratio"] != float64(0) {
		t.Fatalf("zero price share did not persist: %+v", weights)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatalf("saved zero price share could not be executed: %v", err)
	}
	free, paid := result.AccountDecisions["41"], result.AccountDecisions["42"]
	if free.Weight <= 0 || free.Weight != paid.Weight {
		t.Fatalf("price affected equally performing accounts with zero price share: free=%+v paid=%+v", free, paid)
	}
}

func TestBalancedPolicyWithOutOfRangePriceSharePreservesPreviousPolicy(t *testing.T) {
	for _, share := range []float64{-0.1, 1.1} {
		t.Run(fmt.Sprint(share), func(t *testing.T) {
			store, _ := controlScopeStore(t)
			before, err := store.PolicySnapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.UpdatePolicy(t.Context(), map[string]any{
				"advanced_policy": map[string]any{"weights": map[string]any{"balanced_price_ratio": share}},
			}, "policy-test")
			if err == nil {
				t.Fatal("out-of-range price share was accepted")
			}
			after, err := store.PolicySnapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != before.Revision {
				t.Fatal("rejected price share changed the active policy")
			}
		})
	}
}
