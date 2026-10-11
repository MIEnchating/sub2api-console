package business_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestLogCleanupRetentionCancellationAcrossTablesKeepsCommittedBatchAndAllowsRetry(t *testing.T) {
	for _, fixture := range []struct {
		table, insert string
		count         func(business.LogCleanupResult) int64
	}{
		{"run_records", `INSERT INTO run_records(run_key,task_name,status,updated_at)
			SELECT printf('run-%d',x),'fixture','succeeded',stamp FROM dates`, func(r business.LogCleanupResult) int64 { return r.Runs }},
		{"runtime_events", `INSERT INTO runtime_events(source_id,event_type,status,summary,created_at)
			SELECT x,'fixture','succeeded','fixture',stamp FROM dates`, func(r business.LogCleanupResult) int64 { return r.Events }},
		{"operation_audit", `INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,created_at)
			SELECT x,printf('operation-%d',x),'fixture','succeeded','confirmed',stamp FROM dates`, func(r business.LogCleanupResult) int64 { return r.Changes }},
	} {
		t.Run(fixture.table, func(t *testing.T) {
			store, db := auditRetentionStore(t)
			if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<4003),
				dates AS (SELECT x,CASE WHEN x=1 THEN '0000-invalid'
				WHEN x=2 THEN '2026-10-01T04:00:00-08:00' ELSE '2026-01-01T00:00:00Z' END stamp FROM n) ` + fixture.insert); err != nil {
				t.Fatal(err)
			}
			cutoff := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result, err := store.ClearLogRecordsWithProgress(ctx, &cutoff, func(progress business.LogCleanupResult) error {
				if fixture.count(progress) != 2000 {
					return fmt.Errorf("unexpected batch: %+v", progress)
				}
				var remaining int
				if err := db.QueryRow("SELECT COUNT(*) FROM " + fixture.table).Scan(&remaining); err != nil {
					return err
				}
				if remaining != 2003 {
					return fmt.Errorf("progress preceded commit: remaining=%d", remaining)
				}
				cancel()
				return nil
			})
			if !errors.Is(err, context.Canceled) || fixture.count(result) != 2000 {
				t.Fatalf("cancelled cleanup=%+v error=%v", result, err)
			}
			result, err = store.ClearLogRecords(t.Context(), &cutoff)
			if err != nil || fixture.count(result) != 2001 {
				t.Fatalf("retry=%+v error=%v", result, err)
			}
			var retained int
			if err := db.QueryRow("SELECT COUNT(*) FROM " + fixture.table).Scan(&retained); err != nil || retained != 2 {
				t.Fatalf("retained=%d error=%v", retained, err)
			}
		})
	}
}

func TestLogCleanupDefersEarlierBackfillButFinishesEqualTimestampAfterCommittedBatch(t *testing.T) {
	store, db := auditRetentionStore(t)
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2001)
		INSERT INTO runtime_events(source_id,event_type,created_at,status,summary)
		SELECT x,'fixture','2026-01-02T00:00:00Z','succeeded','expired' FROM n`); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	result, err := store.ClearLogRecordsWithProgress(t.Context(), &cutoff, func(progress business.LogCleanupResult) error {
		if progress.Events != 2000 {
			return nil
		}
		// Another writer can commit after the reported batch. Earlier backfill
		// waits for the next cleanup; equal timestamps must not be skipped.
		_, err := db.Exec(`INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) VALUES
			(3000,'fixture','2026-01-01T00:00:00Z','succeeded','earlier backfill'),
			(3001,'fixture','2026-01-02T00:00:00Z','succeeded','same timestamp')`)
		return err
	})
	if err != nil || result.Events != 2002 {
		t.Fatalf("cleanup=%+v error=%v; want 2002 committed deletions", result, err)
	}
	var retainedID int
	if err := db.QueryRow(`SELECT source_id FROM runtime_events`).Scan(&retainedID); err != nil || retainedID != 3000 {
		t.Fatalf("retained backfill=%d error=%v", retainedID, err)
	}
	result, err = store.ClearLogRecords(t.Context(), &cutoff)
	if err != nil || result.Events != 1 {
		t.Fatalf("next cleanup=%+v error=%v", result, err)
	}
}

func TestLogCleanupFailedCursorBatchRollsBackAndRetries(t *testing.T) {
	store, db := auditRetentionStore(t)
	if _, err := db.Exec(`INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) VALUES
		(1,'fixture','2026-01-01T00:00:00Z','succeeded','expired'),
		(2,'fixture','2026-01-01T00:00:00Z','succeeded','expired');
		CREATE TRIGGER fail_event_cleanup BEFORE DELETE ON runtime_events
		WHEN OLD.source_id=2 BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	result, err := store.ClearLogRecordsWithProgress(t.Context(), &cutoff, func(business.LogCleanupResult) error {
		t.Fatal("failed batch reported progress")
		return nil
	})
	if err == nil || result.Events != 0 {
		t.Fatalf("failed cleanup=%+v error=%v", result, err)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events`).Scan(&remaining); err != nil || remaining != 2 {
		t.Fatalf("rollback retained=%d error=%v", remaining, err)
	}
	if _, err := db.Exec(`DROP TRIGGER fail_event_cleanup`); err != nil {
		t.Fatal(err)
	}
	result, err = store.ClearLogRecords(t.Context(), &cutoff)
	if err != nil || result.Events != 2 {
		t.Fatalf("retry=%+v error=%v", result, err)
	}
}

func TestLogCleanupCursorUsesExistingTimeIndexBoundsWithoutSorting(t *testing.T) {
	_, db := auditRetentionStore(t)
	for _, fixture := range []struct{ table, column, extra string }{
		{"run_records", "updated_at", " AND COALESCE(status,'') NOT IN ('queued','running','waiting_input')"},
		{"runtime_events", "created_at", ""},
		{"operation_audit", "created_at", ""},
	} {
		t.Run(fixture.table, func(t *testing.T) {
			query := "EXPLAIN QUERY PLAN SELECT rowid," + fixture.column + " AS stamp FROM " + fixture.table +
				" WHERE 1=1" + fixture.extra + " AND " + fixture.column + " < ? AND console_log_before(" + fixture.column +
				",?,?) AND " + fixture.column + " >= ? ORDER BY " + fixture.column + " LIMIT 2000"
			rows, err := db.Query(query, "2026-10-03T00:00:00Z", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Unix(), 0, "2026-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plan strings.Builder
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plan.WriteString(detail)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(plan.String(), fixture.column+">?") || !strings.Contains(plan.String(), fixture.column+"<?") || strings.Contains(plan.String(), "TEMP B-TREE") {
				t.Fatalf("cursor must seek within time index without sorting: %s", plan.String())
			}
		})
	}
}
