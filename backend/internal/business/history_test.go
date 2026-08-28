package business

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryReadModelsPreserveNullabilityAndSurfaceDamagedJSON(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','example-0.1','{}','now')`,
		`INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json) VALUES(-1,'test','now','succeeded','ok','not-json')`,
		`INSERT INTO health_samples(account_id,group_name,result,latency_p95,observed_at,source,evidence_key,payload_json) VALUES('41','codex','success','120','now','traffic','request-1','{}')`,
		`INSERT INTO routing_decisions(account_id,group_name,schedulable,updated_at,payload_json) VALUES('41','codex',2,'now','{}')`,
		`INSERT INTO usage_records(request_id,account_id,account_name,group_name,is_error,error_reason,observed_at,source,payload_json) VALUES('req/1','41','example-0.1','codex',1,'HTTP 500','now','traffic','{}')`,
		`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at) VALUES('a','account.probe','account','41','PROBE','firing','now','now')`,
		`INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,remote_confirmed,readback_confirmed,object_type,object_id,group_names_json,writeback,created_at) VALUES(-1,'op','routing.writeback','failed','writeback',0,NULL,'account','41','["codex"]',1,'now')`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatalf("fixture failed: %v\n%s", err, statement)
		}
	}

	events, err := store.Events(ctx, nil)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if _, found := events[0].Payload["_invalid_configuration"]; !found {
		t.Fatalf("damaged payload was hidden: %#v", events[0].Payload)
	}
	decisions, err := store.RoutingDecisions(ctx, nil, nil, nil)
	if err != nil || decisions[0].Schedulable != nil {
		t.Fatalf("invalid boolean became enabled: %#v err=%v", decisions, err)
	}
	trace, err := store.RequestTrace(ctx, "req/1")
	if err != nil || !trace.Matched || len(trace.Records) != 1 || len(trace.RecentErrors) != 1 {
		t.Fatalf("unexpected trace: %#v err=%v", trace, err)
	}
	alerts, err := store.Alerts(ctx, nil)
	if err != nil || len(alerts) != 1 || alerts[0].ObjectName == nil || *alerts[0].ObjectName != "example-0.1" {
		t.Fatalf("unexpected alerts: %#v err=%v", alerts, err)
	}
	audit, err := store.AuditEvents(ctx, nil, true)
	if err != nil || len(audit) != 1 || len(audit[0].GroupNames) != 1 || audit[0].RemoteConfirmed == nil ||
		audit[0].ObjectName == nil || *audit[0].ObjectName != "example-0.1" {
		t.Fatalf("unexpected audit: %#v err=%v", audit, err)
	}
}

