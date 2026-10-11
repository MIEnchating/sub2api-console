package business_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func accountOwnershipFixture(t testing.TB, otherAccounts int) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ownership.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, "_pragma=foreign_keys%28ON%29"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
 INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('upstream-a','now','now'),('upstream-b','now','now');
 INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('a.example','upstream-a',1,'now'),('b.example','upstream-b',1,'now');
 INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at) VALUES('a.example','https://a.example','sub2api','ready','now'),('b.example','https://b.example','sub2api','ready','now');
 INSERT INTO accounts(id,name,upstream_host,metadata_json,updated_at) VALUES('41','selected','old.example','{}','now');
 INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) VALUES('41','alias-a.example','key-41','selected key','codex','now');
 INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at) SELECT id,'upstream-a',upstream_key_id,'now' FROM bindings WHERE local_account_id='41';`)
	if err != nil {
		t.Fatal(err)
	}
	if otherAccounts > 0 {
		_, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?)
 INSERT INTO accounts(id,name,upstream_host,metadata_json,updated_at) SELECT CAST(x+10000 AS TEXT),'other','b.example','{}','now' FROM n;
 INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) SELECT id,'b.example',id,'other key','codex','now' FROM accounts WHERE id<>'41';
 INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at) SELECT id,'upstream-b',upstream_key_id,'now' FROM bindings WHERE local_account_id<>'41'`, otherAccounts)
		if err != nil {
			t.Fatal(err)
		}
	}
	return store, db
}

func TestAccountDetailResolvesOnlyItsStableBindingOwnership(t *testing.T) {
	store, _ := accountOwnershipFixture(t, 2)
	detail, err := store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.UpstreamID == nil || *detail.UpstreamID != "upstream-a" || detail.UpstreamHost == nil || *detail.UpstreamHost != "a.example" || !detail.UpstreamHostRepairable {
		t.Fatalf("selected account lost stable ownership: %+v", detail.AccountStatus)
	}
	if len(detail.Bindings) != 1 || detail.Bindings[0].LocalAccountID != "41" {
		t.Fatalf("unrelated bindings leaked into detail: %+v", detail.Bindings)
	}
}

func TestAccountDetailWithMultipleIdentitiesDoesNotOfferHostRepair(t *testing.T) {
	store, db := accountOwnershipFixture(t, 1)
	_, err := db.Exec(`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) VALUES('41','b.example','second-key','second','codex','now');
 INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at) SELECT id,'upstream-b',upstream_key_id,'now' FROM bindings WHERE local_account_id='41' AND upstream_key_id='second-key'`)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.UpstreamHostRepairable || len(detail.Bindings) != 2 {
		t.Fatalf("ambiguous ownership marked repairable: %+v", detail)
	}
}

func TestMissingAccountDetailDoesNotResolveAnotherAccountsBindings(t *testing.T) {
	store, _ := accountOwnershipFixture(t, 1)
	if _, err := store.Account(t.Context(), "99"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing account error=%v", err)
	}
}

func BenchmarkSingleAccountWithLargeBindingDirectory(b *testing.B) {
	store, _ := accountOwnershipFixture(b, 5000)
	b.ReportAllocs()
	for b.Loop() {
		detail, err := store.Account(b.Context(), "41")
		if err != nil || detail == nil || detail.ID != "41" {
			b.Fatalf("account detail=%+v error=%v", detail, err)
		}
	}
}
