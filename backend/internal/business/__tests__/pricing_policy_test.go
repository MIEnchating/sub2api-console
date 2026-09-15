package business_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func openPricingPolicyStore(t *testing.T) *business.Store {
	t.Helper()
	store, err := business.Open(filepath.Join(t.TempDir(), "pricing-policy.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func pricingPolicyPatch(minimums any) map[string]any {
	return map[string]any{"advanced_policy": map[string]any{"price_management": map[string]any{
		"exchange_group_sets":        []any{[]any{"13", "7", "21"}},
		"group_min_cost_multipliers": minimums,
	}}}
}

func TestPricingPolicyWithoutMinimumsHasNoGroupRestriction(t *testing.T) {
	store := openPricingPolicyStore(t)
	snapshot, err := store.PolicySnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pricing := snapshot.AdvancedPolicy["price_management"].(map[string]any)
	minimums, ok := pricing["group_min_cost_multipliers"].(map[string]any)
	if !ok || len(minimums) != 0 {
		t.Fatalf("default minimums = %#v, want empty object", pricing["group_min_cost_multipliers"])
	}
}

func TestPricingPolicyMinimumsPreserveDecimalPrecisionAndZero(t *testing.T) {
	store := openPricingPolicyStore(t)
	ctx := context.Background()
	_, err := store.UpdatePolicy(ctx, pricingPolicyPatch(map[string]any{
		"13": " 0.12345678901234567890123456789 ", "7": "0", "21": "   ",
	}), "operator")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pricing := snapshot.AdvancedPolicy["price_management"].(map[string]any)
	want := map[string]any{"13": "0.12345678901234567890123456789", "7": "0"}
	if !reflect.DeepEqual(pricing["group_min_cost_multipliers"], want) {
		t.Fatalf("saved minimums = %#v, want %#v", pricing["group_min_cost_multipliers"], want)
	}
}

func TestPricingPolicyReplacingMinimumsRemovesClearedEntries(t *testing.T) {
	for _, test := range []struct {
		name string
		next map[string]any
	}{
		{name: "removing one group preserves only the submitted group", next: map[string]any{"7": "0.5"}},
		{name: "empty object clears every minimum", next: map[string]any{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openPricingPolicyStore(t)
			ctx := context.Background()
			if _, err := store.UpdatePolicy(ctx, pricingPolicyPatch(map[string]any{"13": "1", "7": "0.5"}), "operator"); err != nil {
				t.Fatal(err)
			}
			snapshot, err := store.UpdatePolicy(ctx, pricingPolicyPatch(test.next), "operator")
			if err != nil {
				t.Fatal(err)
			}
			pricing := snapshot.AdvancedPolicy["price_management"].(map[string]any)
			if !reflect.DeepEqual(pricing["group_min_cost_multipliers"], test.next) {
				t.Fatalf("replacement minimums = %#v, want %#v", pricing["group_min_cost_multipliers"], test.next)
			}
		})
	}
}

func TestPricingPolicyInvalidMinimumsRejectTheUpdate(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "non-object", value: []any{}},
		{name: "negative", value: map[string]any{"13": "-0.1"}},
		{name: "fraction", value: map[string]any{"13": "1/2"}},
		{name: "unbounded exponent", value: map[string]any{"13": "1e1001"}},
		{name: "numeric value", value: map[string]any{"13": 0.5}},
		{name: "missing value", value: map[string]any{"13": nil}},
		{name: "group name", value: map[string]any{"premium": "1"}},
		{name: "zero group ID", value: map[string]any{"0": "1"}},
		{name: "group outside exchange sets", value: map[string]any{"99": "1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openPricingPolicyStore(t)
			ctx := context.Background()
			before, err := store.PolicySnapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.UpdatePolicy(ctx, pricingPolicyPatch(test.value), "operator")
			if err == nil || !strings.Contains(err.Error(), "group_min_cost_multipliers") {
				t.Fatalf("invalid minimum accepted or field missing from error: %v", err)
			}
			after, err := store.PolicySnapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if before.Revision != after.Revision {
				t.Fatal("invalid update changed the saved policy")
			}
		})
	}
}

func TestPricingPolicyGroupDeletionClearsMinimumsOutsideSurvivingExchangeSets(t *testing.T) {
	store := openPricingPolicyStore(t)
	ctx := context.Background()
	groups := []map[string]any{
		{"id": "13", "name": "premium", "platform": "openai"},
		{"id": "7", "name": "standard", "platform": "openai"},
		{"id": "21", "name": "economy", "platform": "openai"},
		{"id": "22", "name": "reserve", "platform": "openai"},
	}
	if _, err := store.SyncManagementSnapshot(ctx, nil, groups, "operator"); err != nil {
		t.Fatal(err)
	}
	_, err := store.UpdatePolicy(ctx, map[string]any{"advanced_policy": map[string]any{"price_management": map[string]any{
		"exchange_group_sets":        []any{[]any{"13", "7"}, []any{"21", "22"}},
		"exchange_group_set_names":   []any{"premium set", "economy set"},
		"group_min_cost_multipliers": map[string]any{"13": "1", "7": "0.5", "21": "0.2"},
	}}}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, nil, groups[1:], "operator"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pricing := snapshot.AdvancedPolicy["price_management"].(map[string]any)
	want := map[string]any{"21": "0.2"}
	if !reflect.DeepEqual(pricing["group_min_cost_multipliers"], want) {
		t.Fatalf("minimums after group deletion = %#v, want %#v", pricing["group_min_cost_multipliers"], want)
	}
	if !reflect.DeepEqual(pricing["exchange_group_sets"], []any{[]any{"21", "22"}}) {
		t.Fatalf("remaining exchange sets = %#v", pricing["exchange_group_sets"])
	}
}
