package business_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestGroupCountsUseCoveringIndexWithoutTemporarySorting(t *testing.T) {
	_, db := pricingProjectionFixture(t)
	for _, scope := range []struct{ name, query string }{
		{"stable", `SELECT group_id,COUNT(*) FROM account_groups WHERE group_id IS NOT NULL GROUP BY group_id`},
		{"legacy", `SELECT group_name,COUNT(*) FROM account_groups WHERE group_id IS NULL GROUP BY group_name`},
	} {
		t.Run(scope.name, func(t *testing.T) {
			rows, err := db.Query("EXPLAIN QUERY PLAN " + scope.query)
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
			if !strings.Contains(plan.String(), "COVERING INDEX ix_account_groups_membership_counts") || strings.Contains(plan.String(), "TEMP B-TREE") {
				t.Fatalf("group counts require table lookups or temporary sorting: %s", plan.String())
			}
		})
	}
}

func TestExistingDatabaseAddsGroupCountIndexAndPreservesMemberships(t *testing.T) {
	path := filepath.Join(t.TempDir(), "membership-upgrade.db")
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
	if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES('41','preserved','0.1','before');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','renamed','7'),('41','legacy',NULL);
		DROP INDEX IF EXISTS ix_account_groups_membership_counts; PRAGMA user_version=8`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		store, err = business.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var indexes, memberships int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name='ix_account_groups_membership_counts' AND type='index'`).Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("membership index after reopen=%d error=%v", indexes, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM account_groups WHERE account_id='41' AND group_rate='0.1'
		AND ((group_name='renamed' AND group_id='7') OR (group_name='legacy' AND group_id IS NULL))`).Scan(&memberships); err != nil || memberships != 2 {
		t.Fatalf("upgrade changed memberships: count=%d error=%v", memberships, err)
	}
}

func TestGroupCountIndexCreationFailureRollsBackEarlierDDLAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "membership-conflict.db")
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
	if _, err := db.Exec(`DROP INDEX IF EXISTS ix_account_groups_membership_counts;
		DROP INDEX ix_upstream_identity_hosts_identity;
		CREATE TABLE ix_account_groups_membership_counts(id INTEGER);
		PRAGMA user_version=8`); err != nil {
		t.Fatal(err)
	}
	if store, err := business.Open(path); err == nil {
		store.Close()
		t.Fatal("accepted conflicting membership index name")
	}
	var indexes, version int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name='ix_upstream_identity_hosts_identity'`).Scan(&indexes); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if indexes != 0 || version != 8 {
		t.Fatalf("failed upgrade escaped rollback: indexes=%d version=%d", indexes, version)
	}
}
