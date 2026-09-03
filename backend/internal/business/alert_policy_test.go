package business

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestAlertPolicyTypedPersistenceAndThresholdNormalization(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	defaults, err := store.AlertPolicy(ctx)
	if err != nil || !defaults.Enabled || !reflect.DeepEqual(defaults.BalanceThresholds, []string{"20", "10", "5"}) ||
		defaults.ProbeFailureStreak != 3 || defaults.ProbeRecoveryStreak != 3 || defaults.StateChangeCooldown != 30 ||
		!reflect.DeepEqual(defaults.RoutingDegradedTypes, []string{"health_score", "gateway_error_rate", "latency", "other"}) ||
		!reflect.DeepEqual(defaults.RecoveryNotificationTypes, []string{"auth", "balance", "group_unavailable"}) {
		t.Fatalf("unexpected defaults: %#v err=%v", defaults, err)
	}
	payload := alertPolicyDocument(defaults)
	payload["balance_thresholds"] = []any{"5.00", "20", "10", "20.0"}
	payload["probe_groups"] = []any{"codex", "codex", " pro "}
	payload["routing_degraded_types"] = []any{"other", "health_score", "health_score"}
	payload["recovery_notification_types"] = []any{"apply_failure", "auth", "auth"}
	stored, err := store.UpdateAlertPolicy(ctx, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.BalanceThresholds, []string{"20", "10", "5"}) || !reflect.DeepEqual(stored.ProbeGroups, []string{"codex", "pro"}) ||
		!reflect.DeepEqual(stored.RoutingDegradedTypes, []string{"health_score", "other"}) ||
		!reflect.DeepEqual(stored.RecoveryNotificationTypes, []string{"auth", "apply_failure"}) {
		t.Fatalf("unexpected normalized policy: %#v", stored)
	}
	var nodeCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM policy_nodes WHERE policy_key='alert-policy'`).Scan(&nodeCount); err != nil {
		t.Fatal(err)
	}
	if nodeCount == 0 {
		t.Fatalf("typed nodes=%d", nodeCount)
	}
}

func TestAlertPolicyRejectsUnknownDegradedAndRecoveryTypes(t *testing.T) {
	store := openPolicyStore(t)
	payload := alertPolicyDocument(DefaultAlertPolicy())
	payload["routing_degraded_types"] = []any{"score"}
	if _, err := store.UpdateAlertPolicy(context.Background(), payload); err == nil {
		t.Fatal("unknown routing degraded type must be rejected")
	}
	payload = alertPolicyDocument(DefaultAlertPolicy())
	payload["recovery_notification_types"] = []any{"everything"}
	if _, err := store.UpdateAlertPolicy(context.Background(), payload); err == nil {
		t.Fatal("unknown recovery notification type must be rejected")
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

func TestAlertPolicyRejectsInvalidRecoveryAndCooldownLimits(t *testing.T) {
	store := openPolicyStore(t)
	payload := alertPolicyDocument(DefaultAlertPolicy())
	payload["probe_recovery_streak"] = 0
	if _, err := store.UpdateAlertPolicy(context.Background(), payload); err == nil {
		t.Fatal("zero probe recovery streak must be rejected")
	}
	payload = alertPolicyDocument(DefaultAlertPolicy())
	payload["state_change_cooldown_minutes"] = 10081
	if _, err := store.UpdateAlertPolicy(context.Background(), payload); err == nil {
		t.Fatal("state change cooldown above seven days must be rejected")
	}
}

func TestZeroStateChangeCooldownIsImmediatelyDue(t *testing.T) {
	now := time.Now().UTC()
	deliveredAt := now.Format(time.RFC3339Nano)
	if !deliveryCooldownDue(&deliveredAt, 0, now) {
		t.Fatal("zero state change cooldown must disable cooling")
	}
	if deliveryCooldownDue(&deliveredAt, 30, now) {
		t.Fatal("positive state change cooldown elapsed immediately")
	}
}
