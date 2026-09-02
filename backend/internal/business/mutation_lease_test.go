package business

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

func TestMutationLeaseAcquiresAllResourcesAtomicallyAndFencesOwners(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-lease.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Now().UTC()

	acquired, err := store.AcquireMutationLease(ctx, "first", []string{"account/41", "upstream/api.example"}, now, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("first acquire = %t, %v", acquired, err)
	}
	acquired, err = store.AcquireMutationLease(ctx, "second", []string{"account/42", "account/41"}, now, time.Minute)
	if err != nil || acquired {
		t.Fatalf("overlapping acquire = %t, %v", acquired, err)
	}
	acquired, err = store.AcquireMutationLease(ctx, "third", []string{"account/42"}, now, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("unrelated resource was partially reserved: acquired=%t err=%v", acquired, err)
	}
	if err := store.ReleaseMutationLease(ctx, "first", []string{"account/41", "upstream/api.example"}); err != nil {
		t.Fatal(err)
	}
	acquired, err = store.AcquireMutationLease(ctx, "second", []string{"account/41"}, now, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("released resource was not reusable: acquired=%t err=%v", acquired, err)
	}
}

func TestMutationLeaseExpirationAndOwnerCheckedRenewal(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-expiry.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Now().UTC()
	resources := []string{"account/41"}
	if acquired, err := store.AcquireMutationLease(ctx, "expired", resources, now, time.Second); err != nil || !acquired {
		t.Fatalf("initial acquire = %t, %v", acquired, err)
	}
	if acquired, err := store.AcquireMutationLease(ctx, "next", resources, now.Add(2*time.Second), time.Minute); err != nil || !acquired {
		t.Fatalf("expired lease was not fenced and replaced: acquired=%t err=%v", acquired, err)
	}
	if renewed, err := store.RenewMutationLease(ctx, "expired", resources, now.Add(3*time.Second), time.Minute); err != nil || renewed {
		t.Fatalf("stale owner renewed replacement lease: renewed=%t err=%v", renewed, err)
	}
	if renewed, err := store.RenewMutationLease(ctx, "next", resources, now.Add(3*time.Second), time.Minute); err != nil || !renewed {
		t.Fatalf("current owner failed to renew: renewed=%t err=%v", renewed, err)
	}
}

func TestMutationLeaseCannotBeRenewedAfterItsExpiry(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-late-renewal.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Now().UTC()
	resources := []string{"account/41"}
	if acquired, err := store.AcquireMutationLease(ctx, "owner", resources, now, time.Second); err != nil || !acquired {
		t.Fatalf("initial acquire = %t, %v", acquired, err)
	}
	if renewed, err := store.RenewMutationLease(ctx, "owner", resources, now.Add(2*time.Second), time.Minute); err != nil || renewed {
		t.Fatalf("expired lease renewed: renewed=%t err=%v", renewed, err)
	}
}

