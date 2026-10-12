package business_test

import (
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractMaintenanceAndDeletePreviewPreserveNumericOrder(t *testing.T) {
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
			for _, statement := range []string{
				`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES('maintenance.contract.test','https://maintenance.contract.test','sub2api','ready','{}','now')`,
				`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('maintenance-contract','now','now')`,
				`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('maintenance.contract.test','maintenance-contract',1,'now')`,
				`INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at) VALUES('902','first','0.1','{}','now'),('9010','second','0.2','{}','now')`,
				`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) VALUES('902','maintenance.contract.test','first','fixture','default','now'),('902','maintenance.contract.test','second','fixture','default','now'),('9010','maintenance.contract.test','third','fixture','default','now')`,
				`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at) SELECT id,'maintenance-contract',upstream_key_id,'now' FROM bindings WHERE upstream_host='maintenance.contract.test'`,
			} {
				if _, err := db.ExecContext(t.Context(), statement); err != nil {
					t.Fatal(err)
				}
			}
			defer func() {
				for _, statement := range []string{
					`DELETE FROM binding_identities WHERE upstream_id='maintenance-contract'`,
					`DELETE FROM bindings WHERE upstream_host='maintenance.contract.test'`,
					`DELETE FROM accounts WHERE id IN ('902','9010')`,
					`DELETE FROM upstream_identity_hosts WHERE upstream_id='maintenance-contract'`,
					`DELETE FROM upstream_identities WHERE upstream_id='maintenance-contract'`,
					`DELETE FROM upstreams WHERE host='maintenance.contract.test'`,
				} {
					if _, err := db.ExecContext(t.Context(), statement); err != nil {
						t.Error(err)
					}
				}
			}()
			for _, tc := range []struct {
				name string
				ids  []string
				want []string
			}{
				{"multiple bindings stay distinct and sort numerically", []string{"9010", "902", "902"}, []string{"902", "902", "9010"}},
				{"single account stays scoped", []string{"9010"}, []string{"9010"}},
				{"missing account returns empty", []string{"missing-contract"}, []string{}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rows, err := store.BoundAccountsForMaintenance(t.Context(), tc.ids)
					if err != nil {
						t.Fatal(err)
					}
					ids := make([]string, 0, len(rows))
					for _, row := range rows {
						ids = append(ids, row.AccountID)
					}
					if !reflect.DeepEqual(ids, tc.want) {
						t.Fatalf("account IDs=%v, want %v", ids, tc.want)
					}
				})
			}
			t.Run("delete preview deduplicates accounts", func(t *testing.T) {
				preview, err := store.UpstreamDeletePreview(t.Context(), "maintenance.contract.test")
				if err != nil {
					t.Fatal(err)
				}
				if len(preview.Accounts) != 2 || preview.Accounts[0].ID != "902" || preview.Accounts[1].ID != "9010" {
					t.Fatalf("unexpected account order: %+v", preview.Accounts)
				}
			})
		})
	}
}
