package business_test

import (
	"strings"
	"testing"
)

func BenchmarkMaintenanceSingleAccount(b *testing.B) {
	store, db := maintenanceScopeFixture(b)
	metadata := `{"platform":"openai","notes":"` + strings.Repeat("fixture ", 1024) + `"}`
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(100) UNION ALL SELECT x+1 FROM n WHERE x<1096)
	INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at) SELECT CAST(x AS TEXT),'bulk','0.1',?,'now' FROM n`, metadata); err != nil {
		b.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
		SELECT id,'maintenance.example','key-'||id,'fixture','default','now' FROM accounts WHERE CAST(id AS INTEGER)>=100`,
		`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
		SELECT id,'fixture-upstream',upstream_key_id,'now' FROM bindings WHERE local_account_id NOT IN ('2','10')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			b.Fatal(err)
		}
	}
	for _, kind := range []string{"names", "bindings"} {
		b.Run(kind, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if kind == "names" {
					rows, err := store.AccountNamesForMaintenance(b.Context(), []string{"2"})
					if err != nil || len(rows) != 1 || rows["2"] != "same-name" {
						b.Fatalf("names=%v err=%v", rows, err)
					}
				} else {
					rows, err := store.BoundAccountsForMaintenance(b.Context(), []string{"2"})
					if err != nil || len(rows) != 2 || rows[0].AccountID != "2" {
						b.Fatalf("bindings=%+v err=%v", rows, err)
					}
				}
			}
		})
	}
}
