package business

import (
	"context"
	"testing"
)

func TestUpstreamGroupBindingAuditComparesBindingsWithStableCatalog(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	upstreamID, err := store.upstreamIdentityID(ctx, "api.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE upstream_catalog_entities
		SET lifecycle_state='missing',missing_observations=2,confirmed_missing_at='now'
		WHERE upstream_id=? AND entity_kind='group' AND entity_id='1';
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
			('43','missing-catalog-account','{}','now'),
			('44','no-group-account','{}','now');
		INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,metadata_json,updated_at) VALUES
			(901,'43','api.example','key-3','key-3','untracked','3','codex','{}','now'),
			(902,'44','api.example','key-4','key-4',NULL,NULL,'codex','{}','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at) VALUES
			(901,?,'key-3','3','now'),
			(902,?,'key-4',NULL,'now')`, upstreamID, upstreamID, upstreamID); err != nil {
		t.Fatal(err)
	}

	result, err := store.UpstreamGroupBindingAudit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalBindings != 3 || result.Present != 0 || result.Missing != 1 || result.Unknown != 2 {
		t.Fatalf("unexpected audit summary: %#v", result)
	}
	if len(result.Items) != 3 {
		t.Fatalf("unexpected audit items: %#v", result.Items)
	}
	if result.Items[0].Host != "api.example" || result.Items[0].Status != "unknown" || result.Items[0].Accounts[0].ID != "44" {
		t.Fatalf("binding without a group ID was not reported: %#v", result.Items[0])
	}
	if result.Items[1].GroupID == nil || *result.Items[1].GroupID != "1" || result.Items[1].Status != "missing" ||
		result.Items[1].Accounts[0].ID != "41" {
		t.Fatalf("confirmed missing group was not reported: %#v", result.Items[1])
	}
	if result.Items[2].GroupID == nil || *result.Items[2].GroupID != "3" || result.Items[2].Status != "unknown" ||
		result.Items[2].Accounts[0].Name == nil || *result.Items[2].Accounts[0].Name != "missing-catalog-account" {
		t.Fatalf("unobserved group was not reported: %#v", result.Items[2])
	}
}

func TestUpstreamGroupBindingAuditAggregatesAccountsByGroup(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	upstreamID, err := store.upstreamIdentityID(ctx, "api.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('43','second-account','{}','now');
		INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,metadata_json,updated_at)
		VALUES(901,'43','api.example','key-3','key-3','codex','1','codex','{}','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at)
		VALUES(901,?,'key-3','1','now')`, upstreamID); err != nil {
		t.Fatal(err)
	}

	result, err := store.UpstreamGroupBindingAudit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalBindings != 2 || result.Present != 1 || result.Missing != 0 || result.Unknown != 0 || len(result.Items) != 1 {
		t.Fatalf("unexpected grouped audit: %#v", result)
	}
	if result.Items[0].AccountCount != 2 || len(result.Items[0].Accounts) != 2 {
		t.Fatalf("group accounts were not aggregated: %#v", result.Items[0])
	}
}
