package business_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestModelCatalogBatchKeepsRequestedOrderAndIgnoresUnselectedInvalidMetadata(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','first','{"platform":" OpenAI ","known_models":["model-b","Model-A","model-a"],"enabled_models":["model-b"]}','now'),
		('42','second','{}','now'),('43','unselected','invalid','now')`); err != nil {
		t.Fatal(err)
	}
	got, err := store.AccountModelCatalogs(t.Context(), []string{" 42 ", "41"})
	if err != nil {
		t.Fatal(err)
	}
	want := []business.AccountModelCatalog{
		{AccountID: "42", AccountName: "second", Models: []string{}, EnabledModels: []string{}},
		{AccountID: "41", AccountName: "first", Platform: "openai", Models: []string{"Model-A", "model-b"}, EnabledModels: []string{"model-b"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalogs=%+v want=%+v", got, want)
	}
}

func TestModelCatalogBatchReportsFirstUnavailableAccountInRequestedOrder(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','broken','invalid','now')`); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		ids  []string
		want string
	}{
		{[]string{"99", "41"}, "账号 99 不存在"},
		{[]string{"41", "99"}, "账号 41 元数据损坏"},
	} {
		got, err := store.AccountModelCatalogs(t.Context(), fixture.ids)
		if err == nil || !strings.Contains(err.Error(), fixture.want) || got != nil {
			t.Fatalf("ids=%v catalogs=%v err=%v", fixture.ids, got, err)
		}
	}
}

func TestModelCatalogBatchRejectsEmptyDuplicateInvalidAndCancelledScope(t *testing.T) {
	store, _ := pricingProjectionFixture(t)
	for _, ids := range [][]string{nil, {}, {"41", " 41 "}, {"not-an-id"}, make([]string, 1001)} {
		if _, err := store.AccountModelCatalogs(t.Context(), ids); err == nil {
			t.Fatalf("invalid scope accepted: %v", ids)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.AccountModelCatalogs(ctx, []string{"41"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read=%v", err)
	}
}

func TestModelSyncPreviewKeepsCallerSnapshotForCatalogAndPolicyThenRefreshes(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','original','{"known_models":["old-model"]}','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAccountModelExclusionList(t.Context(), []string{"old-*"}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountTestModels(t.Context(), "41", []string{"old-probe"}, "test"); err != nil {
		t.Fatal(err)
	}
	err := store.WithReadSnapshot(t.Context(), func(ctx context.Context) error {
		if _, err := db.Exec(`UPDATE accounts SET name='new',metadata_json='{"known_models":["new-model"]}' WHERE id='41'`); err != nil {
			return err
		}
		if err := store.SaveAccountModelExclusionList(t.Context(), []string{"new-*"}, "test"); err != nil {
			return err
		}
		if err := store.SetAccountTestModels(t.Context(), "41", []string{"new-probe"}, "test"); err != nil {
			return err
		}
		got, err := store.AccountModelSyncPreview(ctx, []string{"41"})
		if err != nil {
			return err
		}
		if len(got.Accounts) != 1 || got.Accounts[0].AccountName != "original" || got.Accounts[0].ProbeModel != "old-probe" || !reflect.DeepEqual(got.BlockedModels, []string{"old-model"}) {
			t.Fatalf("mixed snapshot: %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.AccountModelSyncPreview(t.Context(), []string{"41"})
	if err != nil || len(got.Accounts) != 1 || got.Accounts[0].AccountName != "new" || got.Accounts[0].ProbeModel != "new-probe" || !reflect.DeepEqual(got.BlockedModels, []string{"new-model"}) {
		t.Fatalf("fresh snapshot: %+v err=%v", got, err)
	}
}
