package business

import (
	"context"
	"path/filepath"
	"testing"
)

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
	for table, expected := range map[string]int{"upstreams": 0, "accounts": 0, "bindings": 0, "upstream_groups": 0} {
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
	if preview.AccountCount != 1 || preview.GroupCount != 1 {
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
