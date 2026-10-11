package upstreamconfig_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestUpdateToKnownAliasSavesAddressWithoutMovingIdentityOrAliasCredentials(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		host       string
		stored     bool
		credential bool
	}{
		{name: "identity-only-alias"},
		{name: "alias-with-catalog", stored: true},
		{name: "alias-with-private-credentials", stored: true, credential: true},
		{name: "explicit-alias", host: addressAliasHost, stored: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newAddressFixture(t, "https://"+addressSourceHost)
			fixture.addAlias(t, scenario.stored)
			if scenario.credential {
				token, refresh := "alias-access-token", "alias-refresh-token"
				if err := fixture.private.SaveAuthRecord(t.Context(), configstore.AuthRecord{
					Host: addressAliasHost, BaseURL: addressAliasURL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
					AccessToken: &token, RefreshToken: &refresh,
				}, nil); err != nil {
					t.Fatal(err)
				}
			}
			aliasBefore, err := fixture.private.AuthRecord(t.Context(), addressAliasHost)
			if err != nil {
				t.Fatal(err)
			}

			fixture.input.Host, fixture.input.BaseURL = scenario.host, addressAliasURL
			fixture.input.AccountBaseURL, fixture.input.RechargeRate = addressAliasURL+"/v1", "2"
			updated, err := fixture.service.Update(t.Context(), addressSourceHost, fixture.input, "test")
			if err != nil {
				t.Fatalf("saving an address belonging to the same stable upstream failed: %v", err)
			}
			if updated.Host != addressSourceHost || updated.UpstreamID != fixture.original.UpstreamID || updated.BaseURL != addressAliasURL || updated.AccountBaseURL != addressAliasURL+"/v1" || updated.RechargeRate != "2" {
				t.Fatalf("address update did not preserve the canonical identity and submitted settings: %#v", updated)
			}
			primary, err := fixture.private.AuthRecord(t.Context(), addressSourceHost)
			if err != nil || primary == nil || primary.BaseURL != addressAliasURL {
				t.Fatalf("canonical authentication address was not saved: %v", err)
			}
			aliasAfter, err := fixture.private.AuthRecord(t.Context(), addressAliasHost)
			if err != nil || !reflect.DeepEqual(aliasBefore, aliasAfter) {
				t.Fatalf("saving the canonical address changed alias credentials: %v", err)
			}
			summary, err := fixture.repository.Upstreams(t.Context())
			if err != nil || len(summary.Hosts) != 1 || summary.Hosts[0].Host != addressSourceHost || !slices.Contains(summary.Hosts[0].Hosts, addressAliasHost) {
				t.Fatalf("address update lost or split the existing alias: %#v err=%v", summary, err)
			}
		})
	}
}

