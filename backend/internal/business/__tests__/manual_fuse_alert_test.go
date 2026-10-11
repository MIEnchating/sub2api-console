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

func TestManualFuseTransitionsAreNotDeliveredAndAutomaticBreakerKeepsCooldown(t *testing.T) {
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
				want := 0
				if len(plan.Pending) != want {
					t.Fatalf("manual transitions must not send; automatic transitions retain cooldown: pending=%d want=%d", len(plan.Pending), want)
				}
			})
		}
	}
}

func TestManualFuseDoesNotCreateBreakerAlert(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		confirmed bool
		reason    string
	}{
		{name: "confirmed control before routing calculation", confirmed: true},
		{name: "confirmed control overrides stale automatic decision", confirmed: true, reason: "连续失败"},
		{name: "legacy manual routing decision", reason: "人工熔断"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := openAlertRuleStore(t)
			if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,updated_at) VALUES('998','manual-account',1,'now');
				INSERT INTO account_groups(account_id,group_name) VALUES('998','国产-平价')`); err != nil {
				t.Fatal(err)
			}
			if scenario.confirmed {
				if err := store.CommitAccountControlReadback(t.Context(), "998", "fuse", "test", false, business.AccountOperation{
					OperationID: "fuse", OperationType: "account.control", State: "succeeded", Phase: "readback",
					RemoteConfirmed: true, ReadbackConfirmed: true,
				}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.reason != "" {
				if _, err := db.Exec(`INSERT INTO routing_decisions(account_id,group_name,routing_state,reason,schedulable,updated_at)
					VALUES('998','国产-平价','fused',?,0,'2099-01-01T00:00:00Z')`, scenario.reason); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM alert_incidents WHERE event_type='account.routing_breaker'`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("manual fuse created %d breaker alerts", count)
				}
			}
		})
	}
}

func TestLegacyManualFuseAlertsCloseWithoutRecoveryNotification(t *testing.T) {
	for _, cause := range []string{"MANUAL_FUSE", "ROUTING_BREAKER:人工熔断"} {
		for _, status := range []string{"firing", "recovered", "suppressed"} {
			t.Run(cause+"/"+status, func(t *testing.T) {
				store, db := openAlertRuleStore(t)
				policy := business.DefaultAlertPolicy()
				policy.RecoveryNotificationTypes = []string{"routing_breaker"}
				updateAlertRulePolicy(t, store, policy)
				if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
					VALUES('manual-998','account.routing_breaker','account','998',?,?,'now','now')`, cause, status); err != nil {
					t.Fatal(err)
				}
				queue, err := store.NotificationQueueDetails(t.Context(), "test-channel", true)
				if err != nil || len(queue.ConsumerItems) != 0 || len(queue.ProducerFiring) != 0 || len(queue.ProducerRecovered) != 0 {
					t.Fatalf("legacy manual alert still queued: %+v err=%v", queue, err)
				}
				for range 2 {
					if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
						t.Fatal(err)
					}
					var state string
					if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='manual-998'`).Scan(&state); err != nil {
						t.Fatal(err)
					}
					if state != "closed" {
						t.Fatalf("manual alert must close without recovery notification: %s", state)
					}
				}
			})
		}
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
