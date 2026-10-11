package business_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestBatchPricingCatalogReturnsSelectedAccountsOnceInCatalogOrder(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','first','{"platform":"openai"}','now'),('42','unrelated','{}','now'),('43','last','{}','now');
		INSERT INTO local_groups(name,remote_id,platform,updated_at) VALUES('standard','6','openai','now'),('other','9','anthropic','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','standard','6'),('42','invalid',NULL),('43','other',NULL)`); err != nil {
		t.Fatal(err)
	}
	global, err := store.PricingCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := store.PricingCatalogForAccounts(t.Context(), []string{"43", "41", "43", "99"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected.Accounts, []business.PricingAccount{global.Accounts[0], global.Accounts[2]}) || !reflect.DeepEqual(selected.Groups, global.Groups) {
		t.Fatalf("scoped catalog lost ordering or selected state: %+v", selected)
	}
}

func TestBatchPricingCatalogEmptyScopeKeepsGroupsWithoutSelectingAccounts(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','target','{}','now');
		INSERT INTO local_groups(name,remote_id,platform,updated_at) VALUES('standard','6','openai','now')`); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{nil, {}} {
		catalog, err := store.PricingCatalogForAccounts(t.Context(), ids)
		if err != nil || catalog.Accounts == nil || len(catalog.Accounts) != 0 || len(catalog.Groups) != 1 {
			t.Fatalf("empty scope selected inventory: catalog=%+v err=%v", catalog, err)
		}
	}
}

func TestBatchPricingCatalogRejectsEntireScopeWhenOneIDIsInvalid(t *testing.T) {
	store, _ := pricingProjectionFixture(t)
	for _, invalid := range []string{"", "0", "041", "41 OR 1=1"} {
		if _, err := store.PricingCatalogForAccounts(t.Context(), []string{"41", invalid}); err == nil {
			t.Fatalf("invalid account scope %q accepted", invalid)
		}
	}
}

func TestBatchPricingCatalogCanceledReadReturnsContextError(t *testing.T) {
	store, _ := pricingProjectionFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.PricingCatalogForAccounts(ctx, []string{"41", "42"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scope read returned %v", err)
	}
}

func TestBatchPricingCatalogReadsLargeScopeWithoutRepeatingAccounts(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1100)
		INSERT INTO accounts(id,name,metadata_json,updated_at) SELECT CAST(x AS TEXT),'fixture','{"platform":"openai"}','now' FROM n`); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 1101)
	for id := 1100; id > 0; id-- {
		ids = append(ids, strconv.Itoa(id))
	}
	ids = append(ids, "41")
	catalog, err := store.PricingCatalogForAccounts(t.Context(), ids)
	if err != nil || len(catalog.Accounts) != 1100 {
		t.Fatalf("large scope accounts=%d err=%v", len(catalog.Accounts), err)
	}
	for index, account := range catalog.Accounts {
		if account.ID != strconv.Itoa(index+1) {
			t.Fatalf("large scope index=%d account ID=%s", index, account.ID)
		}
	}
}

func TestBatchPricingCatalogReusesCallerSnapshotForAllSelectedAccounts(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','before','{}','now'),('42','peer','{}','now');
		INSERT INTO local_groups(name,remote_id,platform,updated_at) VALUES('standard','6','openai','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','standard'),('42','standard')`); err != nil {
		t.Fatal(err)
	}
	err := store.WithReadSnapshot(t.Context(), func(ctx context.Context) error {
		if _, err := db.Exec(`UPDATE accounts SET name='after' WHERE id='41'; UPDATE local_groups SET platform='anthropic'`); err != nil {
			return err
		}
		catalog, err := store.PricingCatalogForAccounts(ctx, []string{"41", "42"})
		if err == nil && (len(catalog.Accounts) != 2 || catalog.Accounts[0].Name != "before" || catalog.Accounts[1].Platform != "openai" || catalog.Groups[0].Platform != "openai") {
			t.Fatalf("batch escaped caller snapshot: %+v", catalog)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.PricingCatalogForAccounts(t.Context(), []string{"41", "42"})
	if err != nil || len(catalog.Accounts) != 2 || catalog.Accounts[0].Name != "after" || catalog.Accounts[1].Platform != "anthropic" {
		t.Fatalf("fresh scope failed to observe commit: %+v err=%v", catalog, err)
	}
}
