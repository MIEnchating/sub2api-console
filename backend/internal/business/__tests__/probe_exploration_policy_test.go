package business_test

import "testing"

func TestProbePerformanceExplorationDefaultsOnAndCanBeDisabled(t *testing.T) {
	store, _ := concurrencyStore(t)
	policy, err := store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if policy["probe"].(map[string]any)["performance_exploration_enabled"] != true {
		t.Fatal("continuous candidate comparison is not enabled by default")
	}
	saved, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"performance_exploration_enabled": false}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if saved.AdvancedPolicy["probe"].(map[string]any)["performance_exploration_enabled"] != false {
		t.Fatal("explicit exploration disable was not preserved")
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"performance_exploration_enabled": "false"}}}, "test"); err == nil {
		t.Fatal("non-boolean exploration setting accepted")
	}
}
