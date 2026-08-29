package business

import (
	"context"
	"testing"
)

func TestBoundAccountMaintenanceUsesNormalizedSiteName(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE upstreams SET base_url='https://api.aiyxgaw.com',metadata_json='{"site_name":"New API"}' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.BoundAccountsForMaintenance(context.Background(), []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ExpectedName != "aiyxgaw-0.1" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestAccountNamesForMaintenanceIncludesAccountsWithoutBindings(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE accounts SET name='Existing Account-0.15' WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	names, err := store.AccountNamesForMaintenance(context.Background(), []string{"42"})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names["42"] != "Existing Account-0.15" {
		t.Fatalf("names=%#v", names)
	}
}

func TestMissingBindingIsNotReportedAsExisting(t *testing.T) {
	store := openReadModelFixture(t)
	if err := store.CommitBindingVerification(context.Background(), []BindingVerification{{AccountID: "41", Exists: false}}); err != nil {
		t.Fatal(err)
	}
	groups, err := store.UpstreamGroups(context.Background(), "api.example", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) == 0 || len(groups[0].BoundAccounts) != 1 || groups[0].BoundAccounts[0].AccountExists {
		t.Fatalf("groups=%#v", groups)
	}
	if groups[0].BoundAccounts[0].BindingStatus == nil || *groups[0].BoundAccounts[0].BindingStatus != "missing" {
		t.Fatalf("binding status=%#v", groups[0].BoundAccounts[0].BindingStatus)
	}
}

func TestCleanupMissingBindingsRemovesStaleAccountAndReleasesGroup(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if err := store.CommitBindingVerification(ctx, []BindingVerification{{AccountID: "41", Exists: false}}); err != nil {
		t.Fatal(err)
	}
	result, err := store.CleanupMissingBindings(ctx, []string{"41"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.Cleaned != 1 {
		t.Fatalf("result=%#v", result)
	}
	for _, table := range []string{"accounts", "bindings", "account_groups"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE ` + map[string]string{
			"accounts": "id", "bindings": "local_account_id", "account_groups": "account_id",
		}[table] + `='41'`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("table=%s count=%d err=%v", table, count, err)
		}
	}
	groups, err := store.UpstreamGroups(ctx, "api.example", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) == 0 || groups[0].Bound {
		t.Fatalf("stale binding still blocks onboarding: %#v", groups)
	}
}
