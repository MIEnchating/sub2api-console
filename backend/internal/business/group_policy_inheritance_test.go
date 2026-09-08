package business

import (
	"context"
	"testing"
)

func TestGroupPolicyInheritedStrategyPreservesProbeModel(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at) VALUES('codex','6','balanced','global_default','now')`); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"enabled": true, "strategy": "speed_first", "min_pool_size": 1, "weight_budget": 400,
		"balanced_price_ratio": 0.5, "breaker_enabled": true, "recovery_enabled": true,
		"weights_enabled": true, "scaling_enabled": false, "probe_enabled": true,
		"probe_interval_seconds": 300, "probe_model": "saved-probe-model",
	}
	if _, err := store.UpdateGroupPolicy(ctx, "6", payload, "operator"); err != nil {
		t.Fatal(err)
	}
	payload["strategy"] = nil
	row, err := store.UpdateGroupPolicy(ctx, "6", payload, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if row.StrategySource != "global_default" || row.Strategy != "balanced" {
		t.Fatalf("strategy not inherited: %#v", row)
	}
	if row.Override == nil || row.Override.ProbeModel == nil || *row.Override.ProbeModel != "saved-probe-model" || row.Override.Strategy != nil {
		t.Fatalf("model lost or strategy fixed: %#v", row.Override)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	binding := document["group_policy_bindings"].(map[string]any)["6"].(map[string]any)
	if _, exists := binding["strategy"]; exists {
		t.Fatal("inherited strategy must be omitted from runtime policy")
	}
	if _, err := store.UpdatePolicy(ctx, map[string]any{"global_strategy": "price_first"}, "operator"); err != nil {
		t.Fatal(err)
	}
	row, err = store.groupByID(ctx, "6")
	if err != nil {
		t.Fatal(err)
	}
	if row.Strategy != "price_first" || row.StrategySource != "global_default" {
		t.Fatalf("global changes not followed: %#v", row)
	}
	if row.Override == nil || row.Override.ProbeModel == nil || *row.Override.ProbeModel != "saved-probe-model" {
		t.Fatalf("global update lost model: %#v", row.Override)
	}
}
