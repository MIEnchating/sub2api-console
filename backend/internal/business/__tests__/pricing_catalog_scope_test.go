package business_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestAccountPricingCatalogReturnsOnlySelectedAccountAndAllGroupOptions(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at) VALUES
		('41','目标','0.4','{"platform":" OpenAI "}','now'),('42','无关','0.2','{}','now');
		INSERT INTO local_groups(name,remote_id,platform,rate_multiplier,updated_at) VALUES
		('标准','6','openai','1','now'),('其他平台','9','anthropic','2','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES
		('41','标准',NULL),('41','别名','6'),('41','外部分组','12'),('42','无效',NULL);
		INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('41',3,'now','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "41", true, "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountIgnoreCostWall(t.Context(), "41", true, "test"); err != nil {
		t.Fatal(err)
	}
	catalog, err := store.PricingCatalogForAccount(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Accounts) != 1 || len(catalog.Groups) != 2 || catalog.Groups[0].ID != "6" || catalog.Groups[1].ID != "9" {
		t.Fatalf("scoped catalog=%+v", catalog)
	}
	account := catalog.Accounts[0]
	if account.ID != "41" || account.Name != "目标" || account.Platform != "openai" || account.Multiplier == nil || *account.Multiplier != "0.4" || !account.GroupsValid || !account.ManualPriority || !account.GroupsLocked || !account.IgnoreCostWall || !reflect.DeepEqual(account.GroupIDs, []string{"6", "12"}) {
		t.Fatalf("selected account lost pricing state: %+v", account)
	}
}

func TestAccountPricingCatalogPreservesInvalidAndMixedMemberships(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','缺失分组','{}','now'),('42','混合平台','{}','now'),('43','无分组','{"platform":"openai"}','now');
		INSERT INTO local_groups(name,remote_id,platform,updated_at) VALUES('标准','6','openai','now'),('其他','9','anthropic','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','缺失',NULL),('42','标准','6'),('42','其他','9')`); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		id, platform string
		valid        bool
		groups       []string
	}{
		{"41", "", false, []string{}},
		{"42", "", true, []string{"6", "9"}},
		{"43", "openai", true, []string{}},
	} {
		catalog, err := store.PricingCatalogForAccount(t.Context(), fixture.id)
		if err != nil || len(catalog.Accounts) != 1 {
			t.Fatalf("id=%s catalog=%+v err=%v", fixture.id, catalog, err)
		}
		account := catalog.Accounts[0]
		if account.Platform != fixture.platform || account.GroupsValid != fixture.valid || !reflect.DeepEqual(account.GroupIDs, fixture.groups) {
			t.Fatalf("id=%s account=%+v", fixture.id, account)
		}
	}
}

func TestAccountPricingCatalogRejectsInvalidScopeWithoutReadingAllAccounts(t *testing.T) {
	store, _ := pricingProjectionFixture(t)
	for _, id := range []string{"", "0", "041", "41 OR 1=1"} {
		if _, err := store.PricingCatalogForAccount(t.Context(), id); err == nil {
			t.Fatalf("invalid scope %q accepted", id)
		}
	}
	catalog, err := store.PricingCatalogForAccount(t.Context(), "99")
	if err != nil || catalog.Accounts == nil || len(catalog.Accounts) != 0 || catalog.Groups == nil {
		t.Fatalf("missing account catalog=%+v err=%v", catalog, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.PricingCatalogForAccount(ctx, "41"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled catalog read: %v", err)
	}
}

func TestAccountPricingCatalogKeepsCallerSnapshotAndRefreshesNextRead(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','目标','{}','now');
		INSERT INTO local_groups(name,remote_id,platform,updated_at) VALUES('标准','6','openai','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','标准')`); err != nil {
		t.Fatal(err)
	}
	err := store.WithReadSnapshot(t.Context(), func(ctx context.Context) error {
		if _, err := db.Exec(`UPDATE local_groups SET platform='anthropic'; UPDATE accounts SET name='新名称'`); err != nil {
			return err
		}
		catalog, err := store.PricingCatalogForAccount(ctx, "41")
		if err != nil {
			return err
		}
		if len(catalog.Accounts) != 1 || catalog.Accounts[0].Name != "目标" || catalog.Accounts[0].Platform != "openai" || catalog.Groups[0].Platform != "openai" {
			t.Fatalf("caller snapshot changed: %+v", catalog)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.PricingCatalogForAccount(t.Context(), "41")
	if err != nil || len(catalog.Accounts) != 1 || catalog.Accounts[0].Name != "新名称" || catalog.Accounts[0].Platform != "anthropic" {
		t.Fatalf("fresh snapshot=%+v err=%v", catalog, err)
	}
}

func BenchmarkAccountPricingCatalogWithUnrelatedAccounts(b *testing.B) {
	store, db := pricingProjectionFixture(b)
	metadata := `{"platform":"openai","notes":"` + strings.Repeat("fixture ", 1024) + `"}`
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
		INSERT INTO accounts(id,name,metadata_json,updated_at) SELECT CAST(x AS TEXT),'fixture',?,'now' FROM n`, metadata); err != nil {
		b.Fatal(err)
	}
	for _, scope := range []string{"global", "account"} {
		b.Run(scope, func(b *testing.B) {
			read := store.PricingCatalog
			want := 1000
			if scope == "account" {
				read = func(ctx context.Context) (business.PricingCatalog, error) {
					return store.PricingCatalogForAccount(ctx, "41")
				}
				want = 1
			}
			b.ReportAllocs()
			for b.Loop() {
				catalog, err := read(b.Context())
				if err != nil || len(catalog.Accounts) != want {
					b.Fatalf("accounts=%d err=%v", len(catalog.Accounts), err)
				}
			}
		})
	}
}
