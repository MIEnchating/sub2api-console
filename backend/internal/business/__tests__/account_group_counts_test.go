package business_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestCommitAccountGroupsCountsStableMembershipAndLegacyNamesSeparately(t *testing.T) {
	store, db := concurrencyStore(t)
	_, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES
		('41','moving','0.1','before'),('42','peer','0.2','before'),('43','stable-peer','0.1','before');
		INSERT INTO local_groups(name,remote_id,account_count,updated_at) VALUES
		('old','7',99,'before'),('new','8',99,'before'),('legacy',NULL,99,'before'),('empty','9',99,'before');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES
		('41','old','7'),('42','old',NULL),('43','old','7'),
		('42','new','8'),('42','legacy',NULL),('42','empty','888')`)
	if err != nil {
		t.Fatal(err)
	}
	operation := business.AccountOperation{OperationID: "move-counts", OperationType: "account.groups", ObjectID: "41", State: "succeeded", Phase: "readback", Actor: "test"}
	if err := store.CommitAccountGroups(t.Context(), "41", []string{"7"}, []string{"8"}, operation); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"old": 2, "new": 2, "legacy": 1, "empty": 0}
	for name, count := range want {
		var actual int
		if err := db.QueryRow(`SELECT account_count FROM local_groups WHERE name=?`, name).Scan(&actual); err != nil || actual != count {
			t.Fatalf("group %s count=%d want=%d error=%v", name, actual, count, err)
		}
	}
}

func TestCommitAccountGroupsAuditFailureRollsBackMembershipAndCounts(t *testing.T) {
	store, db := concurrencyStore(t)
	seedGroupLock(t, store)
	if err := store.CommitAccountGroups(t.Context(), "41", []string{"7", "8"}, []string{"8"}, business.AccountOperation{}); err == nil {
		t.Fatal("accepted incomplete audit")
	}
	var links, firstCount, secondCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM account_groups WHERE account_id='41'`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT account_count FROM local_groups WHERE remote_id='7'`).Scan(&firstCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT account_count FROM local_groups WHERE remote_id='8'`).Scan(&secondCount); err != nil {
		t.Fatal(err)
	}
	if links != 2 || firstCount != 2 || secondCount != 1 {
		t.Fatalf("partial commit: links=%d first=%d second=%d", links, firstCount, secondCount)
	}
}
