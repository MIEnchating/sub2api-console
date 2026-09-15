package business_test

import (
	"fmt"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

const capacityWaitLegacyReason = "上游可用并发已变化，本轮扩容或恢复会超过用户并发上限；请重新计算调度"
const capacityWaitDetailedReason = "共享并发额度不足，已拦截本轮扩容或恢复（未执行）：上游用户上限 500，已分配及预留 500，剩余 0，本次需新增 100（账号当前 0 → 目标 100）；请同步上游与账号后重新计算调度"

func TestLegacyCapacityWaitStopsQueuedAlertsAndClosesWithoutRecoveryNotification(t *testing.T) {
	for _, reason := range []string{capacityWaitLegacyReason, capacityWaitDetailedReason} {
		for _, status := range []string{"firing", "recovered", "suppressed"} {
			t.Run(status+reason, func(t *testing.T) {
				store, db := openAlertRuleStore(t)
				if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','capacity-account','now')`); err != nil {
					t.Fatal(err)
				}
				policy := business.DefaultAlertPolicy()
				policy.RecoveryNotificationTypes = []string{"apply_failure"}
				updateAlertRulePolicy(t, store, policy)
				if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{
					OperationID: "old-wait", OperationType: "routing.writeback", State: "failed", Phase: "remote-write",
					ObjectID: "41", Error: &reason, GroupNames: []string{}, Writeback: true,
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
					VALUES('console:routing:apply:41','routing.apply_failure','account','41',?,?,'now','now')`, "APPLY_FAILED:"+reason, status); err != nil {
					t.Fatal(err)
				}
				queue, err := store.NotificationQueueDetails(t.Context(), "test-channel", true)
				if err != nil || len(queue.ConsumerItems) != 0 {
					t.Fatalf("legacy capacity alert is still queued before next evaluation: %+v err=%v", queue, err)
				}
				for range 2 {
					if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
						t.Fatal(err)
					}
					var state string
					if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:routing:apply:41'`).Scan(&state); err != nil {
						t.Fatal(err)
					}
					if state != "closed" {
						t.Fatalf("legacy wait should close without announcing recovery: %s", state)
					}
				}
			})
		}
	}
}

func TestCapacityWaitDoesNotClearPreviousRealWriteFailure(t *testing.T) {
	store, db := openAlertRuleStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','capacity-account','now')`); err != nil {
		t.Fatal(err)
	}
	reason := "共享并发复核失败，已阻止扩容和恢复：远端接口 HTTP 503"
	for i, state := range []string{"failed", "skipped"} {
		op := business.AccountOperation{OperationID: fmt.Sprint(i), OperationType: "routing.writeback", ObjectID: "41", GroupNames: []string{}, State: state, Phase: "calculation"}
		if state == "failed" {
			op.Error, op.Phase, op.Writeback = &reason, "remote-write", true
		}
		if err := store.RecordAccountOperation(t.Context(), op); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	var state, cause string
	if err := db.QueryRow(`SELECT status,cause_code FROM alert_incidents WHERE incident_key='console:routing:apply:41'`).Scan(&state, &cause); err != nil {
		t.Fatal(err)
	}
	if state != "firing" || cause != "APPLY_FAILED:"+reason {
		t.Fatalf("real failure was hidden by a waiting round: %s %s", state, cause)
	}
}
