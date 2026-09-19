package business_test

import "testing"

func TestScopedRoutingReadDoesNotLoadUnrelatedUpstreamConfiguration(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,updated_at) VALUES
 ('41','selected',NULL,'now'),('42','unrelated','fixture.example','now');
 INSERT INTO account_groups(account_id,group_name) VALUES('41','selected'),('42','unrelated');
 UPDATE upstreams SET metadata_json='invalid' WHERE host='fixture.example'`); err != nil {
		t.Fatal(err)
	}
	id := "41"
	accounts, err := store.RoutingAccounts(t.Context(), &id, nil)
	if err != nil || len(accounts) != 1 || accounts[0].ID != id {
		t.Fatalf("scoped read loaded unrelated configuration: accounts=%d err=%v", len(accounts), err)
	}
	if _, err := store.RoutingCapacityAccounts(t.Context()); err == nil {
		t.Fatal("global capacity must still validate unrelated reservations")
	}
	if _, err := store.RoutingAccounts(t.Context(), nil, nil); err == nil {
		t.Fatal("full routing read must still validate all upstreams")
	}
}
