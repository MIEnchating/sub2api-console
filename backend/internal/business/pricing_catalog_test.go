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
	want := []membership{{name: "标准", id: "6", rate: "0.35"}, {name: "低价", id: "7", rate: "0.4"}}
	if !reflect.DeepEqual(memberships, want) {
		t.Fatalf("confirmed local memberships=%#v want=%#v", memberships, want)
	}
	var eventType string
	if err := store.db.QueryRowContext(ctx, `SELECT event_type FROM runtime_events WHERE source_id=?`, result.EventID).Scan(&eventType); err != nil || eventType != "pricing.groups.synced" {
		t.Fatalf("pricing sync event=%q err=%v", eventType, err)
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
