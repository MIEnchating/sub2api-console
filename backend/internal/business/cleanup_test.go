package business

import (
	"context"
	"strings"
	"testing"
)

func TestDeleteAccountProjectionRemovesBindingsRuntimeDataAndPolicyReferences(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('37','delete-me','{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('37','special');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,metadata_json,updated_at)
		VALUES('37','https://upstream.example.com','key-8','delete-me','special','{}','now');
		INSERT INTO paused_accounts(account_id,reason,enabled,updated_at) VALUES('37','test',1,'now');
	`); err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	document["account_test_models"] = map[string]any{"37": "gpt-test", "38": "keep"}
	document["scope"] = map[string]any{
		"paused_account_ids": []any{"37", "38"}, "excluded_account_ids": []any{"37"}, "manual_fused_account_ids": []any{},
	}
	if err := store.writePolicyDocument(ctx, tx, "control-plane", document, "now"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	name, field := "delete-me", "deleted"
	err = store.DeleteAccountProjection(ctx, "37", AccountOperation{
		OperationID: "account-delete-test", OperationType: "account.delete", State: "succeeded", Phase: "readback",
		Actor: "tester", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "37", ObjectName: &name,
		GroupNames: []string{"special"}, FieldName: &field, Before: map[string]any{"id": "37"}, After: map[string]any{"deleted": true}, Writeback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM accounts WHERE id='37'`,
		`SELECT COUNT(*) FROM bindings WHERE local_account_id='37'`,
		`SELECT COUNT(*) FROM account_groups WHERE account_id='37'`,
		`SELECT COUNT(*) FROM paused_accounts WHERE account_id='37'`,
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("projection row remained: count=%d err=%v query=%s", count, err, query)
		}
	}
	document, err = store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	models := document["account_test_models"].(map[string]any)
	if _, present := models["37"]; present || models["38"] != "keep" {
		t.Fatalf("test model references not cleaned selectively: %#v", models)
	}
	scope := document["scope"].(map[string]any)
	paused := scope["paused_account_ids"].([]any)
	if len(paused) != 1 || paused[0] != "38" {
		t.Fatalf("scope references not cleaned selectively: %#v", scope)
	}
}

