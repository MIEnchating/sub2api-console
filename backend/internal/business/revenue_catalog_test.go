package business

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRevenueCatalogUsesConsoleAccountsGroupsAndStableBindings(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "revenue.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	statements := []string{
		`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','alpha','{}','now'),('42','unbound','{}','now')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','6')`,
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES('edge.example','https://edge.example','sub2api','已鉴权','{}','now')`,
		`INSERT INTO recharge_rates(host,recharge_rate,updated_at) VALUES('auth.example','2','now')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,source_auth_host,metadata_json,updated_at)
		 VALUES('41','edge.example','91','alpha-key','codex','auth.example','{}','now')`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := store.RevenueCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Accounts) != 2 || len(catalog.Accounts[0].Bindings) != 1 || catalog.Accounts[0].Groups[0] != "codex" {
		t.Fatalf("catalog=%#v", catalog)
	}
	binding := catalog.Accounts[0].Bindings[0]
	if binding.AuthHost != "auth.example" || binding.UpstreamHost != "edge.example" || binding.UpstreamKeyID != "91" || binding.RechargeRate != "2" {
		t.Fatalf("binding=%#v", binding)
	}
	if len(catalog.Accounts[1].Bindings) != 0 {
		t.Fatalf("unbound account=%#v", catalog.Accounts[1])
	}
}
