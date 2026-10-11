package business_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMaintenanceDiscoveryPreservesNumericOrderAndDeduplicatesBindings(t *testing.T) {
	store, _ := maintenanceScopeFixture(t)
	ids, err := store.BoundAccountIDsForMaintenance(t.Context())
	if err != nil || !reflect.DeepEqual(ids, []string{"2", "10"}) {
		t.Fatalf("IDs=%v err=%v", ids, err)
	}
}

func TestMaintenanceDiscoveryMatchesNormalizedPrimaryAndSourceButNotOtherAliases(t *testing.T) {
	store, db := maintenanceScopeFixture(t)
	for _, statement := range []string{
		`UPDATE bindings SET source_auth_host=' HTTPS://SOURCE.EXAMPLE/v1/ ' WHERE local_account_id='10'`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('alias.example','fixture-upstream',0,'now')`,
		`UPDATE accounts SET metadata_json='invalid'`,
		`UPDATE upstreams SET metadata_json='null'`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, fixture := range []struct {
		host string
		want []string
	}{
		{"https://MAINTENANCE.EXAMPLE/", []string{"2", "10"}},
		{"source.example", []string{"10"}},
		{"alias.example", []string{}},
		{"missing.example", []string{}},
		{" ", []string{}},
		{"///", []string{}},
	} {
		t.Run(fixture.host, func(t *testing.T) {
			ids, err := store.BoundAccountIDsForMaintenanceHost(t.Context(), fixture.host)
			if err != nil || !reflect.DeepEqual(ids, fixture.want) {
				t.Fatalf("IDs=%v err=%v want=%v", ids, err, fixture.want)
			}
		})
	}
}

func TestMaintenanceDiscoveryExcludesBindingsWithoutCompleteStableIdentity(t *testing.T) {
	store, db := maintenanceScopeFixture(t)
	for _, statement := range []string{
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('no-primary','now','now'),('no-upstream','now','now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('missing.example','no-upstream',1,'now')`,
		`INSERT INTO accounts(id,name,updated_at) VALUES('40','no-identity','now'),('50','no-primary','now'),('60','no-upstream','now')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) VALUES
		('40','maintenance.example','40','fixture','default','now'),('50','maintenance.example','50','fixture','default','now'),
		('60','maintenance.example','60','fixture','default','now'),('70','maintenance.example','70','fixture','default','now')`,
		`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
		SELECT id,CASE local_account_id WHEN '50' THEN 'no-primary' WHEN '60' THEN 'no-upstream' ELSE 'fixture-upstream' END,upstream_key_id,'now'
		FROM bindings WHERE local_account_id IN ('50','60','70')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := store.BoundAccountIDsForMaintenance(t.Context())
	if err != nil || !reflect.DeepEqual(ids, []string{"2", "10"}) {
		t.Fatalf("IDs=%v err=%v", ids, err)
	}
	ids, err = store.BoundAccountIDsForMaintenanceHost(t.Context(), "maintenance.example")
	if err != nil || !reflect.DeepEqual(ids, []string{"2", "10"}) {
		t.Fatalf("Host IDs=%v err=%v", ids, err)
	}
}

func TestMaintenanceDiscoveryCancelledQueryReturnsCancellation(t *testing.T) {
	store, _ := maintenanceScopeFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.BoundAccountIDsForMaintenance(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("all discovery error=%v", err)
	}
	if _, err := store.BoundAccountIDsForMaintenanceHost(ctx, "maintenance.example"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Host discovery error=%v", err)
	}
}
