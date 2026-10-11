package business_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestConfigurationReadMatchesCatalogBalancesAndSharedCapacity(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if _, err := db.Exec(`UPDATE upstreams SET raw_balance='12.3456789',mapped_balance='6.17283945' WHERE host='fixture.example'`); err != nil {
		t.Fatal(err)
	}
	summary, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := store.UpstreamConfigurationHost(t.Context(), "fixture.example")
	if err != nil || configuration == nil {
		t.Fatalf("configuration unavailable: %+v, %v", configuration, err)
	}
	expected := summary.Hosts[0]
	// These overview-only aggregates are deliberately absent from editor reads.
	expected.Hosts = []string{}
	expected.AccountCount, expected.GroupCount, expected.RequestCount = 0, 0, 0
	expected.AccountRequestUsage = []business.UpstreamAccountRequestUsage{}
	if !reflect.DeepEqual(expected, *configuration) {
		t.Fatalf("configuration projection changed catalog fields:\nexpected %+v\nactual %+v", expected, *configuration)
	}
}

func TestConfigurationReadPreservesUnknownAccountCapacity(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if _, err := db.Exec(`UPDATE accounts SET concurrency=NULL WHERE id='43'`); err != nil {
		t.Fatal(err)
	}
	configuration, err := store.UpstreamConfigurationHost(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	if configuration.AllocatedConcurrency != nil || configuration.TargetConcurrency != nil {
		t.Fatalf("unknown capacity became a finite value: %+v", configuration)
	}
}

func TestConfigurationReadRejectsConflictingStableBindings(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	_, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "second.example", BaseURL: "https://second.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
		VALUES('41','second.example','8','other-key','a','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpstreamConfigurationHost(t.Context(), "fixture.example"); err == nil || !strings.Contains(err.Error(), "绑定了多个上游") {
		t.Fatalf("ambiguous capacity was accepted: %v", err)
	}
}

func TestConfigurationReadRequiresPrimaryHostAndRespectsCancellation(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	for _, host := range []string{"alias.example", "missing.example", ""} {
		configuration, err := store.UpstreamConfigurationHost(t.Context(), host)
		if err != nil || configuration != nil {
			t.Fatalf("non-primary Host %q resolved: %+v, %v", host, configuration, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.UpstreamConfigurationHost(ctx, "fixture.example"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not returned: %v", err)
	}
}

func TestCatalogIdentityLookupResolvesAliasesAndIgnoresOrphanCatalog(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	primary, err := store.UpstreamCatalogIdentityID(t.Context(), "fixture.example")
	if err != nil || primary == "" {
		t.Fatalf("primary identity unavailable: %q, %v", primary, err)
	}
	alias, err := store.UpstreamCatalogIdentityID(t.Context(), "alias.example")
	if err != nil || alias != primary {
		t.Fatalf("alias lost identity: %q, %v", alias, err)
	}
	if _, err := db.Exec(`DELETE FROM upstreams WHERE host='fixture.example'`); err != nil {
		t.Fatal(err)
	}
	identity, err := store.UpstreamCatalogIdentityID(t.Context(), "alias.example")
	if err != nil || identity != "" {
		t.Fatalf("orphan identity became an active catalog entry: %q, %v", identity, err)
	}
}
