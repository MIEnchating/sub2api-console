package business_test

import (
	"reflect"
	"testing"
)

func TestCostWallPolicyDefaultsOnAndPersistsIndependentSwitches(t *testing.T) {
	store, _ := concurrencyStore(t)
	before, err := store.PolicySnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"enabled": true, "fallback_enabled": true, "stop_auto_probe": true}
	if !reflect.DeepEqual(before.AdvancedPolicy["cost_wall"], want) {
		t.Fatalf("cost wall defaults: got %#v", before.AdvancedPolicy["cost_wall"])
	}
	for _, field := range []string{"fallback_enabled", "stop_auto_probe", "enabled"} {
		want[field] = false
		if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
			"cost_wall": want,
		}}, "test"); err != nil {
			t.Fatal(err)
		}
		after, err := store.PolicySnapshot(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(after.AdvancedPolicy["cost_wall"], want) || !reflect.DeepEqual(before.AutoApply, after.AutoApply) {
			t.Fatalf("cost wall setting must persist independently: %+v", after.AdvancedPolicy["cost_wall"])
		}
	}
}

func TestCostWallPolicyRejectsNonBooleanValuesWithoutSaving(t *testing.T) {
	for _, field := range []string{"enabled", "fallback_enabled", "stop_auto_probe"} {
		t.Run(field, func(t *testing.T) {
			store, _ := concurrencyStore(t)
			before, err := store.PolicySnapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
				"cost_wall": map[string]any{field: "false"},
			}}, "test"); err == nil {
				t.Fatal("non-boolean switch accepted")
			}
			after, err := store.PolicySnapshot(t.Context())
			if err != nil || before.Revision != after.Revision {
				t.Fatalf("invalid config must not be saved: %v", err)
			}
		})
	}
}
