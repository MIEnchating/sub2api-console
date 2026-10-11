package business_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestLogCleanupUsesActualTimeAndPreservesUnparseableDates(t *testing.T) {
	for _, cutoffText := range []string{"2026-10-01T12:00:00Z", "2026-10-01T12:00:00.12Z", "2026-10-01T12:00:00.120000001Z", "2026-10-01T20:00:00.12+08:00"} {
		t.Run(cutoffText, func(t *testing.T) {
			store, db := auditRetentionStore(t)
			cutoff, err := time.Parse(time.RFC3339Nano, cutoffText)
			if err != nil {
				t.Fatal(err)
			}
			older, newer := cutoff.Add(-time.Nanosecond), cutoff.Add(time.Nanosecond)
			stamps := []string{
				older.Format(time.RFC3339Nano),
				older.In(time.FixedZone("east", 8*3600)).Format(time.RFC3339Nano),
				cutoff.Format(time.RFC3339Nano),
				cutoff.UTC().Format("2006-01-02T15:04:05.000000000Z"),
				newer.Format(time.RFC3339Nano),
				newer.In(time.FixedZone("west", -8*3600)).Format(time.RFC3339Nano),
				"", "0000-invalid", "2026-09-31T12:00:00Z",
			}
			for index, stamp := range stamps {
				id := index + 1
				if _, err := db.Exec(`INSERT INTO run_records(run_key,task_name,status,updated_at) VALUES(?,'fixture','succeeded',?)`, fmt.Sprint(id), stamp); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) VALUES(?,'fixture',?,'succeeded','fixture')`, id, stamp); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,created_at) VALUES(?,?,'fixture','succeeded','confirmed',?)`, id, fmt.Sprint(id), stamp); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`INSERT INTO run_records(run_key,task_name,status,updated_at) VALUES('active','fixture','running',?)`, older.Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
			result, err := store.ClearLogRecords(t.Context(), &cutoff)
			if err != nil {
				t.Fatal(err)
			}
			if result.Runs != 2 || result.Events != 2 || result.Changes != 2 {
				t.Fatalf("wrong cutoff counts: %+v", result)
			}
			for _, table := range []string{"run_records", "runtime_events", "operation_audit"} {
				column := "source_id"
				want := []string{"3", "4", "5", "6", "7", "8", "9"}
				if table == "run_records" {
					column = "run_key"
					want = append(want, "active")
				}
				rows, err := db.Query("SELECT CAST(" + column + " AS TEXT) FROM " + table + " ORDER BY 1")
				if err != nil {
					t.Fatal(err)
				}
				var retained []string
				for rows.Next() {
					var id string
					if err := rows.Scan(&id); err != nil {
						t.Fatal(err)
					}
					retained = append(retained, id)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				rows.Close()
				if !reflect.DeepEqual(retained, want) {
					t.Fatalf("%s retained %v, want %v", table, retained, want)
				}
			}
		})
	}
}

func TestLogCleanupSkipsRetainedDatesAndContinuesAcrossCommittedBatches(t *testing.T) {
	store, db := auditRetentionStore(t)
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<4002)
		INSERT INTO runtime_events(source_id,event_type,created_at,status,summary)
		SELECT x,'fixture',CASE WHEN x<=2001 THEN '2026-10-01T04:00:00.000000001-08:00'
		ELSE '2026-10-01T20:00:00.000000000+08:00' END,'succeeded','fixture' FROM n`); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 10, 1, 12, 0, 0, 1, time.UTC)
	var counts []int64
	result, err := store.ClearLogRecordsWithProgress(t.Context(), &cutoff, func(progress business.LogCleanupResult) error {
		counts = append(counts, progress.Events)
		return nil
	})
	if err != nil || result.Events != 2001 || !reflect.DeepEqual(counts, []int64{2000, 2001}) {
		t.Fatalf("cleanup=%+v progress=%v error=%v", result, counts, err)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events WHERE source_id<=2001`).Scan(&remaining); err != nil || remaining != 2001 {
		t.Fatalf("retained=%d error=%v", remaining, err)
	}
}

func TestLogCleanupWithoutCutoffStillRemovesUnparseableDates(t *testing.T) {
	store, db := auditRetentionStore(t)
	if _, err := db.Exec(`INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) VALUES(1,'fixture','invalid','succeeded','fixture')`); err != nil {
		t.Fatal(err)
	}
	result, err := store.ClearLogRecords(t.Context(), nil)
	if err != nil || result.Events != 1 {
		t.Fatalf("cleanup=%+v error=%v", result, err)
	}
}
