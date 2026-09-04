package business

import (
	"context"
	"testing"
)

func TestPrepareAlertDeliveryRequiresPrivateCredentialsEvenWhenRulesEnabled(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO operational_snapshots(
		namespace,state_key,value_json,updated_at
	) VALUES('sub2api','sub2api-notify-rules.json',
		'{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','2026-08-31T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO alert_incidents(
		incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at
	) VALUES('auth-1','upstream.auth','host','api.example','AUTH','firing',
		'2026-08-31T10:00:00Z','2026-08-31T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	plan, err := store.PrepareAlertDelivery(ctx, NotificationChannelKey("qqbot", ""), false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Configured || len(plan.Pending) != 0 || plan.Skipped != 1 {
		t.Fatalf("missing private credentials produced a sendable plan: %#v", plan)
	}
	var status, detail string
	if err := store.db.QueryRowContext(ctx, `SELECT delivery_status,last_error FROM alert_incidents
		WHERE incident_key='auth-1'`).Scan(&status, &detail); err != nil {
		t.Fatal(err)
	}
	if status != "未配置渠道" || detail != "QQBot 通知凭据或目标未配置完整" {
		t.Fatalf("status=%q detail=%q", status, detail)
	}
}

func TestSuccessfulMultiplierChangeNotificationClosesTheOneTimeIncident(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO operational_snapshots(
		namespace,state_key,value_json,updated_at
	) VALUES('sub2api','sub2api-notify-rules.json',
		'{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','2026-09-04T08:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO alert_incidents(
		incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at,delivery_status
	) VALUES('multiplier-up-1','account.multiplier_increased','account','41',
		'MULTIPLIER_INCREASED:0.1 -> 0.2','firing','2026-09-04T08:00:00Z','2026-09-04T08:00:00Z','未配置渠道')`); err != nil {
		t.Fatal(err)
	}
	channelKey := NotificationChannelKey("qqbot", "target")
	plan, err := store.PrepareAlertDelivery(ctx, channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pending) != 1 || plan.Pending[0].IncidentKey != "multiplier-up-1" {
		t.Fatalf("one-time multiplier notification plan=%#v", plan)
	}
	messageID := "message-1"
	if err := store.FinalizeAlertDelivery(ctx, channelKey, []AlertDeliveryOutcome{{
		IncidentKey: "multiplier-up-1", Success: true, MessageID: &messageID,
	}}); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "multiplier-up-1", "closed")
	plan, err = store.PrepareAlertDelivery(ctx, channelKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pending) != 0 {
		t.Fatalf("delivered multiplier change was queued again: %#v", plan.Pending)
	}
}

func TestAlertEvaluationLeavesOneTimeMultiplierIncidentPending(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO alert_incidents(
		incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at,delivery_status
	) VALUES('multiplier-down-1','account.multiplier_decreased','account','41',
		'MULTIPLIER_DECREASED:0.2 -> 0.1','firing','2026-09-04T08:00:00Z','2026-09-04T08:00:00Z','未配置渠道')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "multiplier-down-1", "firing")
}
