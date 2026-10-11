package business_test

import (
	"context"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestPricingCatalogControlFlagsRemainIndependentForEachAccount(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES
		('41','locked','now'),('42','ignored','now'),('43','ordinary','now');
		INSERT INTO local_groups(name,remote_id,updated_at) VALUES('standard','6','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','standard','6')`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "41", true, "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountIgnoreCostWall(t.Context(), "42", true, "test"); err != nil {
		t.Fatal(err)
	}
	for _, read := range []func(context.Context) (business.PricingCatalog, error){
		store.PricingCatalog,
		func(ctx context.Context) (business.PricingCatalog, error) {
			return store.PricingCatalogForAccounts(ctx, []string{"41", "42", "43"})
		},
	} {
		catalog, err := read(t.Context())
		if err != nil || len(catalog.Accounts) != 3 {
			t.Fatalf("catalog=%+v err=%v", catalog, err)
		}
		for _, account := range catalog.Accounts {
			if account.GroupsLocked != (account.ID == "41") || account.IgnoreCostWall != (account.ID == "42") {
				t.Fatalf("account control flags mixed: %+v", account)
			}
		}
	}
}

func TestPricingCatalogUnreadableControlScopeRejectsRead(t *testing.T) {
	for _, nodeType := range []string{"null", "array", "string"} {
		t.Run(nodeType, func(t *testing.T) {
			store, db := pricingProjectionFixture(t)
			if _, err := db.Exec(`UPDATE policy_nodes SET node_type=?,scalar_value='invalid'
				WHERE policy_key='control-plane' AND key_name='scope'`, nodeType); err != nil {
				t.Fatal(err)
			}
			if _, err := store.PricingCatalog(t.Context()); err == nil {
				t.Fatal("unreadable control scope must not report accounts as unlocked")
			}
			if _, err := store.PricingCatalogForAccount(t.Context(), "41"); err == nil {
				t.Fatal("scoped catalog must reject unreadable control scope")
			}
		})
	}
}

func TestPricingCatalogMissingControlDocumentReturnsUnprotectedAccount(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`DELETE FROM policy_nodes WHERE policy_key='control-plane';
		INSERT INTO accounts(id,name,updated_at) VALUES('41','ordinary','now')`); err != nil {
		t.Fatal(err)
	}
	catalog, err := store.PricingCatalogForAccount(t.Context(), "41")
	if err != nil || len(catalog.Accounts) != 1 {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}
	if catalog.Accounts[0].GroupsLocked || catalog.Accounts[0].IgnoreCostWall {
		t.Fatalf("missing policy invented protection: %+v", catalog.Accounts[0])
	}
}

func BenchmarkPricingCatalogWithLargeControlScope(b *testing.B) {
	store, db := pricingProjectionFixture(b)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','target','now');
		INSERT INTO policy_nodes(policy_key,parent_id,key_name,node_type,updated_at)
		SELECT 'control-plane',p.id,n.key_name,'array','now' FROM policy_nodes p
		CROSS JOIN (SELECT 'group_locked_account_ids' AS key_name UNION ALL SELECT 'ignore_cost_wall_account_ids') n
		WHERE p.policy_key='control-plane' AND p.key_name='scope'
		AND NOT EXISTS(SELECT 1 FROM policy_nodes c WHERE c.parent_id=p.id AND c.key_name=n.key_name);
		WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
		INSERT INTO policy_nodes(policy_key,parent_id,list_index,node_type,scalar_value,updated_at)
		SELECT 'control-plane',p.id,n.x-1,'string',CAST(n.x AS TEXT),'now'
		FROM n CROSS JOIN policy_nodes p WHERE p.policy_key='control-plane'
		AND p.key_name IN ('group_locked_account_ids','ignore_cost_wall_account_ids')`); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		catalog, err := store.PricingCatalogForAccount(b.Context(), "41")
		if err != nil || len(catalog.Accounts) != 1 {
			b.Fatalf("catalog=%+v err=%v", catalog, err)
		}
		if !catalog.Accounts[0].GroupsLocked || !catalog.Accounts[0].IgnoreCostWall {
			b.Fatal("control scope lost target protection")
		}
	}
}
