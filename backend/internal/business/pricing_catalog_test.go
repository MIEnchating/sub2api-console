package business

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPricingCatalogReadsOnlyConsoleAccountAndGroupProjection(t *testing.T) {
	store := openPricingCatalogStore(t)
	catalog, err := store.PricingCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Groups) != 2 || catalog.Groups[0].ID != "6" || catalog.Groups[0].Name != "标准" ||
		catalog.Groups[0].Platform != "openai" || catalog.Groups[0].RateMultiplier == nil || *catalog.Groups[0].RateMultiplier != "1" {
		t.Fatalf("local pricing groups=%#v", catalog.Groups)
	}
	if len(catalog.Accounts) != 2 {
		t.Fatalf("local pricing accounts=%#v", catalog.Accounts)
	}
	first := catalog.Accounts[0]
	if first.ID != "41" || first.Name != "local-account" || first.Platform != "openai" ||
		first.Multiplier == nil || *first.Multiplier != "0.4" || !first.GroupsValid || !first.ManualPriority || !reflect.DeepEqual(first.GroupIDs, []string{"6", "9"}) {
		t.Fatalf("first local pricing account=%#v", first)
	}
	if catalog.Accounts[1].GroupsValid {
		t.Fatalf("membership without a stable local group ID was accepted: %#v", catalog.Accounts[1])
	}
}

func TestPricingCatalogDerivesMissingAccountPlatformFromCurrentGroups(t *testing.T) {
	store := openPricingCatalogStore(t)
	if _, err := store.db.Exec(`INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at)
		VALUES('43','missing-platform','0.2','{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
		VALUES('43','标准','6','0.2'),('43','低价','7','0.2')`); err != nil {
		t.Fatal(err)
	}

	catalog, err := store.PricingCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range catalog.Accounts {
		if account.ID == "43" {
			if account.Platform != "openai" {
				t.Fatalf("derived platform=%q account=%#v", account.Platform, account)
			}
			return
		}
	}
	t.Fatal("derived-platform account was not returned")
}

func TestPricingCatalogDoesNotDerivePlatformFromMixedCurrentGroups(t *testing.T) {
	store := openPricingCatalogStore(t)
	if _, err := store.db.Exec(`INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at)
		VALUES('44','mixed-platform','0.2','{}','now');
		INSERT INTO local_groups(name,remote_id,platform,rate_multiplier,updated_at)
		VALUES('其他平台','8','anthropic','1','now');
		INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
		VALUES('44','标准','6','0.2'),('44','其他平台','8','0.2')`); err != nil {
		t.Fatal(err)
	}

	catalog, err := store.PricingCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range catalog.Accounts {
		if account.ID == "44" {
			if account.Platform != "" {
				t.Fatalf("mixed groups derived unsafe platform=%q account=%#v", account.Platform, account)
			}
			return
		}
	}
	t.Fatal("mixed-platform account was not returned")
}

