package business_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestRunLogCleanupUsesTerminalIndexWithAndWithoutCutoff(t *testing.T) {
	_, db := auditRetentionStore(t)
	for _, cutoff := range []bool{false, true} {
		t.Run(map[bool]string{false: "all", true: "expired"}[cutoff], func(t *testing.T) {
			query := `EXPLAIN QUERY PLAN DELETE FROM run_records WHERE rowid IN (
				SELECT rowid FROM run_records WHERE 1=1
				AND COALESCE(status,'') NOT IN ('queued','running','waiting_input')`
			args := []any{}
			if cutoff {
				query += " AND updated_at < ? AND console_log_before(updated_at,?,?)"
				args = append(args, "2026-02-03T00:00:00Z", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).Unix(), 0)
			}
			query += " ORDER BY updated_at LIMIT 1000)"
			rows, err := db.Query(query, args...)
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
			if !strings.Contains(plan.String(), "ix_run_records_log_cleanup") || strings.Contains(plan.String(), "TEMP B-TREE") {
				t.Fatalf("cleanup scans active records or sorts candidates: %s", plan.String())
			}
		})
	}
}

func TestReopenAddsRunCleanupIndexAndKeepsActiveAndRecentRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-cleanup.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP INDEX IF EXISTS ix_run_records_log_cleanup;
		CREATE INDEX IF NOT EXISTS ix_run_records_updated_at ON run_records(updated_at);
		INSERT INTO run_records(run_key,task_name,status,updated_at) VALUES
		('queued','fixture','queued','2026-01-01T00:00:00Z'),
		('running','fixture','running','2026-01-01T00:00:00Z'),
		('waiting','fixture','waiting_input','2026-01-01T00:00:00Z'),
		('legacy','fixture',NULL,'2026-01-01T00:00:00Z'),
		('failed','fixture','failed','2026-01-01T00:00:00Z'),
		('succeeded','fixture','succeeded','2026-01-01T00:00:00Z'),
		('recent','fixture','succeeded','2026-02-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var indexes, records int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name='ix_run_records_log_cleanup'`).Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("cleanup index not repaired: count=%d err=%v", indexes, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name='ix_run_records_updated_at'`).Scan(&indexes); err != nil || indexes != 0 {
		t.Fatalf("replaced cleanup index retained: count=%d err=%v", indexes, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM run_records`).Scan(&records); err != nil || records != 7 {
		t.Fatalf("reopen changed history: count=%d err=%v", records, err)
	}
	cutoff := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	result, err := store.ClearLogRecords(t.Context(), &cutoff)
	if err != nil || result.Runs != 3 || result.Events != 0 || result.Changes != 0 {
		t.Fatalf("cleanup counts=%+v err=%v", result, err)
	}
	rows, err := db.Query(`SELECT run_key FROM run_records ORDER BY run_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var retained []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		retained = append(retained, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(retained, ","); got != "queued,recent,running,waiting" {
		t.Fatalf("retained records=%s", got)
	}
}
