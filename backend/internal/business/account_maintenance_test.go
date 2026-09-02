package business

import (
	"context"
	"database/sql"
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

func TestBoundAccountMaintenanceRejectsCorruptUpstreamMetadata(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE upstreams SET metadata_json='not-json' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BoundAccountsForMaintenance(context.Background(), []string{"41"}); err == nil {
		t.Fatal("corrupt upstream metadata must stop maintenance before it derives remote changes")
	}
}

func TestBoundAccountMaintenanceIncludesManualSyncPolicy(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, false, "operator"); err != nil {
		t.Fatal(err)
	}
	rows, err := store.BoundAccountsForMaintenance(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].ManualPriority || rows[0].SyncBalanceMultiplier {
		t.Fatalf("disabled manual sync policy not preserved: %#v", rows)
	}
	if _, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, true, "operator"); err != nil {
		t.Fatal(err)
	}
	rows, err = store.BoundAccountsForMaintenance(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].ManualPriority || !rows[0].SyncBalanceMultiplier {
		t.Fatalf("enabled manual sync policy not preserved: %#v", rows)
	}
}

func TestBoundAccountMaintenanceUsesSourceAuthHostRechargeRate(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`INSERT INTO recharge_rates(host,recharge_rate,updated_at) VALUES('auth.example','10','now');
		UPDATE bindings SET source_auth_host='auth.example' WHERE local_account_id='41'`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.BoundAccountsForMaintenance(context.Background(), []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RechargeRate != "10" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestBoundAccountMaintenanceKeepsLastSuccessfulRawRate(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE bindings SET upstream_rate='1.3' WHERE local_account_id='41'`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.BoundAccountsForMaintenance(context.Background(), []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].KnownRawRate != "1.3" || rows[0].KnownRawRateSource != "account_observation" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestBoundAccountMaintenanceFallsBackToFixedGroupCatalogRawRate(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE bindings SET upstream_rate=NULL WHERE local_account_id='41';
		UPDATE upstream_groups SET raw_rate='0.2' WHERE host='api.example'
			AND group_id=(SELECT upstream_group_id FROM bindings WHERE local_account_id='41')`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.BoundAccountsForMaintenance(context.Background(), []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].KnownRawRate != "0.2" || rows[0].KnownRawRateSource != "group_catalog" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestBoundAccountMaintenanceUsesSourceAuthGroupCatalogBeforePrimaryHost(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE bindings SET upstream_rate=NULL,source_auth_host='auth.example' WHERE local_account_id='41';
		INSERT INTO recharge_rates(host,recharge_rate,updated_at) VALUES('auth.example','10','now');
		INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,updated_at)
		SELECT 'auth.example',upstream_group_id,'auth group','active','1.5','now'
		FROM bindings WHERE local_account_id='41';
		UPDATE upstream_groups SET raw_rate='9.9' WHERE host='api.example'
			AND group_id=(SELECT upstream_group_id FROM bindings WHERE local_account_id='41')`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.BoundAccountsForMaintenance(context.Background(), []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RateSourceHost() != "auth.example" || rows[0].RechargeRate != "10" ||
		rows[0].KnownRawRate != "1.5" || rows[0].KnownRawRateSource != "group_catalog" {
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

func TestAccountDefaultsRepairTracksConsoleOnboardingAndClearsLoadFactor(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	rows, err := store.BoundAccountsForMaintenance(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ConsoleOnboarded {
		t.Fatalf("account must not be treated as console onboarding before its audit exists: %#v", rows)
	}
	name := "example-0.1"
	field := "created"
	if err := store.RecordAccountOperation(ctx, AccountOperation{
		OperationID: "onboarding-defaults-test", OperationType: "account.onboarding", State: "succeeded", Phase: "readback",
		Actor: "console", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41", ObjectName: &name, FieldName: &field,
		After: map[string]any{"concurrency": 0, "priority": 0}, Writeback: true,
	}); err != nil {
		t.Fatal(err)
	}
	rows, err = store.BoundAccountsForMaintenance(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].ConsoleOnboarded {
		t.Fatalf("rows=%#v", rows)
	}
	priority, concurrency := int64(1), int64(10)
	if err := store.CommitAccountDefaultsRepairs(ctx, []AccountDefaultsRepairCommit{{
		AccountID: "41", Priority: &priority, Concurrency: &concurrency, LoadFactorPresent: true, RemoteRepaired: true,
	}}, "operator"); err != nil {
		t.Fatal(err)
	}
	var storedPriority, storedConcurrency int64
	var loadFactor sql.NullString
	if err := store.db.QueryRow(`SELECT priority,concurrency,load_factor FROM accounts WHERE id='41'`).Scan(
		&storedPriority, &storedConcurrency, &loadFactor,
	); err != nil {
		t.Fatal(err)
	}
	if storedPriority != 1 || storedConcurrency != 10 || loadFactor.Valid {
		t.Fatalf("priority=%d concurrency=%d loadFactor=%#v", storedPriority, storedConcurrency, loadFactor)
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

func TestRepairAccountUpstreamHostsUsesUnambiguousBindingOwnership(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`UPDATE accounts SET upstream_host='relay.example' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	result, err := store.RepairAccountUpstreamHosts(ctx, []string{"41"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.Repaired != 1 || result.Skipped != 0 || result.EventID >= 0 || len(result.Items) != 1 || result.Items[0].Status != "已修复" {
		t.Fatalf("result=%#v", result)
	}
	var host string
	if err := store.db.QueryRow(`SELECT upstream_host FROM accounts WHERE id='41'`).Scan(&host); err != nil {
		t.Fatal(err)
	}
	if host != "api.example" {
		t.Fatalf("host=%q", host)
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
