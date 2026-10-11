package business_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkConfirmedGroupReadbackCounts(b *testing.B) {
	for _, indexed := range []bool{false, true} {
		b.Run(fmt.Sprintf("membership-index=%t", indexed), func(b *testing.B) {
			benchmarkConfirmedGroupReadbackCounts(b, indexed)
		})
	}
}

func groupCountBenchmarkFixture(b *testing.B, indexed bool) (*business.Store, *sql.DB) {
	b.Helper()
	store, db := pricingProjectionFixture(b)
	if _, err := db.Exec(`DROP INDEX IF EXISTS ix_account_groups_membership_counts`); err != nil {
		b.Fatal(err)
	}
	if indexed {
		if _, err := db.Exec(`CREATE INDEX ix_account_groups_membership_counts ON account_groups(group_id,group_name)`); err != nil {
			b.Fatal(err)
		}
	}
	for _, statement := range []string{
		`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<200)
		INSERT INTO local_groups(name,remote_id,account_count,updated_at)
		SELECT 'group-'||x,CAST(x AS TEXT),10,'before' FROM n`,
		`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000)
		INSERT INTO accounts(id,name,multiplier,updated_at) SELECT CAST(x AS TEXT),'account-'||x,'0.1','before' FROM n`,
		`INSERT INTO account_groups(account_id,group_name,group_id)
		SELECT id,'group-'||((CAST(id AS INTEGER)-1)%200+1),CAST((CAST(id AS INTEGER)-1)%200+1 AS TEXT) FROM accounts`,
	} {
		if _, err := db.Exec(statement); err != nil {
			b.Fatal(err)
		}
	}
	return store, db
}

func benchmarkConfirmedGroupReadbackCounts(b *testing.B, indexed bool) {
	store, _ := groupCountBenchmarkFixture(b, indexed)
	operation := business.AccountOperation{
		OperationID: "readback-benchmark", OperationType: "account.groups.sync", State: "succeeded", Phase: "readback",
		ObjectID: "1", Actor: "test", RemoteConfirmed: true, ReadbackConfirmed: true,
	}
	groups := [][]business.LocalOnboardingGroup{
		{{ID: "2", Name: "group-2"}}, {{ID: "1", Name: "group-1"}},
	}
	index := 0
	b.ReportAllocs()
	for b.Loop() {
		if err := store.CommitAccountGroupsReadback(b.Context(), "1", groups[index%2], nil, operation); err != nil {
			b.Fatal(err)
		}
		index++
	}
}

// Isolate index maintenance during a full membership replacement, including
// the existing constraints and cost triggers, without group-count reads.
func BenchmarkGroupMembershipIndexWrites(b *testing.B) {
	for _, indexed := range []bool{false, true} {
		b.Run(fmt.Sprintf("membership-index=%t", indexed), func(b *testing.B) {
			_, db := groupCountBenchmarkFixture(b, indexed)
			b.ReportAllocs()
			for b.Loop() {
				tx, err := db.BeginTx(b.Context(), nil)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := tx.Exec(`DELETE FROM account_groups`); err != nil {
					_ = tx.Rollback()
					b.Fatal(err)
				}
				if _, err := tx.Exec(`INSERT INTO account_groups(account_id,group_name,group_id)
					SELECT id,'group-'||((CAST(id AS INTEGER)-1)%200+1),CAST((CAST(id AS INTEGER)-1)%200+1 AS TEXT) FROM accounts`); err != nil {
					_ = tx.Rollback()
					b.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