func TestHistoryOrdersMixedSourcesByActualTimeAndSourceSequence(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history-order.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	events := []string{
		`INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json) VALUES
			(100,'remote-new','2026-08-27T12:00:00Z','succeeded','remote-new','{}'),
			(-4,'local-mid','2026-08-27T11:00:00Z','succeeded','local-mid','{}'),
			(-1,'local-same-old','2026-08-27T10:00:00Z','succeeded','local-same-old','{}'),
			(-2,'local-same-new','2026-08-27T10:00:00Z','succeeded','local-same-new','{}'),
			(1,'remote-same-old','2026-08-27T09:00:00Z','succeeded','remote-same-old','{}'),
			(2,'remote-same-new','2026-08-27T09:00:00Z','succeeded','remote-same-new','{}')`,
		`INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,group_names_json,writeback,created_at) VALUES
			(100,'remote-new','sync','succeeded','readback','[]',1,'2026-08-27T12:00:00Z'),
			(-4,'local-mid','sync','succeeded','readback','[]',1,'2026-08-27T11:00:00Z'),
			(-1,'local-same-old','sync','succeeded','readback','[]',1,'2026-08-27T10:00:00Z'),
			(-2,'local-same-new','sync','succeeded','readback','[]',1,'2026-08-27T10:00:00Z'),
			(1,'remote-same-old','sync','succeeded','readback','[]',1,'2026-08-27T09:00:00Z'),
			(2,'remote-same-new','sync','succeeded','readback','[]',1,'2026-08-27T09:00:00Z')`,
	}
	for _, statement := range events {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	runtimeEvents, err := store.Events(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	auditEvents, err := store.AuditEvents(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{100, -4, -2, -1, 2, 1}
	if len(runtimeEvents) != len(want) || len(auditEvents) != len(want) {
		t.Fatalf("unexpected lengths: runtime=%d audit=%d", len(runtimeEvents), len(auditEvents))
	}
	for index := range want {
		if runtimeEvents[index].ID != want[index] || auditEvents[index].ID != want[index] {
			t.Fatalf("index %d order mismatch: runtime=%d audit=%d want=%d", index, runtimeEvents[index].ID, auditEvents[index].ID, want[index])
		}
	}
}

func TestAuditEventsIncludesConfirmedNoopRemoteReadback(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history-readback.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO operation_audit(
		source_id,operation_id,operation_type,state,phase,remote_confirmed,readback_confirmed,group_names_json,before_json,after_json,writeback,created_at
	) VALUES
		(-1,'confirmed-read','routing.writeback','succeeded','readback',0,1,'[]','{"priority":10}','{"priority":10}',0,'2026-08-28T10:00:00Z'),
		(-2,'calculation','routing.writeback','succeeded','calculation',0,0,'[]',NULL,NULL,0,'2026-08-28T10:00:01Z'),
		(-3,'false-read','routing.writeback','succeeded','readback',0,1,'[]',NULL,NULL,0,'2026-08-28T10:00:02Z')`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.AuditEvents(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].OperationID != "confirmed-read" || rows[0].Writeback {
		t.Fatalf("有效远程读取复核没有进入日志，或空值伪读回记录泄漏：%#v", rows)
	}
}

func TestClearAlertsKeepsFiringIncidentDeduplicationAndDeletesEndedAlerts(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "alerts.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO alert_incidents VALUES('active','test','host','active.example','TEST','firing','now','now','已发送',NULL);
		INSERT INTO alert_incidents VALUES('ended','test','host','ended.example','TEST','closed','now','now','已发送',NULL);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO alert_deliveries VALUES('active','qqbot:test','sent',1,NULL,'now','now');
		INSERT INTO alert_deliveries VALUES('ended','qqbot:test','sent',1,NULL,'now','now');
	`); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.ClearAlerts(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	var incidents, deliveries int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM alert_incidents`).Scan(&incidents)
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM alert_deliveries`).Scan(&deliveries)
	if incidents != 1 || deliveries != 1 {
		t.Fatalf("clear removed active alert deduplication: incidents=%d deliveries=%d", incidents, deliveries)
	}
	var incidentKey string
	if err := store.db.QueryRow(`SELECT incident_key FROM alert_incidents`).Scan(&incidentKey); err != nil || incidentKey != "active" {
		t.Fatalf("remaining incident=%q err=%v", incidentKey, err)
	}
}

func TestAlertsIncludeNotificationAttemptMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "alert-list.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO alert_incidents VALUES('active','test','host','active.example','TEST','firing','first','last','已发送',NULL);
		INSERT INTO alert_deliveries VALUES('active','qqbot:test','sent',1,NULL,'2026-08-28T15:00:00Z','2026-08-28T15:00:00Z');
	`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Alerts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DeliveryAttempts != 1 || rows[0].DeliveredAt == nil || *rows[0].DeliveredAt != "2026-08-28T15:00:00Z" {
		t.Fatalf("alert delivery metadata missing: %#v", rows)
	}
}

func TestClearLogRecordsHonorsCutoffAndProtectsRunningRuns(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "logs.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	old := "2026-07-01T00:00:00Z"
	recent := "2026-08-26T00:00:00Z"
	statements := []string{
		`INSERT INTO run_records(run_key,task_name,status,payload_json,updated_at) VALUES('old','旧任务','succeeded','{}','` + old + `'),('active','运行任务','running','{}','` + old + `'),('recent','新任务','failed','{}','` + recent + `')`,
		`INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json) VALUES(-10,'old','` + old + `','succeeded','old','{}'),(-11,'recent','` + recent + `','succeeded','recent','{}')`,
		`INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,group_names_json,writeback,created_at) VALUES(-10,'old','sync','succeeded','writeback','[]',1,'` + old + `'),(-11,'recent','sync','succeeded','writeback','[]',1,'` + recent + `')`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	result, err := store.ClearLogRecords(ctx, &cutoff)
	if err != nil || result.Runs != 1 || result.Events != 1 || result.Changes != 1 {
		t.Fatalf("unexpected cleanup: %#v err=%v", result, err)
	}
	var runs, events, changes int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM run_records`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM runtime_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM operation_audit`).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if runs != 2 || events != 1 || changes != 1 {
		t.Fatalf("remaining runs=%d events=%d changes=%d", runs, events, changes)
	}
}
