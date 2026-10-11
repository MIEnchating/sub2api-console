package business_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestOnboardingCapacityIgnoresUnrelatedUpstreamMetadata(t *testing.T) {
	store, db := accountOwnershipFixture(t, 2)
	if _, err := db.Exec(`UPDATE upstreams SET metadata_json='{"concurrency_limit":12}' WHERE host='a.example';
		UPDATE upstreams SET metadata_json='invalid' WHERE host='b.example'`); err != nil {
		t.Fatal(err)
	}
	capacity, err := store.OnboardingCapacity(t.Context(), "upstream-a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.Limit == nil || *capacity.Limit != 12 || len(capacity.Accounts) != 1 || capacity.Accounts[0].ID != "41" {
		t.Fatalf("scoped capacity=%+v", capacity)
	}
	if _, err := store.RoutingCapacityAccounts(t.Context()); err == nil {
		t.Fatal("global capacity must still reject invalid upstream metadata")
	}
}

func TestOnboardingCapacityIncludesAliasAndUngroupedAccountsWithTheirConfirmedState(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	var id string
	if err := db.QueryRow(`SELECT upstream_id FROM upstream_identity_hosts WHERE host='fixture.example'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	want, err := store.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := store.OnboardingCapacity(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(capacity.Accounts, want) || len(capacity.Accounts) != 4 {
		t.Fatalf("scoped accounts differ from global inventory: %+v", capacity.Accounts)
	}
	if capacity.Limit == nil || *capacity.Limit != 12 || capacity.Status != "known" {
		t.Fatalf("capacity=%+v", capacity)
	}
}

func TestOnboardingCapacityRetainsGlobalConflictingIdentityGuard(t *testing.T) {
	store, db := accountOwnershipFixture(t, 1)
	if _, err := db.Exec(`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
		VALUES('10001','a.example','another-key','conflict','group','now');
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('upstream-c','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('c.example','upstream-c',1,'now');
		INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at) VALUES('c.example','https://c.example','sub2api','ready','now')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"upstream-a", "upstream-b", "upstream-c"} {
		if _, err := store.OnboardingCapacity(t.Context(), id); err == nil || !strings.Contains(err.Error(), "绑定了多个上游") {
			t.Fatalf("scope=%s accepted conflicting identity: %v", id, err)
		}
	}
}

func TestOnboardingCapacityRejectsMissingOrMalformedSelectedUpstream(t *testing.T) {
	store, db := accountOwnershipFixture(t, 0)
	if _, err := store.OnboardingCapacity(t.Context(), ""); err == nil {
		t.Fatal("accepted empty identity")
	}
	if _, err := store.OnboardingCapacity(t.Context(), "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing identity error=%v", err)
	}
	if _, err := db.Exec(`UPDATE upstreams SET metadata_json='invalid' WHERE host='a.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OnboardingCapacity(t.Context(), "upstream-a"); err == nil {
		t.Fatal("accepted invalid selected configuration")
	}
}

func TestOnboardingCapacityReusesCallerSnapshotAndNextReadSeesCommittedChanges(t *testing.T) {
	store, db := accountOwnershipFixture(t, 0)
	if _, err := db.Exec(`UPDATE upstreams SET metadata_json='{"concurrency_limit":12}';
		UPDATE accounts SET concurrency=2,schedulable=1 WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	err := store.WithReadSnapshot(t.Context(), func(ctx context.Context) error {
		if _, err := db.Exec(`UPDATE upstreams SET metadata_json='{"concurrency_limit":20}';
			UPDATE accounts SET concurrency=8 WHERE id='41'`); err != nil {
			return err
		}
		capacity, err := store.OnboardingCapacity(ctx, "upstream-a")
		if err != nil {
			return err
		}
		if capacity.Limit == nil || *capacity.Limit != 12 || len(capacity.Accounts) != 1 || capacity.Accounts[0].Concurrency == nil || *capacity.Accounts[0].Concurrency != 2 {
			t.Fatalf("mixed or replaced caller snapshot: %+v", capacity)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := store.OnboardingCapacity(t.Context(), "upstream-a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.Limit == nil || *capacity.Limit != 20 || len(capacity.Accounts) != 1 || capacity.Accounts[0].Concurrency == nil || *capacity.Accounts[0].Concurrency != 8 {
		t.Fatalf("fresh read reused old snapshot: %+v", capacity)
	}
}

func TestOnboardingCapacityReturnsNoAccountsForUnoccupiedUpstream(t *testing.T) {
	store, _ := accountOwnershipFixture(t, 0)
	capacity, err := store.OnboardingCapacity(t.Context(), "upstream-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(capacity.Accounts) != 0 {
		t.Fatalf("unexpected accounts: %+v", capacity.Accounts)
	}
}
