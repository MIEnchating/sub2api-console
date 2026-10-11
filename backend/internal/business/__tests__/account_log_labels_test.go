package business_test

import (
	"context"
	"errors"
	"testing"
)

func TestAccountLogLabelsOnlyReadRequestedIdentitiesAndPreserveGroups(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','当前账号','{}','now'),('42','无分组账号','{}','now'),('43','不相关账号','{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES
		('41','分组一','7'),('41','分组二','8'),('41','旧分组',NULL);`); err != nil {
		t.Fatal(err)
	}
	labels, err := store.AccountLogLabels(context.Background(), []string{"41", " 42 ", "41", "", "99"})
	if err != nil || len(labels) != 2 {
		t.Fatalf("labels = %#v, %v; want only existing requested accounts", labels, err)
	}
	if labels["41"] == nil || labels["41"].Name != "当前账号" || len(labels["41"].GroupIDs) != 3 ||
		labels["42"] == nil || labels["42"].Name != "无分组账号" || len(labels["42"].GroupIDs) != 0 {
		t.Fatalf("labels lost names or memberships: %#v", labels)
	}
	for name, want := range map[string]string{"分组一": "7", "分组二": "8"} {
		if got := labels["41"].GroupIDs[name]; got == nil || *got != want {
			t.Fatalf("group %q = %v; want %s", name, got, want)
		}
	}
	if labels["41"].GroupIDs["旧分组"] != nil {
		t.Fatal("missing historical group ID must not be inferred")
	}
}

func TestAccountLogLabelsEmptyScopeDoesNotReadAllAccounts(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	if _, err := db.Exec(`DROP TABLE account_groups; DROP TABLE accounts`); err != nil {
		t.Fatal(err)
	}
	labels, err := store.AccountLogLabels(context.Background(), []string{"", " "})
	if err != nil || len(labels) != 0 {
		t.Fatalf("empty scope queried account directory: %#v, %v", labels, err)
	}
}

func TestAccountLogLabelsHonorsCallerReadSnapshot(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','读取前','{}','now')`); err != nil {
		t.Fatal(err)
	}
	err := store.WithReadSnapshot(context.Background(), func(ctx context.Context) error {
		if _, err := db.Exec(`UPDATE accounts SET name='读取后' WHERE id='41'`); err != nil {
			return err
		}
		labels, err := store.AccountLogLabels(ctx, []string{"41"})
		if err != nil || labels["41"] == nil || labels["41"].Name != "读取前" {
			t.Fatalf("labels escaped caller snapshot: %#v, %v", labels, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccountLogLabelsHonorsCancellation(t *testing.T) {
	store, _ := openTrafficRankingStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.AccountLogLabels(ctx, []string{"41"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled labels query = %v", err)
	}
}
