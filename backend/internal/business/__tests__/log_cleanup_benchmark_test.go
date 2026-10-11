package business_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkLogCleanupEvents(b *testing.B) {
	for range b.N {
		b.StopTimer()
		path := filepath.Join(b.TempDir(), "business.db")
		store, err := business.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		if err := store.Bootstrap(b.Context()); err != nil {
			b.Fatal(err)
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000)
			INSERT INTO runtime_events(source_id,event_type,created_at,status,summary)
			SELECT x,'fixture','2000-01-01T00:00:00Z','succeeded','expired' FROM n`); err != nil {
			b.Fatal(err)
		}
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		result, err := store.ClearLogRecords(b.Context(), &cutoff)
		b.StopTimer()
		if err != nil || result.Events != 20000 {
			b.Fatalf("cleanup=%+v err=%v", result, err)
		}
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLogCleanupAudit(b *testing.B) {
	for range b.N {
		b.StopTimer()
		path := filepath.Join(b.TempDir(), "business.db")
		store, err := business.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		if err := store.Bootstrap(b.Context()); err != nil {
			b.Fatal(err)
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000)
			INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,object_id,created_at)
			SELECT x,printf('operation-%d',x),'routing.writeback','succeeded','confirmed',printf('account-%d',x%100),
			'2000-01-01T00:00:00Z' FROM n`); err != nil {
			b.Fatal(err)
		}
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		result, err := store.ClearLogRecords(b.Context(), &cutoff)
		b.StopTimer()
		if err != nil || result.Changes != 20000 {
			b.Fatalf("cleanup=%+v err=%v", result, err)
		}
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
