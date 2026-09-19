package business_test

import (
	"fmt"
	"reflect"
	"testing"
)

func TestIndependentUpstreamReductionDefaultsOffAndEnablingPreservesGlobalScaling(t *testing.T) {
	store, _ := concurrencyStore(t)
	before, err := store.PolicySnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	section, ok := before.AdvancedPolicy["upstream_concurrency"].(map[string]any)
	if !ok || section["enabled"] != false {
		t.Fatalf("independent reduction must be explicitly disabled initially: %#v", section)
	}
	updated, err := store.UpdatePolicy(t.Context(), map[string]any{
		"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true}},
	}, "test-operator")
	if err != nil {
		t.Fatal(err)
	}
	if updated.AdvancedPolicy["upstream_concurrency"].(map[string]any)["enabled"] != true {
		t.Fatal("independent reduction did not enable")
	}
	if !reflect.DeepEqual(before.AdvancedPolicy["scaling"], updated.AdvancedPolicy["scaling"]) || !reflect.DeepEqual(before.AutoApply, updated.AutoApply) {
		t.Fatal("independent reduction changed global scaling or field switches")
	}
	readback, err := store.PolicySnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readback.AdvancedPolicy["upstream_concurrency"], updated.AdvancedPolicy["upstream_concurrency"]) {
		t.Fatal("independent reduction did not persist")
	}
}

func TestIndependentUpstreamReductionRejectsNonBooleanAndUnknownFields(t *testing.T) {
	for name, section := range map[string]map[string]any{
		"string switch": {"enabled": "true"},
		"unknown field": {"enabled": true, "global_max_concurrency": 1},
	} {
		t.Run(name, func(t *testing.T) {
			store, _ := concurrencyStore(t)
			before, err := store.PolicySnapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": section}}, "test-operator")
			if err == nil {
				t.Fatal("invalid independent reduction policy was accepted")
			}
			after, err := store.PolicySnapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if before.Revision != after.Revision {
				t.Fatal("rejected configuration changed the policy")
			}
		})
	}
}

func TestSelectedUpstreamConcurrencyScopePersistsStableAccountIDs(t *testing.T) {
	store, _ := concurrencyStore(t)
	section := map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{"41", "42"}}
	_, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": section}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	readback, err := store.PolicySnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readback.AdvancedPolicy["upstream_concurrency"], section) {
		t.Fatalf("selected scope not persisted: %#v", readback.AdvancedPolicy["upstream_concurrency"])
	}
}

func TestUpstreamConcurrencyScopeRejectsInvalidModeOrAccountIDs(t *testing.T) {
	for _, section := range []map[string]any{
		{"account_mode": "everything"}, {"upstream_ids": "upstream-a"}, {"upstream_ids": []any{1}}, {"upstream_ids": []any{" "}}, {"account_ids": "41"}, {"account_ids": []any{41}}, {"account_ids": []any{"account-name"}}, {"account_ids": []any{"0"}},
	} {
		t.Run(fmt.Sprint(section), func(t *testing.T) {
			store, _ := concurrencyStore(t)
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": section}}, "test"); err == nil {
				t.Fatal("invalid scope accepted")
			}
		})
	}
}

func TestUpstreamConcurrencyScopePersistsStableUpstreamIDs(t *testing.T) {
	store, _ := concurrencyStore(t)
	section := map[string]any{"enabled": true, "account_mode": "upstreams", "upstream_ids": []any{"Upstream-A", "upstream-b"}}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": section}}, "test"); err != nil {
		t.Fatal(err)
	}
	readback, err := store.PolicySnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readback.AdvancedPolicy["upstream_concurrency"], section) {
		t.Fatalf("upstream scope not persisted: %#v", readback.AdvancedPolicy["upstream_concurrency"])
	}
}
