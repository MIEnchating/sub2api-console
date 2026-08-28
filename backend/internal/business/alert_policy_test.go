package business

import (
	"context"
	"reflect"
	"testing"
)

func TestAlertPolicyTypedPersistenceAndThresholdNormalization(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	defaults, err := store.AlertPolicy(ctx)
	if err != nil || !defaults.Enabled || !reflect.DeepEqual(defaults.BalanceThresholds, []string{"20", "10", "5"}) {
		t.Fatalf("unexpected defaults: %#v err=%v", defaults, err)
	}
	payload := alertPolicyDocument(defaults)
	payload["balance_thresholds"] = []any{"5.00", "20", "10", "20.0"}
	payload["probe_groups"] = []any{"codex", "codex", " pro "}
	stored, err := store.UpdateAlertPolicy(ctx, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.BalanceThresholds, []string{"20", "10", "5"}) || !reflect.DeepEqual(stored.ProbeGroups, []string{"codex", "pro"}) {
		t.Fatalf("unexpected normalized policy: %#v", stored)
	}
	var nodeCount, legacyCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM policy_nodes WHERE policy_key='alert-policy'`).Scan(&nodeCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM policies WHERE key='alert-policy'`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if nodeCount == 0 || legacyCount != 0 {
		t.Fatalf("typed nodes=%d legacy=%d", nodeCount, legacyCount)
	}
}

func TestAlertPolicyRejectsUnknownAndPartialWrites(t *testing.T) {
	store := openPolicyStore(t)
	if _, err := store.UpdateAlertPolicy(context.Background(), map[string]any{"enabled": true}); err == nil {
		t.Fatal("partial alert policy must be rejected")
	}
	payload := alertPolicyDocument(DefaultAlertPolicy())
	payload["unknown"] = true
	if _, err := store.UpdateAlertPolicy(context.Background(), payload); err == nil {
		t.Fatal("unknown alert policy field must be rejected")
	}
}