func TestDeleteAccountProjectionWithScopeRemovesOnlyStableIdentityKeyProjection(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES
			('upstream-1','now','now'),('upstream-2','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('api.example','upstream-1',1,'now'),('alias.example','upstream-1',0,'now'),
			('other.example','upstream-2',1,'now');
		INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,metadata_json,updated_at) VALUES
			('api.example','key-8','delete-me','group-1','active','{}','now'),
			('alias.example','key-8','delete-me','group-1','active','{}','now'),
			('other.example','key-8','keep-me','group-2','active','{}','now');
		INSERT INTO upstream_catalog_entities(
			upstream_id,entity_kind,entity_id,name,lifecycle_state,missing_observations,updated_at
		) VALUES
			('upstream-1','key','key-8','delete-me','active',0,'now'),
			('upstream-2','key','key-8','keep-me','active',0,'now');
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('37','delete-me','{}','now');
		INSERT INTO bindings(
			id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,metadata_json,updated_at
		) VALUES(91,'37','api.example','key-8','delete-me','special','{}','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
			VALUES(91,'upstream-1','key-8','now');
	`); err != nil {
		t.Fatal(err)
	}
	scope := AccountDeleteScope{BindingID: 91, UpstreamID: "upstream-1", UpstreamKeyID: "key-8"}
	if err := store.ConfirmAccountDeleteScope(ctx, "37", scope); err != nil {
		t.Fatal(err)
	}
	name, field := "delete-me", "deleted"
	if err := store.DeleteAccountProjectionWithScope(ctx, "37", scope, AccountOperation{
		OperationID: "scoped-account-delete", OperationType: "account.delete", State: "succeeded", Phase: "readback",
		Actor: "tester", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "37", ObjectName: &name,
		FieldName: &field, Before: map[string]any{"upstream_id": "upstream-1", "key_id": "key-8"},
		After: map[string]any{"deleted": true}, Writeback: true,
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM accounts WHERE id='37'`,
		`SELECT COUNT(*) FROM upstream_keys WHERE host IN ('api.example','alias.example') AND key_id='key-8'`,
		`SELECT COUNT(*) FROM upstream_catalog_entities WHERE upstream_id='upstream-1' AND entity_kind='key' AND entity_id='key-8'`,
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("scoped projection row remained: count=%d err=%v query=%s", count, err, query)
		}
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM upstream_keys WHERE host='other.example' AND key_id='key-8'`,
		`SELECT COUNT(*) FROM upstream_catalog_entities WHERE upstream_id='upstream-2' AND entity_kind='key' AND entity_id='key-8'`,
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 1 {
			t.Fatalf("same key ID on another stable identity was deleted: count=%d err=%v query=%s", count, err, query)
		}
	}
	keys, err := store.onboardingKeys(ctx, "api.example")
	if err != nil || len(keys) != 0 {
		t.Fatalf("deleted key remained an active onboarding candidate: keys=%#v err=%v", keys, err)
	}
}

func TestReconcileDeletedUpstreamKeyProjectionIsScopedAndIdempotent(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES
			('upstream-1','now','now'),('upstream-2','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('api.example','upstream-1',1,'now'),('alias.example','upstream-1',0,'now'),
			('other.example','upstream-2',1,'now');
		INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,metadata_json,updated_at) VALUES
			('api.example','key-8','delete-me','group-1','active','{}','now'),
			('alias.example','key-8','delete-me','group-1','active','{}','now'),
			('other.example','key-8','keep-me','group-2','active','{}','now');
		INSERT INTO upstream_catalog_entities(
			upstream_id,entity_kind,entity_id,name,lifecycle_state,missing_observations,updated_at
		) VALUES
			('upstream-1','key','key-8','delete-me','active',0,'now'),
			('upstream-2','key','key-8','keep-me','active',0,'now');
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('37','delete-me','{}','now');
		INSERT INTO bindings(
			id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,metadata_json,updated_at
		) VALUES(91,'37','api.example','key-8','delete-me','special','{}','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
			VALUES(91,'upstream-1','key-8','now');
	`); err != nil {
		t.Fatal(err)
	}
	scope := AccountDeleteScope{BindingID: 91, UpstreamID: "upstream-1", UpstreamKeyID: "key-8"}
	for attempt := 0; attempt < 2; attempt++ {
		if err := store.ReconcileDeletedUpstreamKeyProjection(ctx, "37", scope); err != nil {
			t.Fatalf("reconcile attempt %d failed: %v", attempt+1, err)
		}
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM upstream_keys WHERE host IN ('api.example','alias.example') AND key_id='key-8'`,
		`SELECT COUNT(*) FROM upstream_catalog_entities WHERE upstream_id='upstream-1' AND entity_kind='key' AND entity_id='key-8'`,
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("reconciled key projection remained: count=%d err=%v query=%s", count, err, query)
		}
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM accounts WHERE id='37'`,
		`SELECT COUNT(*) FROM bindings WHERE id=91 AND local_account_id='37'`,
		`SELECT COUNT(*) FROM upstream_keys WHERE host='other.example' AND key_id='key-8'`,
		`SELECT COUNT(*) FROM upstream_catalog_entities WHERE upstream_id='upstream-2' AND entity_kind='key' AND entity_id='key-8'`,
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 1 {
			t.Fatalf("unrelated projection changed: count=%d err=%v query=%s", count, err, query)
		}
	}
}

func TestReconcileDeletedUpstreamKeyProjectionRechecksExclusiveScope(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('upstream-1','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('api.example','upstream-1',1,'now'),('alias.example','upstream-1',0,'now');
		INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,metadata_json,updated_at) VALUES
			('api.example','key-8','delete-me','group-1','active','{}','now');
		INSERT INTO upstream_catalog_entities(
			upstream_id,entity_kind,entity_id,name,lifecycle_state,missing_observations,updated_at
		) VALUES('upstream-1','key','key-8','delete-me','active',0,'now');
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
			('37','delete-me','{}','now'),('38','new-owner','{}','now');
		INSERT INTO bindings(
			id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,metadata_json,updated_at
		) VALUES(91,'37','api.example','key-8','delete-me','special','{}','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
			VALUES(91,'upstream-1','key-8','now');
	`); err != nil {
		t.Fatal(err)
	}
	scope := AccountDeleteScope{BindingID: 91, UpstreamID: "upstream-1", UpstreamKeyID: "key-8"}
	if err := store.ConfirmAccountDeleteScope(ctx, "37", scope); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO bindings(
			id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,metadata_json,updated_at
		) VALUES(92,'38','alias.example','key-8','new-owner','special','{}','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
			VALUES(92,'upstream-1','key-8','now');
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileDeletedUpstreamKeyProjection(ctx, "37", scope); err == nil || !strings.Contains(err.Error(), "共享 Key") {
		t.Fatalf("changed exclusive scope was accepted: %v", err)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM upstream_keys WHERE host='api.example' AND key_id='key-8'`,
		`SELECT COUNT(*) FROM upstream_catalog_entities WHERE upstream_id='upstream-1' AND entity_kind='key' AND entity_id='key-8'`,
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 1 {
			t.Fatalf("projection was cleaned after exclusive scope changed: count=%d err=%v query=%s", count, err, query)
		}
	}
}

