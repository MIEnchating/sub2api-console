package business_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDetectionScopeReusesCallerSnapshotAndNextReadSeesChangedMembership(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','old','now'),('42','new','now');
		INSERT INTO local_groups(name,remote_id,updated_at) VALUES('original','7','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','original','7')`); err != nil {
		t.Fatal(err)
	}
	err := store.WithReadSnapshot(t.Context(), func(ctx context.Context) error {
		if _, err := db.Exec(`UPDATE local_groups SET name='renamed' WHERE remote_id='7'; DELETE FROM account_groups; INSERT INTO account_groups(account_id,group_name,group_id) VALUES('42','renamed','7')`); err != nil {
			return err
		}
		got, err := store.DetectionTaskScope(ctx, []string{"7"})
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(got.AccountIDs, []string{"41"}) || got.GroupNamesByID["7"] != "original" {
			t.Fatalf("escaped caller snapshot: %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.DetectionTaskScope(t.Context(), []string{"7"})
	if err != nil || !reflect.DeepEqual(got.AccountIDs, []string{"42"}) || got.GroupNamesByID["7"] != "renamed" {
		t.Fatalf("fresh scope=%+v err=%v", got, err)
	}
}

func TestDetectionScopeDeduplicatesGroupsAndMembershipsWithoutNameFallback(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','selected','now'),('42','same name only','now');
		INSERT INTO local_groups(name,remote_id,updated_at) VALUES('first','7','now'),('second','8','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','first','7'),('41','alias','7'),('41','second','8'),('42','first',NULL)`); err != nil {
		t.Fatal(err)
	}
	got, err := store.DetectionTaskScope(t.Context(), []string{"8", "7", "8"})
	if err != nil || !reflect.DeepEqual(got.AccountIDs, []string{"41"}) || !reflect.DeepEqual(got.GroupIDsByAccount["41"], []string{"7", "8"}) || len(got.GroupNamesByID) != 2 {
		t.Fatalf("scope=%+v err=%v", got, err)
	}
}

func TestDetectionScopeEmptyMissingAndCancelledRequestsNeverExpandToAllGroups(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES('first','7','now')`); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{nil, {}} {
		got, err := store.DetectionTaskScope(t.Context(), ids)
		if err != nil || got.AccountIDs == nil || len(got.AccountIDs) != 0 || len(got.GroupNamesByID) != 0 {
			t.Fatalf("empty scope=%+v err=%v", got, err)
		}
	}
	if _, err := store.DetectionTaskScope(t.Context(), []string{"7", "99", "98"}); err == nil || !strings.Contains(err.Error(), "分组 99 不存在") {
		t.Fatalf("missing group error=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.DetectionTaskScope(ctx, []string{"7"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scope=%v", err)
	}
}
