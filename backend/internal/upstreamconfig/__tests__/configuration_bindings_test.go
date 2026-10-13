package upstreamconfig_test

import "testing"

func TestConfigurationReadPreservesBoundAccountAvailability(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		accountExists bool
		bindingStatus *string
		available     bool
	}{
		{"existing account with legacy binding", true, nil, true},
		{"existing account with active binding", true, new("active"), true},
		{"existing account with missing binding", true, new("missing"), false},
		{"deleted account with retained binding", false, new("active"), false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newAddressFixture(t, "https://"+addressSourceHost)
			if scenario.accountExists {
				if _, err := f.db.ExecContext(t.Context(), `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('editor-account','关联账号','{}','now')`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.db.ExecContext(t.Context(), `INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,effective_rate,updated_at) VALUES(?,'7','测试分组','active','1','1','now')`, addressSourceHost); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.ExecContext(t.Context(), `INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,status,updated_at) VALUES('editor-account',?,'17','测试 Key','7','default',?,'now')`, addressSourceHost, scenario.bindingStatus); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.ExecContext(t.Context(), `INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at)
				SELECT id,?,upstream_key_id,upstream_group_id,'now' FROM bindings WHERE upstream_host=?`, f.original.UpstreamID, addressSourceHost); err != nil {
				t.Fatal(err)
			}

			result, err := f.service.Get(t.Context(), addressSourceHost)
			if err != nil {
				t.Fatalf("editor configuration must remain readable: %v", err)
			}
			if result.UpstreamID != f.original.UpstreamID || len(result.Groups) != 1 || len(result.Groups[0].BoundAccounts) != 1 {
				t.Fatalf("configuration lost stable identity or binding: %+v", result)
			}
			account := result.Groups[0].BoundAccounts[0]
			if account.AccountID != "editor-account" || account.AccountExists != scenario.available {
				t.Fatalf("bound account: %+v, want available=%v", account, scenario.available)
			}
		})
	}
}
