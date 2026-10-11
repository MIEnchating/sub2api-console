package business_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestNotificationQueueCountsPreserveDeliveryStates(t *testing.T) {
	for _, scenario := range []struct {
		name                           string
		configured, delivery, recovery bool
		want                           business.NotificationQueueSnapshot
	}{
		{"configured", true, true, true, business.NotificationQueueSnapshot{ProducerFiring: 5, ProducerRecovered: 1, ConsumerPending: 3, ConsumerFailed: 2}},
		{"private configuration missing", false, true, true, business.NotificationQueueSnapshot{ProducerFiring: 5, ProducerRecovered: 1}},
		{"delivery disabled", true, false, true, business.NotificationQueueSnapshot{ProducerFiring: 5, ProducerRecovered: 1}},
		{"recovery disabled", true, true, false, business.NotificationQueueSnapshot{ProducerFiring: 5, ProducerRecovered: 1, ConsumerPending: 2, ConsumerFailed: 2}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := openAlertRuleStore(t)
			policy := business.DefaultAlertPolicy()
			policy.DeliveryEnabled, policy.NotifyRecovery = scenario.delivery, scenario.recovery
			updateAlertRulePolicy(t, store, policy)
			if _, err := db.Exec(`INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at)
				VALUES('sub2api','sub2api-notify-rules.json','{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','now')`); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			for _, item := range []struct{ key, status, event, delivery string }{
				{"new", "firing", "upstream.auth", ""},
				{"retry", "firing", "upstream.auth", "failed"},
				{"unknown", "firing", "upstream.auth", "commit_unknown"},
				{"sent", "firing", "upstream.auth", "sent"},
				{"recovered", "recovered", "upstream.auth", ""},
				{"observing", "firing", "account.routing_degraded", ""},
				{"closed", "closed", "upstream.auth", ""},
			} {
				if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
					VALUES(?,?,'host','fixture.example','AUTH',?,?,?)`, item.key, item.event, item.status, now, now); err != nil {
					t.Fatal(err)
				}
				if item.delivery != "" {
					if _, err := db.Exec(`INSERT INTO alert_deliveries(incident_key,channel_key,status,attempts,delivered_at,updated_at)
						VALUES(?,'fixture-channel',?,1,?,?)`, item.key, item.delivery, now, now); err != nil {
						t.Fatal(err)
					}
				}
			}
			snapshot, err := store.NotificationQueueSnapshot(t.Context(), "fixture-channel", scenario.configured)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot != scenario.want {
				t.Fatalf("snapshot=%+v want=%+v", snapshot, scenario.want)
			}
			details, err := store.NotificationQueueDetails(t.Context(), "fixture-channel", scenario.configured)
			if err != nil {
				t.Fatal(err)
			}
			counts := business.NotificationQueueSnapshot{ProducerFiring: len(details.ProducerFiring), ProducerRecovered: len(details.ProducerRecovered), ConsumerPending: len(details.ConsumerPending), ConsumerFailed: len(details.ConsumerFailed)}
			if snapshot != counts || len(details.ConsumerItems) != 6 {
				t.Fatalf("snapshot and detail contract disagree: %+v %+v", snapshot, details)
			}
		})
	}
}

func TestNotificationQueueReadsLeaveDatabaseStateUnchanged(t *testing.T) {
	store, db := openAlertRuleStore(t)
	if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at,delivery_status,last_error)
		VALUES('retry','upstream.auth','host','fixture.example','AUTH','firing','now','now','发送失败','fixture failure');
		INSERT INTO alert_deliveries(incident_key,channel_key,status,attempts,updated_at) VALUES('retry','fixture-channel','failed',2,'now')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := store.NotificationQueueSnapshot(t.Context(), "fixture-channel", true); err != nil {
			t.Fatal(err)
		}
		if _, err := store.NotificationQueueDetails(t.Context(), "fixture-channel", true); err != nil {
			t.Fatal(err)
		}
	}
	var status, detail, delivery string
	var attempts int
	if err := db.QueryRow(`SELECT i.delivery_status,i.last_error,d.status,d.attempts FROM alert_incidents i JOIN alert_deliveries d USING(incident_key)`).Scan(&status, &detail, &delivery, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "发送失败" || detail != "fixture failure" || delivery != "failed" || attempts != 2 {
		t.Fatalf("queue reads changed persisted delivery state: %s %s %s %d", status, detail, delivery, attempts)
	}
}

func TestNotificationQueueCountsPreserveCooldownsAndRepeats(t *testing.T) {
	store, db := openAlertRuleStore(t)
	policy := business.DefaultAlertPolicy()
	policy.RepeatIntervalMinutes = 60
	updateAlertRulePolicy(t, store, policy)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	old := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at)
		VALUES('sub2api','sub2api-notify-rules.json','{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at) VALUES
		('transition','account.routing_breaker','account','41','ROUTING_BREAKER','firing',?,?),
		('repeat','upstream.auth','host','fixture.example','AUTH','firing',?,?),
		('digest','account.routing_degraded','account','42','ROUTING_DEGRADED','firing',?,?),
		('historical-digest','account.routing_degraded','account','43','ROUTING_DEGRADED','closed',?,?),
		('self-recovered','account.routing_degraded','account','44','ROUTING_DEGRADED','recovered',?,?)`,
		old, now, old, now, old, now, old, now, old, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_deliveries(incident_key,channel_key,status,attempts,delivered_at,updated_at) VALUES
		('transition','fixture-channel','transition',1,NULL,?),
		('repeat','fixture-channel','sent',1,?,?),
		('historical-digest','fixture-channel','sent',1,?,?)`,
		now, old, old, now, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.NotificationQueueSnapshot(t.Context(), "fixture-channel", true)
	if err != nil {
		t.Fatal(err)
	}
	want := business.NotificationQueueSnapshot{ProducerFiring: 3, ProducerRecovered: 1, ConsumerPending: 1}
	if snapshot != want {
		t.Fatalf("snapshot=%+v want=%+v", snapshot, want)
	}
	details, err := store.NotificationQueueDetails(t.Context(), "fixture-channel", true)
	if err != nil {
		t.Fatal(err)
	}
	wantStates := map[string]string{"transition": "状态变化冷却中", "repeat": "待发送", "digest": "降级告警汇总冷却中", "self-recovered": "无需发送"}
	if len(details.ConsumerItems) != len(wantStates) {
		t.Fatalf("wrong detail count: %+v", details)
	}
	for _, item := range details.ConsumerItems {
		if item.QueueStatus != wantStates[item.IncidentKey] {
			t.Fatalf("%s status=%s want=%s", item.IncidentKey, item.QueueStatus, wantStates[item.IncidentKey])
		}
	}
}

func TestNotificationQueueEmptyAndCancelledReads(t *testing.T) {
	store, _ := openAlertRuleStore(t)
	details, err := store.NotificationQueueDetails(t.Context(), "fixture-channel", true)
	if err != nil {
		t.Fatal(err)
	}
	want := business.NotificationQueueDetails{ProducerFiring: []business.NotificationQueueItem{}, ProducerRecovered: []business.NotificationQueueItem{}, ConsumerPending: []business.NotificationQueueItem{}, ConsumerFailed: []business.NotificationQueueItem{}, ConsumerItems: []business.NotificationQueueItem{}}
	if !reflect.DeepEqual(details, want) {
		t.Fatalf("empty queue must expose arrays: %+v", details)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.NotificationQueueSnapshot(ctx, "fixture-channel", true); err == nil {
		t.Fatal("cancelled snapshot succeeded")
	}
	if _, err := store.NotificationQueueDetails(ctx, "fixture-channel", true); err == nil {
		t.Fatal("cancelled details succeeded")
	}
}
