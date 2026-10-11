package business_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestSyncPricingGroupCountsSeparatesStableIDsFromLegacyNames(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES
		('41','moving','0.1','before'),('42','peer','0.2','before'),('43','stable-peer','0.1','before');
		INSERT INTO local_groups(name,remote_id,account_count,updated_at) VALUES
		('old','7',99,'before'),('new','8',99,'before'),('legacy',NULL,99,'before'),('empty','9',99,'before');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES
		('41','old','7'),('42','old',NULL),('43','old','7'),
		('42','new','8'),('42','legacy',NULL),('42','empty','888')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncPricingAccountGroups(t.Context(), map[string][]string{"41": {"8"}}, "test"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{"old": 2, "new": 2, "legacy": 1, "empty": 0} {
		var count int
		var updatedAt string
		if err := db.QueryRow(`SELECT account_count,updated_at FROM local_groups WHERE name=?`, name).Scan(&count, &updatedAt); err != nil {
			t.Fatal(err)
		}
		if count != want || updatedAt == "before" {
			t.Errorf("group %s count=%d want=%d updatedAt=%s", name, count, want, updatedAt)
		}
	}
}

func TestSyncPricingGroupCountsUsesAllConfirmedChangesBeforeCounting(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES
		('41','first','0.1','before'),('42','second','0.2','before');
		INSERT INTO local_groups(name,remote_id,account_count,updated_at) VALUES('old','7',2,'before'),('new','8',0,'before');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','old','7'),('42','old','7')`); err != nil {
		t.Fatal(err)
	}
	result, err := store.SyncPricingAccountGroups(t.Context(), map[string][]string{"42": {"8", "8"}, "41": {"8"}}, "test")
	if err != nil || result.Accounts != 2 || result.GroupLinks != 2 {
		t.Fatalf("confirmed batch result=%+v error=%v", result, err)
	}
	for name, want := range map[string]int{"old": 0, "new": 2} {
		var count int
		if err := db.QueryRow(`SELECT account_count FROM local_groups WHERE name=?`, name).Scan(&count); err != nil || count != want {
			t.Fatalf("group %s count=%d want=%d error=%v", name, count, want, err)
		}
	}
	records, err := store.PricingChangeRecords(t.Context(), 10)
	if err != nil || len(records) != 1 || len(records[0].Changes) != 2 || records[0].Changes[0].AccountID != "41" || records[0].Changes[1].AccountID != "42" {
		t.Fatalf("confirmed batch audit=%+v error=%v", records, err)
	}
}

func TestSyncPricingGroupCountsAuditFailureRollsBackMembershipAndCounts(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES('41','moving','0.1','before');
		INSERT INTO local_groups(name,remote_id,account_count,updated_at) VALUES('old','7',1,'before'),('new','8',0,'before');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','old','7');
		CREATE TRIGGER reject_pricing_audit BEFORE INSERT ON runtime_events
		WHEN NEW.event_type='pricing.groups.synced' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncPricingAccountGroups(t.Context(), map[string][]string{"41": {"8"}}, "test"); err == nil || !strings.Contains(err.Error(), "audit unavailable") {
		t.Fatalf("audit failure was not reported: %v", err)
	}
	var groupID string
	if err := db.QueryRow(`SELECT group_id FROM account_groups WHERE account_id='41'`).Scan(&groupID); err != nil || groupID != "7" {
		t.Fatalf("membership escaped rollback: group=%s error=%v", groupID, err)
	}
	for name, want := range map[string]int{"old": 1, "new": 0} {
		var count int
		var updatedAt string
		if err := db.QueryRow(`SELECT account_count,updated_at FROM local_groups WHERE name=?`, name).Scan(&count, &updatedAt); err != nil || count != want || updatedAt != "before" {
			t.Fatalf("group %s escaped rollback: count=%d updatedAt=%s error=%v", name, count, updatedAt, err)
		}
	}
	records, err := store.PricingChangeRecords(t.Context(), 10)
	if err != nil || len(records) != 0 {
		t.Fatalf("failed sync left an audit: %+v error=%v", records, err)
	}
}

func TestSyncPricingGroupCountsConflictingDirectoryNamesRefuseEntireWrite(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES('41','moving','0.1','before');
		INSERT INTO local_groups(name,remote_id,account_count,updated_at) VALUES('old','7',1,'before'),('new','8',0,'before');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','old','7'),('41','conflicting','8')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncPricingAccountGroups(t.Context(), map[string][]string{"41": {"8"}}, "test"); err == nil || !strings.Contains(err.Error(), "多个名称") {
		t.Fatalf("conflicting stable group identity accepted: %v", err)
	}
	catalog, err := store.PricingCatalogForAccount(t.Context(), "41")
	if err != nil || len(catalog.Accounts) != 1 || !reflect.DeepEqual(catalog.Accounts[0].GroupIDs, []string{"7", "8"}) {
		t.Fatalf("conflicting directory caused a partial membership write: %+v error=%v", catalog, err)
	}
}