func TestUpdateToKnownAliasRetainsAccountBindingsAndRecalculatesBothCatalogs(t *testing.T) {
	fixture := newAddressFixture(t, "https://"+addressSourceHost)
	fixture.addAlias(t, true)
	for _, item := range []struct{ host, account, group, key string }{
		{addressSourceHost, "41", "7", "key-primary"},
		{addressAliasHost, "42", "8", "key-alias"},
	} {
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,updated_at) VALUES(?,?,'标准组','active','2','now')`, []any{item.host, item.group}},
			{`INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,updated_at) VALUES(?,?,'绑定 Key',?,'active','now')`, []any{item.host, item.key, item.group}},
			{`INSERT INTO accounts(id,name,upstream_host,updated_at) VALUES(?,'绑定账号',?,'now')`, []any{item.account, item.host}},
			{`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at)
				VALUES(?,?,?,'绑定 Key',?,'标准组','now')`, []any{item.account, item.host, item.key, item.group}},
			{`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at)
				SELECT id,?,upstream_key_id,upstream_group_id,'now' FROM bindings WHERE local_account_id=?`, []any{fixture.original.UpstreamID, item.account}},
		} {
			if _, err := fixture.db.ExecContext(t.Context(), statement.query, statement.args...); err != nil {
				t.Fatal(err)
			}
		}
	}

	fixture.input.Host, fixture.input.BaseURL, fixture.input.RechargeRate = "", addressAliasURL, "2"
	if _, err := fixture.service.Update(t.Context(), addressSourceHost, fixture.input, "test"); err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct{ host, account, group, key string }{
		{addressSourceHost, "41", "7", "key-primary"},
		{addressAliasHost, "42", "8", "key-alias"},
	} {
		var bindingHost, accountHost, identity, key, group, cost string
		err := fixture.db.QueryRowContext(t.Context(), `SELECT b.upstream_host,a.upstream_host,bi.upstream_id,bi.upstream_key_id,bi.upstream_group_id,g.effective_rate
			FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id JOIN accounts a ON a.id=b.local_account_id
			JOIN upstream_groups g ON g.host=b.upstream_host AND g.group_id=b.upstream_group_id WHERE b.local_account_id=?`, item.account).Scan(
			&bindingHost, &accountHost, &identity, &key, &group, &cost,
		)
		if err != nil || bindingHost != item.host || accountHost != item.host || identity != fixture.original.UpstreamID || key != item.key || group != item.group || cost != "1" {
			t.Fatalf("account %s lost its stable binding or recalculated cost: host=%s accountHost=%s identity=%s key=%s group=%s cost=%s err=%v", item.account, bindingHost, accountHost, identity, key, group, cost, err)
		}
	}
}

func TestUpdateWithUnchangedAddressDoesNotRenameToAnotherConfiguredHost(t *testing.T) {
	fixture := newAddressFixture(t, addressAliasURL)
	other := fixture.input
	other.Host = addressAliasHost
	if _, err := fixture.service.Create(t.Context(), other, "test"); err != nil {
		t.Fatal(err)
	}
	otherBefore, err := fixture.private.AuthRecord(t.Context(), addressAliasHost)
	if err != nil {
		t.Fatal(err)
	}

	fixture.input.Host, fixture.input.RechargeRate = "", "2"
	updated, err := fixture.service.Update(t.Context(), addressSourceHost, fixture.input, "test")
	if err != nil || updated.Host != addressSourceHost || updated.UpstreamID != fixture.original.UpstreamID || updated.RechargeRate != "2" {
		t.Fatalf("saving existing connection settings incorrectly attempted a Host migration: %#v err=%v", updated, err)
	}
	otherAfter, err := fixture.private.AuthRecord(t.Context(), addressAliasHost)
	if err != nil || !reflect.DeepEqual(otherBefore, otherAfter) {
		t.Fatalf("saving unchanged settings modified another upstream's authentication: %v", err)
	}
}

func TestUpdateToKnownAliasWhenLoginFailsPreservesSavedConfiguration(t *testing.T) {
	fixture := newAddressFixture(t, "https://"+addressSourceHost)
	fixture.addAlias(t, true)
	publicBefore, err := fixture.repository.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	privateBefore, err := fixture.private.AuthRecord(t.Context(), addressSourceHost)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("isolated upstream login denied")
	fixture.verifier.failure = failure

	fixture.input.Host, fixture.input.BaseURL = "", addressAliasURL
	_, err = fixture.service.Update(t.Context(), addressSourceHost, fixture.input, "test")
	if !errors.Is(err, failure) {
		t.Fatalf("failed remote authentication was not returned: %v", err)
	}
	publicAfter, err := fixture.repository.Upstreams(t.Context())
	if err != nil || !reflect.DeepEqual(publicBefore, publicAfter) {
		t.Fatalf("failed login changed public configuration: %v", err)
	}
	privateAfter, err := fixture.private.AuthRecord(t.Context(), addressSourceHost)
	if err != nil || !reflect.DeepEqual(privateBefore, privateAfter) {
		t.Fatalf("failed login changed saved authentication: %v", err)
	}
}
