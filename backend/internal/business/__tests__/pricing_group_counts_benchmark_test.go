package business_test

import "testing"

func BenchmarkSyncPricingGroupCounts(b *testing.B) {
	store, db := pricingProjectionFixture(b)
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<200)
		INSERT INTO local_groups(name,remote_id,account_count,updated_at) SELECT printf('group-%d',x),CAST(x AS TEXT),10,'before' FROM n;
		WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000)
		INSERT INTO accounts(id,name,multiplier,updated_at) SELECT CAST(x AS TEXT),printf('account-%d',x),'0.1','before' FROM n;
		INSERT INTO account_groups(account_id,group_name,group_id)
		SELECT id,printf('group-%d',((CAST(id AS INTEGER)-1)%200)+1),CAST(((CAST(id AS INTEGER)-1)%200)+1 AS TEXT) FROM accounts`); err != nil {
		b.Fatal(err)
	}
	changes := map[string][]string{"1": {"2"}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.SyncPricingAccountGroups(b.Context(), changes, "benchmark"); err != nil {
			b.Fatal(err)
		}
		if changes["1"][0] == "2" {
			changes["1"][0] = "1"
		} else {
			changes["1"][0] = "2"
		}
	}
	var memberships int
	if err := db.QueryRow(`SELECT SUM(account_count) FROM local_groups`).Scan(&memberships); err != nil || memberships != 2000 {
		b.Fatalf("memberships=%d error=%v", memberships, err)
	}
}
