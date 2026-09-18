package business_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestRestoreOutcomeResolvesApplyFailureOnlyAfterSuccessfulReadback(t *testing.T) {
	for _, scenario := range []struct {
		name, state string
		confirmed   bool
		want        string
	}{
		{name: "confirmed restore", state: "succeeded", confirmed: true, want: "recovered"},
		{name: "unconfirmed restore", state: "succeeded", want: "firing"},
		{name: "skipped restore", state: "skipped", want: "firing"},
		{name: "failed restore", state: "failed", want: "firing"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := openPolicyChangeAlertStore(t)
			reason := "账号自动执行后读回不一致：load_factor"
			recordPolicyChangeOperation(t, store, "failed-write", reason, true, false)
			if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			op := business.AccountOperation{OperationID: "restore", OperationType: "routing.restore", ObjectID: "41", GroupNames: []string{}, State: scenario.state, Phase: "readback", ReadbackConfirmed: scenario.confirmed}
			if scenario.state == "failed" {
				op.Error = &reason
			}
			if err := store.RecordAccountOperation(t.Context(), op); err != nil {
				t.Fatal(err)
			}
			if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:routing:apply:41'`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != scenario.want {
				t.Fatalf("restore alert status=%s want=%s", status, scenario.want)
			}
			account, err := store.Account(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if scenario.confirmed && account.ApplyError != nil && *account.ApplyError == reason {
				t.Fatalf("confirmed restore retained stale account error: %s", *account.ApplyError)
			}
			if !scenario.confirmed && (account.ApplyError == nil || *account.ApplyError != reason) {
				t.Fatalf("unconfirmed restore hid the account failure: %+v", account.ApplyError)
			}
		})
	}
}

func TestNewWriteFailureAfterConfirmedRestoreStillAlerts(t *testing.T) {
	store, _ := openPolicyChangeAlertStore(t)
	if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{OperationID: "restore", OperationType: "routing.restore", ObjectID: "41", GroupNames: []string{}, State: "succeeded", Phase: "readback", ReadbackConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	reason := "新一轮写入失败"
	recordPolicyChangeOperation(t, store, "new-failure", reason, false, false)
	if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	queue, err := store.NotificationQueueDetails(t.Context(), "test-channel", true)
	if err != nil || len(queue.ProducerFiring) != 1 || queue.ProducerFiring[0].CauseCode != "APPLY_FAILED:"+reason {
		t.Fatalf("old restore masked a new failure: %+v err=%v", queue, err)
	}
}

func TestFailedRestoreAfterSuccessfulWriteCreatesApplyFailure(t *testing.T) {
	store, _ := openPolicyChangeAlertStore(t)
	if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{OperationID: "write", OperationType: "routing.writeback", ObjectID: "41", GroupNames: []string{}, State: "succeeded", Phase: "readback", ReadbackConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	reason := "交还控制权读回失败"
	if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{OperationID: "restore", OperationType: "routing.restore", ObjectID: "41", GroupNames: []string{}, State: "failed", Phase: "remote-readback", Error: &reason}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	queue, err := store.NotificationQueueDetails(t.Context(), "test-channel", true)
	if err != nil || len(queue.ProducerFiring) != 1 || queue.ProducerFiring[0].CauseCode != "APPLY_FAILED:"+reason {
		t.Fatalf("failed restore was hidden: %+v err=%v", queue, err)
	}
}
