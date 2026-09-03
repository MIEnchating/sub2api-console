package business

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

func TestDeleteUpstreamProjectionWaitsForIdentityCatalogLease(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "delete-catalog-lease.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	name := "Example"
	if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	_, release, err := mutationguard.Acquire(ctx, store, mutationguard.UpstreamCatalog())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	deleteCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err = store.DeleteUpstreamProjection(deleteCtx, "api.example", nil, UpstreamDeleteAudit{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("delete bypassed identity catalog lease: %v", err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstreams WHERE host='api.example'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("blocked delete left %d upstream rows", count)
	}
}

func TestUpstreamDeleteStorePreservesHostValidationErrors(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "delete-host-validation.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, testCase := range []struct {
		name, host, expected string
	}{
		{name: "empty", host: " / ", expected: "上游 Host 不能为空"},
		{name: "missing", host: "missing.example", expected: "上游 Host 不存在"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := store.UpstreamDeletePreview(ctx, testCase.host); err == nil || err.Error() != testCase.expected {
				t.Fatalf("preview error=%v want=%q", err, testCase.expected)
			}
			if _, err := store.DeleteUpstreamProjection(ctx, testCase.host, nil, UpstreamDeleteAudit{}); err == nil || err.Error() != testCase.expected {
				t.Fatalf("delete error=%v want=%q", err, testCase.expected)
			}
		})
	}
}

func TestDeleteUpstreamProjectionReconcilesUnrelatedIdentitiesWithoutCatalogSelfLock(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "delete-reconcile-self-lock.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	name := "Target"
	if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "target.example", Name: &name, BaseURL: "https://target.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES
			('a.example','https://a.example','sub2api','ready','{"alias_hosts":["b.example"]}','now'),
			('b.example','https://b.example','sub2api','ready','{}','now')`,
		`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES
			('stable-a','2026-01-01T00:00:00Z','now'),('stable-b','2026-02-01T00:00:00Z','now')`,
		`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('a.example','stable-a',1,'now'),('b.example','stable-b',1,'now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	deleteCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if _, err := store.DeleteUpstreamProjection(deleteCtx, "target.example", nil, UpstreamDeleteAudit{}); err != nil {
		t.Fatalf("delete self-locked while reconciling unrelated identities: %v", err)
	}
	var identities int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT upstream_id) FROM upstream_identity_hosts
		WHERE host IN ('a.example','b.example')`).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if identities != 1 {
		t.Fatalf("unrelated identities were not reconciled: %d", identities)
	}
}

func TestDeleteUpstreamProjectionDoesNotRepairUnrelatedBindingIdentities(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "delete-scoped-relations.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	identityIDs := map[string]string{}
	for _, host := range []string{"target.example", "other.example", "third.example"} {
		name := host
		created, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
			Host: host, Name: &name, BaseURL: "https://" + host, UpstreamType: "sub2api",
			AuthMode: "sub2api_user_token", RechargeRate: "1",
		})
		if err != nil {
			t.Fatal(err)
		}
		identityIDs[host] = created.UpstreamID
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,upstream_host,paused,updated_at)
			VALUES('42','unrelated account','other.example',0,'now')`,
		`INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
			VALUES(7,'42','other.example','key-1','key','group','now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO binding_identities(
		binding_id,upstream_id,upstream_key_id,updated_at
	) VALUES(7,?,'key-1','now')`, identityIDs["third.example"]); err != nil {
		t.Fatal(err)
	}
	_, releaseAccount, err := mutationguard.Acquire(ctx, store, mutationguard.Account("42"))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseAccount()
	if _, err := store.DeleteUpstreamProjection(ctx, "target.example", nil, UpstreamDeleteAudit{}); err != nil {
		t.Fatal(err)
	}
	var upstreamID string
	if err := store.db.QueryRowContext(ctx, `SELECT upstream_id FROM binding_identities WHERE binding_id=7`).Scan(&upstreamID); err != nil {
		t.Fatal(err)
	}
	if upstreamID != identityIDs["third.example"] {
		t.Fatalf("unrelated binding identity was repaired under the target lease: got %q want %q", upstreamID, identityIDs["third.example"])
	}
}

