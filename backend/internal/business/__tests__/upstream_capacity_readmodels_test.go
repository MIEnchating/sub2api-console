package business_test

import (
	"database/sql"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func seedCapacityAccounts(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		UPDATE upstreams SET metadata_json='{"concurrency_limit":12,"concurrency_status":"known","concurrency_checked_at":"2026-09-15T00:00:00Z"}' WHERE host='fixture.example';
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
		SELECT 'alias.example',upstream_id,0,'now' FROM upstream_identity_hosts WHERE host='fixture.example';
		INSERT INTO accounts(id,name,upstream_host,concurrency,schedulable,target_concurrency,target_schedulable,updated_at) VALUES
		('41','primary','fixture.example',5,1,3,1,'now'),
		('42','alias','alias.example',4,1,6,1,'now'),
		('43','without-group','fixture.example',2,1,NULL,NULL,'now'),
		('44','stopped','fixture.example',100,0,NULL,NULL,'now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','a'),('41','b'),('42','a');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) VALUES
		('41','fixture.example','7','key-a','a','now'),('41','alias.example','7','key-a','b','now');`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpstreamSummaryCountsUniqueActiveAccountsAcrossAliasesAndGroups(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	summary, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Hosts) != 1 {
		t.Fatalf("aliases became independent budgets: %#v", summary.Hosts)
	}
	upstream := summary.Hosts[0]
	if upstream.ConcurrencyLimit == nil || *upstream.ConcurrencyLimit != 12 || upstream.ConcurrencyStatus != "known" {
		t.Fatalf("profile capacity not exposed: %#v", upstream)
	}
	if upstream.AllocatedConcurrency == nil || *upstream.AllocatedConcurrency != 11 {
		t.Fatalf("duplicate membership, missing group, or stopped account changed configured total: %#v", upstream)
	}
	if upstream.TargetConcurrency == nil || *upstream.TargetConcurrency != 11 {
		t.Fatalf("target total did not combine proposed and unchanged capacities: %#v", upstream)
	}
}

func TestRoutingCapacityIncludesUnboundGroupAccountsAndOneIdentityForAliases(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	accounts, err := store.RoutingAccounts(t.Context(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 3 {
		t.Fatalf("routing memberships changed: %#v", accounts)
	}
	identity := accounts[0].UpstreamID
	for _, account := range accounts {
		if account.UpstreamID == "" || account.UpstreamID != identity || account.UpstreamConcurrencyLimit == nil || *account.UpstreamConcurrencyLimit != 12 {
			t.Fatalf("membership did not receive shared stable budget: %#v", account)
		}
	}
	inventory, err := store.RoutingCapacityAccounts(t.Context())
	if err != nil || len(inventory) != 4 {
		t.Fatalf("capacity inventory omitted accounts without groups: %#v, %v", inventory, err)
	}
}

func TestUnknownOrUnlimitedActiveAccountCapacityIsNotReportedAsZero(t *testing.T) {
	for _, test := range []struct {
		name     string
		capacity any
	}{{"missing", nil}, {"unlimited", 0}} {
		t.Run(test.name, func(t *testing.T) {
			store, db := concurrencyStore(t)
			seedCapacityAccounts(t, db)
			if _, err := db.Exec(`UPDATE accounts SET concurrency=? WHERE id='43'`, test.capacity); err != nil {
				t.Fatal(err)
			}
			summary, err := store.Upstreams(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if summary.Hosts[0].AllocatedConcurrency != nil || summary.Hosts[0].TargetConcurrency != nil {
				t.Fatalf("unbounded active capacity became a finite sum: %#v", summary.Hosts[0])
			}
		})
	}
}

func TestStoppedAccountDoesNotCountUntilItsTargetReenablesScheduling(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if _, err := db.Exec(`UPDATE accounts SET target_concurrency=1,target_schedulable=1 WHERE id='44'`); err != nil {
		t.Fatal(err)
	}
	summary, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Hosts[0].AllocatedConcurrency == nil || *summary.Hosts[0].AllocatedConcurrency != 11 || summary.Hosts[0].TargetConcurrency == nil || *summary.Hosts[0].TargetConcurrency != 12 {
		t.Fatalf("pending enablement was mixed into confirmed configured capacity: %#v", summary.Hosts[0])
	}
}

func TestConflictingStableUpstreamBindingsRejectCapacityCalculation(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	_, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "second.example", BaseURL: "https://second.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
		VALUES('41','second.example','8','other-user-key','a','now')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RoutingCapacityAccounts(t.Context()); err == nil {
		t.Fatal("conflicting account ownership silently assigned shared capacity")
	}
}

func TestUpdatingUpstreamConfigurationRequiresFreshConcurrencyBeforeGrowth(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	_, err := store.UpdateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "fixture.example", BaseURL: "https://fixture.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Hosts[0].ConcurrencyStatus != business.UpstreamConcurrencyStale || summary.Hosts[0].ConcurrencyLimit == nil || *summary.Hosts[0].ConcurrencyLimit != 12 {
		t.Fatalf("updated configuration still advertises a fresh user capacity: %#v", summary.Hosts[0])
	}
}

func TestReplacingVerifiedAuthenticationRequiresFreshConcurrencyBeforeGrowth(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if err := store.UpdateUpstreamClassification(t.Context(), "fixture.example", "sub2api", "sub2api_user_token"); err != nil {
		t.Fatal(err)
	}
	summary, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Hosts[0].ConcurrencyStatus != business.UpstreamConcurrencyStale {
		t.Fatalf("newly verified credentials inherited a fresh limit before profile sync: %#v", summary.Hosts[0])
	}
}
