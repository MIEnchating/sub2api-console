package business

import (
	"context"
	"path/filepath"
	"testing"
)

func TestUpstreamIdentityMergesExplicitAliasesAndSurvivesMutableAddressChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(
		host,base_url,upstream_type,auth_status,metadata_json,updated_at
	) VALUES
		('subclaude.example','https://subclaude.example','sub2api','已鉴权','{"alias_hosts":["192.0.2.44:8080"]}','now'),
		('192.0.2.44:8080','https://subclaude.example','sub2api','已鉴权','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}

	first, err := store.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Hosts) != 1 || first.Hosts[0].UpstreamID == "" || len(first.Hosts[0].Hosts) != 2 {
		t.Fatalf("explicit aliases must share one upstream ID: %#v", first.Hosts)
	}
	upstreamID := first.Hosts[0].UpstreamID
	aliasID, err := store.upstreamIdentityID(ctx, "192.0.2.44:8080")
	if err != nil || aliasID != upstreamID {
		t.Fatalf("alias identity=%q, want %q, err=%v", aliasID, upstreamID, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET base_url='https://changed.example'
		WHERE host='subclaude.example'`); err != nil {
		t.Fatal(err)
	}
	second, err := store.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Hosts) != 1 || second.Hosts[0].UpstreamID != upstreamID {
		t.Fatalf("mutable address change replaced upstream ID: before=%q after=%#v", upstreamID, second.Hosts)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	third, err := reopened.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Hosts) != 1 || third.Hosts[0].UpstreamID != upstreamID {
		t.Fatalf("reopening database replaced upstream ID: before=%q after=%#v", upstreamID, third.Hosts)
	}
}

func TestCreateUpstreamConfigurationAssignsStableUpstreamIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	name := "Example"
	created, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.UpstreamID == "" {
		t.Fatalf("created upstream has no stable identity: %#v", created)
	}
	summary, err := store.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Hosts) != 1 || summary.Hosts[0].UpstreamID != created.UpstreamID {
		t.Fatalf("created identity was not persisted: created=%#v summary=%#v", created, summary.Hosts)
	}
}

func TestUpstreamIdentityMergeMovesStableBindingsAndCatalogTombstones(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "merge.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('a.example','https://a.example','sub2api','已鉴权','{}','2026-08-30T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstream_groups(host,group_id,name,status,updated_at) VALUES('a.example','6','pro','active','2026-08-30T00:00:00Z');
		INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,updated_at) VALUES('a.example','91','pro-key','6','active','2026-08-30T00:00:00Z');
		INSERT INTO accounts(id,name,upstream_host,paused,updated_at) VALUES('41','account','a.example',0,'now');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at)
		VALUES('41','a.example','91','pro-key','6','codex','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('b.example','https://b.example','sub2api','已鉴权','{}','2026-08-30T01:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET metadata_json='{"alias_hosts":["b.example"]}' WHERE host='a.example'`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureUpstreamIdentities(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}
	summary, err := store.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Hosts) != 1 {
		t.Fatalf("identities were not merged: %#v", summary.Hosts)
	}
	var bindingID, catalogID string
	if err := store.db.QueryRow(`SELECT upstream_id FROM binding_identities LIMIT 1`).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT upstream_id FROM upstream_catalog_entities WHERE entity_kind='key' AND entity_id='91'`).Scan(&catalogID); err != nil {
		t.Fatal(err)
	}
	if bindingID != summary.Hosts[0].UpstreamID || catalogID != bindingID {
		t.Fatalf("stable relations were not merged: summary=%q binding=%q catalog=%q", summary.Hosts[0].UpstreamID, bindingID, catalogID)
	}
}
