package business_test

import (
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

func TestDatabaseContractUpstreamConfigurationReadsBoundAccountExistence(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			privateConfig := config
			if config.Driver == dbconn.SQLite {
				privateConfig.Path = filepath.Join(t.TempDir(), "private.sqlite3")
			}
			private, err := configstore.OpenDatabase(t.Context(), privateConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer private.Close()
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			const host = "editor.contract.test"
			created, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
				Host: host, BaseURL: "https://" + host, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				for _, statement := range []string{
					`DELETE FROM bindings WHERE upstream_host='editor.contract.test'`,
					`DELETE FROM accounts WHERE id='editor-account'`,
					`DELETE FROM upstream_groups WHERE host='editor.contract.test'`,
					`DELETE FROM recharge_rates WHERE host='editor.contract.test'`,
					`DELETE FROM upstreams WHERE host='editor.contract.test'`,
				} {
					if _, err := db.ExecContext(t.Context(), statement); err != nil {
						t.Error(err)
					}
				}
				if _, err := db.ExecContext(t.Context(), `DELETE FROM upstream_identities WHERE upstream_id=?`, created.UpstreamID); err != nil {
					t.Error(err)
				}
			}()
			service := upstreamconfig.New(store, private, nil)
			t.Run("no groups returns an editable empty configuration", func(t *testing.T) {
				result, err := service.Get(t.Context(), host)
				if err != nil || result.UpstreamID != created.UpstreamID || len(result.Groups) != 0 {
					t.Fatalf("empty configuration: %+v, %v", result, err)
				}
			})
			for _, statement := range []string{
				`INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,effective_rate,updated_at) VALUES('editor.contract.test','7','测试分组','active','1','1','now')`,
				`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at) VALUES('editor-account','editor.contract.test','17','测试 Key','7','default','now')`,
			} {
				if _, err := db.ExecContext(t.Context(), statement); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(t.Context(), `INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at)
				SELECT id,?,upstream_key_id,upstream_group_id,'now' FROM bindings WHERE upstream_host=?`, created.UpstreamID, host); err != nil {
				t.Fatal(err)
			}
			for _, scenario := range []struct {
				name   string
				update string
				exists bool
			}{
				{"existing account with legacy null binding status remains visible", "", true},
				{"missing binding remains visible but account is unavailable", `UPDATE bindings SET status='missing' WHERE upstream_host='editor.contract.test'`, false},
				{"restored binding makes the account available", `UPDATE bindings SET status='active' WHERE upstream_host='editor.contract.test'`, true},
				{"deleted account keeps the historical binding unavailable", `DELETE FROM accounts WHERE id='editor-account'`, false},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					if _, err := db.ExecContext(t.Context(), `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('editor-account','关联账号','{}','now')`); err != nil {
						t.Fatal(err)
					}
					defer func() {
						if _, err := db.ExecContext(t.Context(), `DELETE FROM accounts WHERE id='editor-account'`); err != nil {
							t.Error(err)
						}
					}()
					if _, err := db.ExecContext(t.Context(), `UPDATE bindings SET status=NULL WHERE upstream_host='editor.contract.test'`); err != nil {
						t.Fatal(err)
					}
					if scenario.update != "" {
						if _, err := db.ExecContext(t.Context(), scenario.update); err != nil {
							t.Fatal(err)
						}
					}
					result, err := service.Get(t.Context(), host)
					if err != nil {
						t.Fatalf("editor configuration must remain readable: %v", err)
					}
					if result.UpstreamID != created.UpstreamID || len(result.Groups) != 1 || len(result.Groups[0].BoundAccounts) != 1 {
						t.Fatalf("configuration lost stable identity or bindings: %+v", result)
					}
					account := result.Groups[0].BoundAccounts[0]
					if account.AccountID != "editor-account" || account.AccountExists != scenario.exists {
						t.Fatalf("bound account: %+v, want exists=%v", account, scenario.exists)
					}
				})
			}
		})
	}
}
