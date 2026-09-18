package business_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

const legacyPolicyChangeReason = "等待写回期间策略已变化，请重新计算调度"

func TestLegacyPolicyChangeClosesWithoutQueuedFailureOrRecovery(t *testing.T) {
	for _, status := range []string{"firing", "recovered", "suppressed"} {
		t.Run(status, func(t *testing.T) {
			store, db := openPolicyChangeAlertStore(t)
			recordPolicyChangeOperation(t, store, "old-policy", legacyPolicyChangeReason, false, false)
			if status == "recovered" {
				if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{
					OperationID: "confirmed", OperationType: "routing.writeback", ObjectID: "41",
					State: "succeeded", Phase: "readback", ReadbackConfirmed: true, GroupNames: []string{},
				}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
				VALUES('console:routing:apply:41','routing.apply_failure','account','41',?,?,'now','now')`, "APPLY_FAILED:"+legacyPolicyChangeReason, status); err != nil {
				t.Fatal(err)
			}
			queue, err := store.NotificationQueueDetails(t.Context(), "test-channel", true)
			if err != nil || len(queue.ConsumerItems) != 0 {
				t.Fatalf("policy deferral must not remain queued before evaluation: %+v err=%v", queue, err)
			}
			for range 2 {
				if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
					t.Fatal(err)
				}
				var actual string
				if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:routing:apply:41'`).Scan(&actual); err != nil {
					t.Fatal(err)
				}
				if actual != "closed" {
					t.Fatalf("policy deferral must close silently, got %s", actual)
				}
				queue, err = store.NotificationQueueDetails(t.Context(), "test-channel", true)
				if err != nil || len(queue.ConsumerItems) != 0 {
					t.Fatalf("policy deferral created a recovery notification: %+v err=%v", queue, err)
				}
			}
		})
	}
}

func TestLegacyPolicyChangeDoesNotAppearAsAccountApplyError(t *testing.T) {
	store, _ := openPolicyChangeAlertStore(t)
	recordPolicyChangeOperation(t, store, "old-policy", legacyPolicyChangeReason, false, false)
	account, err := store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.ApplyError == nil || *account.ApplyError != "尚未应用到 Sub2API" {
		t.Fatalf("policy deferral must leave pending state without a failure: %+v", account.ApplyError)
	}
	accounts, err := store.Accounts(t.Context())
	if err != nil || len(accounts) != 1 {
		t.Fatalf("load accounts: %+v err=%v", accounts, err)
	}
	if accounts[0].ApplyError == nil || *accounts[0].ApplyError != "尚未应用到 Sub2API" {
		t.Fatalf("policy deferral must leave list pending state without a failure: %+v", accounts[0].ApplyError)
	}
}

func TestLegacyPolicyChangePreservesEarlierRealFailure(t *testing.T) {
	for _, reason := range []string{"读取账号失败：HTTP 503", "写入请求超时", "写回后读回失败"} {
		t.Run(reason, func(t *testing.T) {
			store, db := openPolicyChangeAlertStore(t)
			recordPolicyChangeOperation(t, store, "real-failure", reason, reason == "写回后读回失败", false)
			recordPolicyChangeOperation(t, store, "old-policy", legacyPolicyChangeReason, false, false)
			if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
				VALUES('console:routing:apply:41','routing.apply_failure','account','41',?,'firing','now','now')`, "APPLY_FAILED:"+legacyPolicyChangeReason); err != nil {
				t.Fatal(err)
			}
			if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			var status, cause string
			if err := db.QueryRow(`SELECT status,cause_code FROM alert_incidents WHERE incident_key='console:routing:apply:41'`).Scan(&status, &cause); err != nil {
				t.Fatal(err)
			}
			if status != "firing" || cause != "APPLY_FAILED:"+reason {
				t.Fatalf("policy deferral hid earlier real failure: %s %s", status, cause)
			}
			account, err := store.Account(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if account.ApplyError == nil || *account.ApplyError != reason {
				t.Fatalf("account did not retain real failure: %+v", account.ApplyError)
			}
		})
	}
}

func TestOlderPolicyDeferralDoesNotHideRecoveryOfConfirmedFailure(t *testing.T) {
	for _, policyFirst := range []bool{true, false} {
		name := "policy deferral before real failure"
		if !policyFirst {
			name = "policy deferral after real failure"
		}
		t.Run(name, func(t *testing.T) {
			store, db := openPolicyChangeAlertStore(t)
			if policyFirst {
				recordPolicyChangeOperation(t, store, "old-policy", legacyPolicyChangeReason, false, false)
			}
			recordPolicyChangeOperation(t, store, "real-failure", legacyPolicyChangeReason, true, false)
			if !policyFirst {
				recordPolicyChangeOperation(t, store, "old-policy", legacyPolicyChangeReason, false, false)
			}
			if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{
				OperationID: "confirmed", OperationType: "routing.writeback", ObjectID: "41",
				State: "succeeded", Phase: "readback", ReadbackConfirmed: true, GroupNames: []string{},
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:routing:apply:41'`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "recovered" {
				t.Fatalf("earlier policy deferral hid a real recovery: %s", status)
			}
			queue, err := store.NotificationQueueDetails(t.Context(), "test-channel", true)
			if err != nil || len(queue.ConsumerItems) != 1 || queue.ConsumerItems[0].Status != "recovered" {
				t.Fatalf("real recovery was removed from queue: %+v err=%v", queue, err)
			}
		})
	}
}

