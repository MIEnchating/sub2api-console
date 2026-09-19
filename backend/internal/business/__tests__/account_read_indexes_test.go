package business_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestAccountHistoryReadsUseCoveringIndexesOnFreshAndExistingDatabases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account-read.db")
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
	for _, existing := range []bool{false, true} {
		if existing {
			if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','preserved','now');
    DROP INDEX IF EXISTS ix_account_stability_covering;
    DROP INDEX IF EXISTS ix_health_samples_account_window;
    DROP INDEX IF EXISTS ix_health_samples_source_window`); err != nil {
				t.Fatal(err)
			}
			reopened, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := reopened.Close(); err != nil {
				t.Fatal(err)
			}
			var name string
			if err := db.QueryRow(`SELECT name FROM accounts WHERE id='41'`).Scan(&name); err != nil || name != "preserved" {
				t.Fatalf("account changed during upgrade: %q %v", name, err)
			}
		}
		for _, q := range []string{
			`SELECT account_id,outcome,COUNT(*),SUM(CASE WHEN observed_at>='2026-09-18' THEN 1 ELSE 0 END) FROM account_stability_samples WHERE observed_at>='2026-08-19' AND observed_at<='2026-09-19' AND account_id IN ('41','42') GROUP BY account_id,outcome`,
			`SELECT id,account_id,observed_at,source,evidence_key FROM health_samples INDEXED BY ix_health_samples_account_window WHERE account_id='41' AND source<>'account-state' ORDER BY account_id,observed_at DESC,id DESC`,
			`SELECT id,account_id,observed_at,source,evidence_key FROM health_samples INDEXED BY ix_health_samples_source_window WHERE account_id='41' AND LOWER(REPLACE(source,'_','-'))='traffic' ORDER BY account_id,observed_at DESC,id DESC`,
		} {
			rows, err := db.Query("EXPLAIN QUERY PLAN " + q)
			if err != nil {
				t.Fatal(err)
			}
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
			rows.Close()
			if !strings.Contains(plan.String(), "COVERING INDEX") {
				t.Fatalf("existing=%t: account history requires table lookups: %s", existing, plan.String())
			}
		}
	}
}