func TestSyncPricingAccountGroupsUpdatesOnlyConfirmedLocalMemberships(t *testing.T) {
	store := openPricingCatalogStore(t)
	ctx := context.Background()
	result, err := store.SyncPricingAccountGroups(ctx, map[string][]string{"41": {"7", "6", "7"}}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.Accounts != 1 || result.GroupLinks != 2 || result.EventID >= 0 {
		t.Fatalf("local pricing sync result=%#v", result)
	}
	rows, err := store.db.QueryContext(ctx, `SELECT group_name,group_id,group_rate FROM account_groups WHERE account_id='41' ORDER BY group_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type membership struct{ name, id, rate string }
	memberships := []membership{}
	for rows.Next() {
		var item membership
		if err := rows.Scan(&item.name, &item.id, &item.rate); err != nil {
			t.Fatal(err)
		}
		memberships = append(memberships, item)
	}
	want := []membership{{name: "标准", id: "6", rate: "0.4"}, {name: "低价", id: "7", rate: "0.4"}}
	if !reflect.DeepEqual(memberships, want) {
		t.Fatalf("confirmed local memberships=%#v want=%#v", memberships, want)
	}
	var eventType string
	if err := store.db.QueryRowContext(ctx, `SELECT event_type FROM runtime_events WHERE source_id=?`, result.EventID).Scan(&eventType); err != nil || eventType != "pricing.groups.synced" {
		t.Fatalf("pricing sync event=%q err=%v", eventType, err)
	}
	records, err := store.PricingChangeRecords(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != result.EventID || records[0].Actor != "operator" || len(records[0].Changes) != 1 {
		t.Fatalf("pricing change records=%#v", records)
	}
	change := records[0].Changes[0]
	if change.AccountID != "41" || change.AccountName != "local-account" ||
		!reflect.DeepEqual(change.Before, []PricingChangeGroup{{ID: "6", Name: "标准"}, {ID: "9", Name: "外部分组"}}) ||
		!reflect.DeepEqual(change.After, []PricingChangeGroup{{ID: "6", Name: "标准"}, {ID: "7", Name: "低价"}}) {
		t.Fatalf("pricing account change=%#v", change)
	}
}

func TestPricingChangeRecordsKeepLegacyAggregateEvents(t *testing.T) {
	store := openPricingCatalogStore(t)
	if _, err := store.db.Exec(`INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json)
		VALUES(-20,'pricing.groups.synced','2026-09-03T08:00:00Z','succeeded','旧价格同步','{"actor":"scheduler","accounts":2,"group_links":3}')`); err != nil {
		t.Fatal(err)
	}

	records, err := store.PricingChangeRecords(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Actor != "scheduler" || records[0].AccountCount != 2 || records[0].GroupLinkCount != 3 || records[0].Changes == nil || len(records[0].Changes) != 0 {
		t.Fatalf("legacy aggregate event was not preserved: %#v", records)
	}
}

func TestPricingBackupCapturesStableAccountGroupMemberships(t *testing.T) {
	store := openPricingCatalogStore(t)
	backup, err := store.CreatePricingBackup(context.Background(), "调价前", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if backup.Name != "调价前" || backup.AccountCount != 2 || backup.Actor != "operator" {
		t.Fatalf("backup=%#v", backup)
	}
	loaded, err := store.PricingBackup(context.Background(), backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Accounts) != 2 || !reflect.DeepEqual(loaded.Accounts[0].GroupIDs, []string{"6", "9"}) ||
		!reflect.DeepEqual(loaded.Accounts[0].GroupNames, []string{"标准", "外部分组"}) {
		t.Fatalf("loaded=%#v", loaded)
	}
	backups, err := store.PricingBackups(context.Background())
	if err != nil || len(backups) != 1 || backups[0].ID != backup.ID {
		t.Fatalf("backups=%#v err=%v", backups, err)
	}
}

func TestPricingBackupRejectsBlankOrDuplicateNames(t *testing.T) {
	store := openPricingCatalogStore(t)
	if _, err := store.CreatePricingBackup(context.Background(), " ", "operator"); err == nil {
		t.Fatal("blank backup name was accepted")
	}
	if _, err := store.CreatePricingBackup(context.Background(), "基线", "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePricingBackup(context.Background(), "基线", "operator"); err == nil {
		t.Fatal("duplicate backup name was accepted")
	}
}

func TestDeletePricingBackupRemovesBackupAndAccountSnapshots(t *testing.T) {
	store := openPricingCatalogStore(t)
	backup, err := store.CreatePricingBackup(context.Background(), "待删除", "operator")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeletePricingBackup(context.Background(), backup.ID); err != nil {
		t.Fatal(err)
	}
	backups, err := store.PricingBackups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("deleted backup remains: %#v", backups)
	}
	var accountSnapshots int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM pricing_backup_accounts WHERE backup_id=?`, backup.ID).Scan(&accountSnapshots); err != nil {
		t.Fatal(err)
	}
	if accountSnapshots != 0 {
		t.Fatalf("deleted backup account snapshots=%d", accountSnapshots)
	}
	if err := store.DeletePricingBackup(context.Background(), backup.ID); err == nil {
		t.Fatal("missing backup deletion succeeded")
	}
}

func openPricingCatalogStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	statements := []string{
		`INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at) VALUES
			('41','local-account','0.4','{"platform":"openai"}','now'),
			('42','invalid-membership','0.2','{"platform":"openai"}','now')`,
		`INSERT INTO local_groups(name,remote_id,platform,rate_multiplier,updated_at) VALUES
			('标准','6','openai','1','now'),('低价','7','openai','0.5','now')`,
		`INSERT INTO account_groups(account_id,group_name,group_id,group_rate) VALUES
			('41','标准',NULL,'0.35'),('41','外部分组','9','0.4'),('42','缺少稳定 ID',NULL,'0.2')`,
		`INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('41',3,'now','now')`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatalf("pricing fixture failed: %v\n%s", err, statement)
		}
	}
	return store
}
