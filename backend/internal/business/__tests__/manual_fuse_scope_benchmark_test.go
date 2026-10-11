package business_test

import "testing"

func BenchmarkManualFuseEvaluationWithUnrelatedMemberships(b *testing.B) {
	store, db := manualFuseScopeStore(b)
	for _, statement := range []string{
		`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10000)
		INSERT INTO accounts(id,name,updated_at) SELECT CAST(x AS TEXT),'account','now' FROM n`,
		`WITH groups(name) AS (VALUES('a'),('b'),('c'),('d'))
		INSERT INTO account_groups(account_id,group_name) SELECT a.id,g.name FROM accounts a CROSS JOIN groups g`,
	} {
		if _, err := db.Exec(statement); err != nil {
			b.Fatal(err)
		}
	}
	setManualFuseScope(b, store, []any{"2", "10"})
	if _, err := store.EvaluateAlertIncidents(b.Context()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		result, err := store.EvaluateAlertIncidents(b.Context())
		if err != nil || result.Findings != 0 {
			b.Fatalf("evaluation=%+v err=%v", result, err)
		}
	}
}
