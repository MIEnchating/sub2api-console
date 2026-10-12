package business_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"testing"
)

func TestDatabaseContractAccountCostAndStabilityConstraints(t *testing.T) {
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
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			for _, query := range []string{
				`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES('contract-41','fixture','0.1234567890123456789','2026-10-12')`,
				`INSERT INTO account_groups(account_id,group_name,group_rate) VALUES('contract-41','contract','9')`,
				`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,local_rate,updated_at) VALUES('contract-41','contract.test','key','fixture','contract','9','2026-10-12')`,
			} {
				if _, err := tx.ExecContext(t.Context(), query); err != nil {
					t.Fatal(err)
				}
			}
			assertCost := func(want string) {
				t.Helper()
				for _, query := range []string{`SELECT group_rate FROM account_groups WHERE account_id='contract-41'`, `SELECT local_rate FROM bindings WHERE local_account_id='contract-41'`} {
					var got string
					if err := tx.QueryRowContext(t.Context(), query).Scan(&got); err != nil || got != want {
						t.Fatalf("cost propagation: %q %v", got, err)
					}
				}
			}
			assertCost("0.1234567890123456789")
			if _, err := tx.ExecContext(t.Context(), `UPDATE accounts SET multiplier='0.25' WHERE id='contract-41'`); err != nil {
				t.Fatal(err)
			}
			assertCost("0.25")
			var before, after int64
			if err := tx.QueryRowContext(t.Context(), `SELECT revision FROM account_stability_version WHERE id=1`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(t.Context(), `INSERT INTO account_stability_samples(account_id,source,evidence_key,observed_at,outcome) VALUES('contract-41','probe','fixture','2026-10-12','passed')`); err != nil {
				t.Fatal(err)
			}
			if err := tx.QueryRowContext(t.Context(), `SELECT revision FROM account_stability_version WHERE id=1`).Scan(&after); err != nil || after != before+1 {
				t.Fatalf("cache invalidation: %d -> %d %v", before, after, err)
			}
		})
	}
}
