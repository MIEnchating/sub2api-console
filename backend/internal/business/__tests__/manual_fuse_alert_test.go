package business_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestAlertEvaluationWithoutControlPolicyKeepsDefaultRules(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "unconfigured-alerts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatalf("absent manual controls must not block other alert rules: %v", err)
	}
}

func TestManualFuseTransitionsDoNotWaitForAutomaticBreakerCooldown(t *testing.T) {
	for _, cause := range []string{"MANUAL_FUSE", "ROUTING_BREAKER:人工熔断", "ROUTING_BREAKER:连续失败"} {
		for _, status := range []string{"firing", "recovered"} {
			t.Run(cause+"/"+status, func(t *testing.T) {
				store, db := openAlertRuleStore(t)
				policy := business.DefaultAlertPolicy()
				policy.RecoveryNotificationTypes = []string{"routing_breaker"}
				updateAlertRulePolicy(t, store, policy)
				if _, err := db.Exec(`INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at)
					VALUES('sub2api','sub2api-notify-rules.json','{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','now')`); err != nil {
					t.Fatal(err)
				}
				now := time.Now().UTC().Format(time.RFC3339Nano)
				if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
					VALUES('manual-998','account.routing_breaker','account','998',?,?,?,?)`, cause, status, now, now); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO alert_deliveries(incident_key,channel_key,status,attempts,updated_at)
					VALUES('manual-998','test-channel','transition',1,?)`, now); err != nil {
					t.Fatal(err)
				}
				plan, err := store.PrepareAlertDelivery(t.Context(), "test-channel", true)
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if cause == "ROUTING_BREAKER:连续失败" {
					want = 0
				}
				if len(plan.Pending) != want {
					t.Fatalf("manual transitions must be ready immediately; automatic transitions retain cooldown: pending=%d want=%d", len(plan.Pending), want)
				}
			})
		}
	}
}

func TestManualFuseAlertsWithoutWaitingForRoutingCalculation(t *testing.T) {
	store, db := openAlertRuleStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,updated_at) VALUES('998','manual-account',1,'now');
		INSERT INTO account_groups(account_id,group_name) VALUES('998','国产-平价')`); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitAccountControlReadback(t.Context(), "998", "fuse", "test", false, business.AccountOperation{
		OperationID: "fuse", OperationType: "account.control", State: "succeeded", Phase: "readback",
		RemoteConfirmed: true, ReadbackConfirmed: true,
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
			t.Fatal(err)
		}
		var cause, status string
		if err := db.QueryRow(`SELECT cause_code,status FROM alert_incidents WHERE event_type='account.routing_breaker' AND object_id='998'`).Scan(&cause, &status); err != nil {
			t.Fatalf("manual fuse requires a later routing calculation to alert: %v", err)
		}
		if cause != "MANUAL_FUSE" || status != "firing" {
			t.Fatalf("manual fuse cause/status = %s/%s", cause, status)
		}
	}
	// A later calculation must keep the same incident rather than replacing the
	// manual control with an automatic consecutive-failure alert.
	if _, err := db.Exec(`INSERT INTO routing_decisions(account_id,group_name,routing_state,reason,schedulable,updated_at)
		VALUES('998','国产-平价','fused','人工熔断',0,'2099-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	var total, manual int
	if err := db.QueryRow(`SELECT COUNT(*),COUNT(CASE WHEN cause_code='MANUAL_FUSE' THEN 1 END)
		FROM alert_incidents WHERE event_type='account.routing_breaker' AND object_id='998' AND status='firing'`).Scan(&total, &manual); err != nil {
		t.Fatal(err)
	}
	if total != 1 || manual != 1 {
		t.Fatalf("later calculation duplicated/reclassified manual fuse: total=%d manual=%d", total, manual)
	}
}

func TestManualFuseRecoveryWithoutNewRoutingDecision(t *testing.T) {
	store, db := openAlertRuleStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,updated_at) VALUES('998','manual-account',1,'now')`); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"fuse", "recover"} {
		if err := store.CommitAccountControlReadback(t.Context(), "998", action, "test", action == "recover", business.AccountOperation{
			OperationID: action, OperationType: "account.control", State: "succeeded", Phase: "readback",
			RemoteConfirmed: true, ReadbackConfirmed: true,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE event_type='account.routing_breaker' AND object_id='998'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "recovered" {
		t.Fatalf("manual recovery still waits for routing calculation: %s", status)
	}
}

func TestDisabledBreakerRuleDoesNotCreateManualFuseAlert(t *testing.T) {
	store, db := openAlertRuleStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,updated_at) VALUES('998','manual-account',1,'now')`); err != nil {
		t.Fatal(err)
	}
	policy := business.DefaultAlertPolicy()
	policy.RoutingBreakerEnabled = false
	updateAlertRulePolicy(t, store, policy)
	if err := store.CommitAccountControlReadback(t.Context(), "998", "fuse", "test", false, business.AccountOperation{
		OperationID: "fuse", OperationType: "account.control", State: "succeeded", Phase: "readback",
		RemoteConfirmed: true, ReadbackConfirmed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM alert_incidents WHERE event_type='account.routing_breaker'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("manual control bypassed the disabled breaker alert rule")
	}
}