func TestPolicyChangeTextDoesNotHideConfirmedOrOtherOperationFailures(t *testing.T) {
	for _, scenario := range []struct {
		name, operationType, reason string
		remote, readback            bool
	}{
		{name: "remote confirmed", operationType: "routing.writeback", reason: legacyPolicyChangeReason, remote: true},
		{name: "readback confirmed", operationType: "routing.writeback", reason: legacyPolicyChangeReason, readback: true},
		{name: "cleanup failure", operationType: "cleanup.delete", reason: legacyPolicyChangeReason},
		{name: "wrapped remote failure", operationType: "routing.writeback", reason: "远端写入失败：" + legacyPolicyChangeReason},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := openPolicyChangeAlertStore(t)
			recordPolicyChangeOperation(t, store, "older-policy", legacyPolicyChangeReason, false, false)
			if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{
				OperationID: "real-failure", OperationType: scenario.operationType, State: "failed", Phase: "remote-readback",
				ObjectID: "41", Error: &scenario.reason, GroupNames: []string{}, Writeback: true,
				RemoteConfirmed: scenario.remote, ReadbackConfirmed: scenario.readback,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:routing:apply:41'`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "firing" {
				t.Fatalf("real failure was closed: %s", status)
			}
			queue, err := store.NotificationQueueDetails(t.Context(), "test-channel", true)
			if err != nil || len(queue.ConsumerItems) != 1 {
				t.Fatalf("real failure was removed from notification queue: %+v err=%v", queue, err)
			}
			account, err := store.Account(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if account.ApplyError == nil || *account.ApplyError != scenario.reason {
				t.Fatalf("real account failure was hidden: %+v", account.ApplyError)
			}
		})
	}
}

func openPolicyChangeAlertStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	store, db := openAlertRuleStore(t)
	if _, err := store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,routing_state,priority,target_priority,updated_at)
		VALUES('41','policy-account',1,'healthy',10,20,'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO routing_decisions(account_id,group_name,schedulable,role,routing_state,updated_at,payload_json)
		VALUES('41','policy-group',1,'healthy','healthy',?,'{}')`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	policy := business.DefaultAlertPolicy()
	policy.RecoveryNotificationTypes = []string{"apply_failure"}
	updateAlertRulePolicy(t, store, policy)
	return store, db
}

func recordPolicyChangeOperation(t *testing.T, store *business.Store, id, reason string, remote, readback bool) {
	t.Helper()
	if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{
		OperationID: id, OperationType: "routing.writeback", State: "failed", Phase: "remote-write",
		ObjectID: "41", Error: &reason, GroupNames: []string{}, Writeback: true,
		RemoteConfirmed: remote, ReadbackConfirmed: readback,
	}); err != nil {
		t.Fatal(err)
	}
}