func TestDeleteUpstreamProjectionCollectsPreviewBeforeNestedQueries(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "delete.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at) VALUES('api.example','https://api.example','sub2api','已鉴权','now')`,
		`INSERT INTO recharge_rates(host,recharge_rate,updated_at) VALUES('api.example','1','now')`,
		`INSERT INTO upstream_groups(host,group_id,name,status,updated_at) VALUES('api.example','6','pro','active','now')`,
		`INSERT INTO accounts(id,name,upstream_host,paused,updated_at) VALUES('41','example-0.2','api.example',0,'now')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','3')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at) VALUES('41','api.example','91','pro-key','6','codex','now')`,
		`INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('41',3,'now','now')`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := store.UpstreamDeletePreview(ctx, "https://api.example/")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Accounts) != 1 || preview.Accounts[0].ID != "41" || len(preview.Accounts[0].Groups) != 1 || preview.Accounts[0].Groups[0] != "codex" {
		t.Fatalf("preview=%#v", preview)
	}
	if _, err := store.DeleteUpstreamProjection(ctx, "api.example", []string{"99"}, UpstreamDeleteAudit{}); err == nil {
		t.Fatal("changed stable-ID scope must stop deletion")
	}
	projection, err := store.DeleteUpstreamProjection(ctx, "api.example", []string{"41"}, UpstreamDeleteAudit{
		Actor: "admin", RemoteDeletedAccounts: 1, PrivateAuthDeleted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.DeletedAccounts != 1 || projection.DeletedGroups != 1 || projection.EventID >= 0 {
		t.Fatalf("projection=%#v", projection)
	}
	for table, expected := range map[string]int{"upstreams": 0, "accounts": 0, "bindings": 0, "upstream_groups": 0, "manual_priority_accounts": 0} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != expected {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var eventType, status string
	if err := store.db.QueryRowContext(ctx, `SELECT event_type,status FROM runtime_events WHERE source_id=?`, projection.EventID).Scan(&eventType, &status); err != nil {
		t.Fatal(err)
	}
	if eventType != "upstream.deleted" || status != "succeeded" {
		t.Fatalf("event=%q status=%q", eventType, status)
	}
}

func TestDeleteUpstreamProjectionRollsBackWhenEventCannotBeWritten(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "delete-rollback.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at) VALUES('api.example','https://api.example','sub2api','已鉴权','now')`,
		`INSERT INTO accounts(id,name,upstream_host,paused,updated_at) VALUES('41','example-0.2','api.example',0,'now')`,
		`CREATE TRIGGER reject_upstream_delete_event BEFORE INSERT ON runtime_events WHEN NEW.event_type='upstream.deleted' BEGIN SELECT RAISE(ABORT, 'event rejected'); END`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.DeleteUpstreamProjection(ctx, "api.example", []string{"41"}, UpstreamDeleteAudit{RemoteDeletedAccounts: 1}); err == nil {
		t.Fatal("event failure must fail the atomic local deletion")
	}
	for _, table := range []string{"upstreams", "accounts"} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func TestDeleteUpstreamProjectionUsesStableIdentityAcrossHostAliases(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "delete-alias.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		 VALUES('api.example','https://api.example','sub2api','已鉴权','{"alias_hosts":["relay.example"]}','now')`,
		`INSERT INTO upstream_groups(host,group_id,name,status,updated_at) VALUES('relay.example','6','pro','active','now')`,
		`INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,updated_at) VALUES('relay.example','91','pro-key','6','active','now')`,
		`INSERT INTO accounts(id,name,upstream_host,paused,updated_at) VALUES('41','example-0.2','relay.example',0,'now')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at)
		 VALUES('41','relay.example','91','pro-key','6','codex','now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := store.UpstreamDeletePreview(ctx, "api.example")
	if err != nil {
		t.Fatal(err)
	}
	if preview.AccountCount != 1 || preview.GroupCount != 1 || len(preview.IdentityHosts) != 2 ||
		preview.IdentityHosts[0] != "api.example" || preview.IdentityHosts[1] != "relay.example" {
		t.Fatalf("preview=%#v", preview)
	}
	if _, err := store.DeleteUpstreamProjection(ctx, "api.example", []string{"41"}, UpstreamDeleteAudit{}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"upstreams", "upstream_groups", "upstream_keys", "upstream_identities", "upstream_identity_hosts", "upstream_catalog_entities"} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}
