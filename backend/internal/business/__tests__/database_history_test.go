package business_test

import (
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractLogRetentionPreservesBoundaryUnknownDatesAndActiveRuns(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, query := range []string{
				`INSERT INTO run_records(run_key,task_name,status,updated_at) VALUES('cleanup-old','fixture','succeeded','2020-01-01T00:00:00Z'),('cleanup-active','fixture','running','2020-01-01T00:00:00Z')`,
				`INSERT INTO runtime_events(source_id,event_type,status,summary,payload_json,created_at) VALUES(99101,'fixture','succeeded','old','{}','2020-01-01T08:00:00.123456788+08:00'),(99102,'fixture','succeeded','boundary','{}','2020-01-01T00:00:00.123456789Z'),(99103,'fixture','succeeded','unknown','{}','unknown')`,
				`INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,created_at) VALUES(99101,'fixture','contract','succeeded','commit','2020-01-01T00:00:00Z')`,
			} {
				if _, err := db.ExecContext(t.Context(), query); err != nil {
					t.Fatal(err)
				}
			}
			defer db.ExecContext(t.Context(), `DELETE FROM run_records WHERE run_key IN ('cleanup-old','cleanup-active')`)
			defer db.ExecContext(t.Context(), `DELETE FROM runtime_events WHERE source_id IN (99101,99102,99103)`)
			defer db.ExecContext(t.Context(), `DELETE FROM operation_audit WHERE source_id=99101`)
			cutoff := time.Date(2020, 1, 1, 0, 0, 0, 123456789, time.UTC)
			result, err := store.ClearLogRecords(t.Context(), &cutoff)
			if err != nil || result.Runs != 1 || result.Events != 1 || result.Changes != 1 {
				t.Fatalf("cleanup: %+v %v", result, err)
			}
			var retained int
			if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM runtime_events WHERE source_id IN (99102,99103)`).Scan(&retained); err != nil || retained != 2 {
				t.Fatalf("boundary/unknown removed: %d %v", retained, err)
			}
			if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM run_records WHERE run_key='cleanup-active'`).Scan(&retained); err != nil || retained != 1 {
				t.Fatalf("active run removed: %d %v", retained, err)
			}
		})
	}
}

func TestDatabaseContractHistorySearchTreatsWildcardsLiterally(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			id, err := store.RecordRuntimeEvent(t.Context(), "CONTRACT_%", "succeeded", "literal search", map[string]any{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.RecordRuntimeEvent(t.Context(), "CONTRACT_AX", "succeeded", "nonmatching", map[string]any{}); err != nil {
				t.Fatal(err)
			}
			events, err := store.SearchEvents(t.Context(), "contract_%", nil)
			if err != nil || len(events) != 1 || events[0].ID != id {
				t.Fatalf("literal search: %+v %v", events, err)
			}
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, query := range []string{
				`INSERT INTO run_records(run_key,task_name,status,updated_at) VALUES('search-literal','CONTRACT_%','succeeded','2026-10-12T00:00:00Z'),('search-other','CONTRACT_AX','succeeded','2026-10-12T00:00:00Z')`,
				`INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,created_at) VALUES(99201,'CONTRACT_%','contract','succeeded','commit','2026-10-12T00:00:00Z'),(99202,'CONTRACT_AX','contract','succeeded','commit','2026-10-12T00:00:00Z')`,
			} {
				if _, err := db.ExecContext(t.Context(), query); err != nil {
					t.Fatal(err)
				}
			}
			defer db.ExecContext(t.Context(), `DELETE FROM run_records WHERE run_key IN ('search-literal','search-other')`)
			defer db.ExecContext(t.Context(), `DELETE FROM operation_audit WHERE source_id IN (99201,99202)`)
			runs, err := store.SearchRunRecords(t.Context(), "contract_%", nil)
			if err != nil || len(runs) != 1 || runs[0].RunKey != "search-literal" {
				t.Fatalf("literal run search: %+v %v", runs, err)
			}
			audits, err := store.SearchAuditEvents(t.Context(), "contract_%", nil)
			if err != nil || len(audits) != 1 || audits[0].ID != 99201 {
				t.Fatalf("literal audit search: %+v %v", audits, err)
			}
		})
	}
}