func TestMutationLeaseSupportsResourceSetsBeyondSQLiteParameterLimit(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-large.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	resources := make([]string, 0, 1205)
	for index := 0; index < 1205; index++ {
		resources = append(resources, fmt.Sprintf("account/%d", index+1))
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if acquired, err := store.AcquireMutationLease(ctx, "large", resources, now, time.Minute); err != nil || !acquired {
		t.Fatalf("large acquire = %t, %v", acquired, err)
	}
	if renewed, err := store.RenewMutationLease(ctx, "large", resources, now.Add(time.Second), time.Minute); err != nil || !renewed {
		t.Fatalf("large renewal = %t, %v", renewed, err)
	}
	if err := store.ReleaseMutationLease(ctx, "large", resources); err != nil {
		t.Fatal(err)
	}
	if acquired, err := store.AcquireMutationLease(ctx, "replacement", resources, now.Add(2*time.Second), time.Minute); err != nil || !acquired {
		t.Fatalf("large replacement acquire = %t, %v", acquired, err)
	}
}

func TestUpstreamIdentityMutationAccountIDsSupportsLargeComponents(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-identity-large.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('stable-tail','now','now')`,
		`INSERT INTO accounts(id,name,upstream_host,paused,updated_at) VALUES
			('41','host account','host-19999.example',0,'now'),('42','identity account',NULL,0,'now')`,
		`INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
			VALUES(7,'42','host-0.example','key-1','key','group','now')`,
		`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
			VALUES(7,'stable-tail','key-1','now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	hosts := make([]string, 20_000)
	upstreamIDs := make([]string, 20_000)
	for index := range hosts {
		hosts[index] = fmt.Sprintf("host-%d.example", index)
		upstreamIDs[index] = fmt.Sprintf("stable-%d", index)
	}
	upstreamIDs[len(upstreamIDs)-1] = "stable-tail"

	accountIDs, err := store.upstreamIdentityMutationAccountIDs(ctx, hosts, upstreamIDs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(accountIDs, "\x00") != "41\x0042" {
		t.Fatalf("large component account IDs = %#v", accountIDs)
	}
}

func TestResolveMutationResourcesIndexesLargeMultiHostComponent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-resolver-large.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	const hostCount = 1_500
	hosts := make([]string, hostCount)
	aliases := make([]string, 0, hostCount-1)
	resources := make([]string, 0, hostCount)
	for index := range hosts {
		hosts[index] = fmt.Sprintf("host-%04d.example", index)
		resources = append(resources, mutationguard.Upstream(hosts[index]))
		if index > 0 {
			aliases = append(aliases, hosts[index])
		}
	}
	aliasJSON, err := json.Marshal(map[string]any{"alias_hosts": aliases})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for index, host := range hosts {
		metadata := "{}"
		if index == 0 {
			metadata = string(aliasJSON)
		}
		upstreamID := fmt.Sprintf("stable-%04d", index)
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstreams(
			host,base_url,upstream_type,auth_status,metadata_json,updated_at
		) VALUES(?,?,'sub2api','ready',?,'now')`, host, "https://"+host, metadata); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_identities(upstream_id,created_at,updated_at)
			VALUES(?,'now','now')`, upstreamID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
			VALUES(?,?,1,'now')`, host, upstreamID); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	resolved, err := store.ResolveMutationResources(ctx, resources)
	if err != nil {
		t.Fatal(err)
	}
	unique := make(map[string]struct{}, len(resolved))
	for _, resource := range resolved {
		unique[resource] = struct{}{}
	}
	if len(unique) != 1+2*hostCount {
		t.Fatalf("unique resolved resources=%d want=%d", len(unique), 1+2*hostCount)
	}
	if _, found := unique[mutationguard.UpstreamCatalog()]; !found {
		t.Fatal("large reconciliation component omitted the catalog resource")
	}
}

func TestMutationLeaseResolvesKnownUpstreamAliasesToStableIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-alias.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('stable-1','now','now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('api.example','stable-1',1,'now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('relay.example','stable-1',0,'now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	_, releasePrimary, err := mutationguard.Acquire(ctx, store, mutationguard.Upstream("api.example"))
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, _, err := mutationguard.Acquire(waitCtx, store, mutationguard.Upstream("HTTPS://RELAY.EXAMPLE/")); !errors.Is(err, context.DeadlineExceeded) {
		_ = releasePrimary()
		t.Fatalf("alias acquire did not wait on stable identity: %v", err)
	}
	if err := releasePrimary(); err != nil {
		t.Fatal(err)
	}
	_, releaseAlias, err := mutationguard.Acquire(ctx, store, mutationguard.Upstream("relay.example"))
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseAlias(); err != nil {
		t.Fatal(err)
	}
}

func TestMutationLeaseCatalogRequestLocksAccountsAcrossPersistedIdentityHosts(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-persisted-alias.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES
			('api.example','https://api.example','sub2api','ready','{}','now'),
			('relay.example','https://relay.example','sub2api','ready','{}','now')`,
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('stable-1','now','now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('api.example','stable-1',1,'now'),('relay.example','stable-1',0,'now')`,
		`INSERT INTO accounts(id,name,upstream_host,paused,updated_at)
			VALUES('41','persisted alias account','relay.example',0,'now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	_, releaseAccount, err := mutationguard.Acquire(ctx, store, mutationguard.Account("41"))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseAccount()
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, _, err := mutationguard.Acquire(
		waitCtx, store, mutationguard.UpstreamCatalog(), mutationguard.Upstream("api.example"),
	); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("catalog mutation bypassed account on persisted identity alias: %v", err)
	}
}

func TestMutationLeaseKeepsRawHostLockAcrossIdentityNamespaceTransition(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-transition.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	_, releaseUnknown, err := mutationguard.Acquire(ctx, store, mutationguard.Upstream("new.example"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('stable-new','now','now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('new.example','stable-new',1,'now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			_ = releaseUnknown()
			t.Fatal(err)
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, _, err := mutationguard.Acquire(waitCtx, store, mutationguard.Upstream("HTTPS://NEW.EXAMPLE/")); !errors.Is(err, context.DeadlineExceeded) {
		_ = releaseUnknown()
		t.Fatalf("known identity bypassed unresolved host lease: %v", err)
	}
	if err := releaseUnknown(); err != nil {
		t.Fatal(err)
	}
	_, releaseKnown, err := mutationguard.Acquire(ctx, store, mutationguard.Upstream("new.example"))
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseKnown(); err != nil {
		t.Fatal(err)
	}
}

func TestMutationLeaseSerializesProspectiveAliasesBeforeIdentityMerge(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-alias-merge.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES
			('a.example','https://a.example','sub2api','ready','{"alias_hosts":["b.example"]}','now'),
			('b.example','https://b.example','sub2api','ready','{}','now')`,
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES
			('stable-a','2026-01-01T00:00:00Z','now'),('stable-b','2026-02-01T00:00:00Z','now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('a.example','stable-a',1,'now'),('b.example','stable-b',1,'now')`,
		`INSERT INTO accounts(id,name,upstream_host,paused,updated_at)
			VALUES('41','alias account','b.example',0,'now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	_, releaseAccount, err := mutationguard.Acquire(ctx, store, mutationguard.Account("41"))
	if err != nil {
		t.Fatal(err)
	}
	waitAccountCtx, cancelAccount := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancelAccount()
	if _, _, err := mutationguard.Acquire(waitAccountCtx, store, mutationguard.Upstream("a.example")); !errors.Is(err, context.DeadlineExceeded) {
		_ = releaseAccount()
		t.Fatalf("identity merge bypassed associated account lease: %v", err)
	}
	if err := releaseAccount(); err != nil {
		t.Fatal(err)
	}

	guarded, releaseMerge, err := mutationguard.Acquire(ctx, store, mutationguard.Upstream("a.example"))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseMerge()
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, _, err := mutationguard.Acquire(waitCtx, store, mutationguard.Upstream("b.example")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("prospective alias bypassed merge lease: %v", err)
	}
	accountIDs, err := store.UpstreamMutationAccountIDs(guarded, "a.example")
	if err != nil {
		t.Fatalf("account discovery under resolved catalog lease failed: %v", err)
	}
	if strings.Join(accountIDs, "\x00") != "41" {
		t.Fatalf("merged alias accounts = %#v", accountIDs)
	}
	var identities int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT upstream_id) FROM upstream_identity_hosts
		WHERE host IN ('a.example','b.example')`).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if identities != 1 {
		t.Fatalf("merged identity count = %d", identities)
	}
}
