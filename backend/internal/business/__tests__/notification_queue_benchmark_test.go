package business_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkNotificationQueueSnapshot(b *testing.B) {
	for _, size := range []int{20, 2000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "notification-queue.db")
			store, err := business.Open(path)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(b.Context()); err != nil {
				b.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = db.Close() })
			tx, err := db.Begin()
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at)
				VALUES('sub2api','sub2api-notify-rules.json','{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','now')`); err != nil {
				b.Fatal(err)
			}
			for index := range size {
				if _, err := tx.Exec(`INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at)
					VALUES(?,'upstream.auth','host','fixture.example','AUTH','firing','now','now')`, fmt.Sprintf("incident-%04d", index)); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				snapshot, err := store.NotificationQueueSnapshot(b.Context(), "fixture-channel", true)
				if err != nil {
					b.Fatal(err)
				}
				if snapshot.ProducerFiring != size || snapshot.ConsumerPending != size {
					b.Fatalf("wrong count: %+v", snapshot)
				}
			}
		})
	}
}