func TestReconcileDeletedUnboundUpstreamKeyProjectionIsScopedAndRechecksReferences(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES
			('upstream-1','now','now'),('upstream-2','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('api.example','upstream-1',1,'now'),('alias.example','upstream-1',0,'now'),
			('other.example','upstream-2',1,'now');
		INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,metadata_json,updated_at) VALUES
			('api.example','key-8','delete-me','group-1','active','{}','now'),
			('alias.example','key-8','delete-me','group-1','active','{}','now'),
			('other.example','key-8','keep-me','group-2','active','{}','now');
		INSERT INTO upstream_catalog_entities(
			upstream_id,entity_kind,entity_id,name,lifecycle_state,missing_observations,updated_at
		) VALUES
			('upstream-1','key','key-8','delete-me','active',0,'now'),
			('upstream-2','key','key-8','keep-me','active',0,'now');
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileDeletedUnboundUpstreamKeyProjection(ctx, "alias.example", "key-8"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileDeletedUnboundUpstreamKeyProjection(ctx, "api.example", "key-8"); err != nil {
		t.Fatalf("reconcile must be idempotent: %v", err)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM upstream_keys WHERE host IN ('api.example','alias.example') AND key_id='key-8'`,
		`SELECT COUNT(*) FROM upstream_catalog_entities WHERE upstream_id='upstream-1' AND entity_kind='key' AND entity_id='key-8'`,
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("deleted identity projection remained: count=%d err=%v", count, err)
		}
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM upstream_keys WHERE host='other.example' AND key_id='key-8'`,
		`SELECT COUNT(*) FROM upstream_catalog_entities WHERE upstream_id='upstream-2' AND entity_kind='key' AND entity_id='key-8'`,
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 1 {
			t.Fatalf("same key ID on another identity changed: count=%d err=%v", count, err)
		}
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO upstream_keys(host,key_id,name,status,metadata_json,updated_at)
			VALUES('api.example','protected-key','protected','active','{}','now');
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('37','owner','{}','now');
		INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,metadata_json,updated_at)
			VALUES(91,'37','alias.example','protected-key','protected','group','{}','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
			VALUES(91,'upstream-1','protected-key','now');
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileDeletedUnboundUpstreamKeyProjection(ctx, "api.example", "protected-key"); err == nil || !strings.Contains(err.Error(), "仍被") {
		t.Fatalf("protected Key projection was accepted: %v", err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_keys WHERE host='api.example' AND key_id='protected-key'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("protected Key projection changed: count=%d err=%v", count, err)
	}
}

func TestConfirmAccountDeleteScopeRejectsSharedBindingAndPendingOnboarding(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES
			('upstream-1','now','now'),('upstream-2','now','now');
		INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES
			('api.example','upstream-1',1,'now'),('alias.example','upstream-1',0,'now'),
			('other.example','upstream-2',1,'now');
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
			('37','delete-me','{}','now'),('38','shared','{}','now'),('39','other-identity','{}','now');
		INSERT INTO bindings(
			id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,metadata_json,updated_at
		) VALUES
			(91,'37','api.example','key-8','delete-me','special','{}','now'),
			(92,'38','alias.example','key-8','shared','special','{}','now'),
			(93,'39','other.example','key-8','other-identity','special','{}','now');
		INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at) VALUES
			(91,'upstream-1','key-8','now'),(92,'upstream-1','key-8','now'),(93,'upstream-2','key-8','now');
	`); err != nil {
		t.Fatal(err)
	}
	scope := AccountDeleteScope{BindingID: 91, UpstreamID: "upstream-1", UpstreamKeyID: "key-8"}
	if err := store.ConfirmAccountDeleteScope(ctx, "37", scope); err == nil || !strings.Contains(err.Error(), "共享 Key") {
		t.Fatalf("shared binding was accepted: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM bindings WHERE id=92`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO onboarding_pending(
		operation_id,upstream_id,upstream_host,upstream_type,upstream_key_id,upstream_account_id,
		upstream_group_id,upstream_group_name,local_group_id,local_group_name,multiplier,reason,created_at,updated_at
	) VALUES('pending-key','','alias.example','sub2api','key-8','','group-1','group','1','local','1','pending','now','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmAccountDeleteScope(ctx, "37", scope); err == nil || !strings.Contains(err.Error(), "开户待续") {
		t.Fatalf("pending onboarding reference was accepted: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM onboarding_pending WHERE operation_id='pending-key'`); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmAccountDeleteScope(ctx, "37", scope); err != nil {
		t.Fatalf("same key ID on another stable identity blocked deletion: %v", err)
	}
	changed := scope
	changed.UpstreamID = "upstream-2"
	if err := store.ConfirmAccountDeleteScope(ctx, "37", changed); err == nil || !strings.Contains(err.Error(), "身份或 Key ID 已变化") {
		t.Fatalf("changed stable identity was accepted: %v", err)
	}
}
