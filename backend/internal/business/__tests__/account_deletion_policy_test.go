package business_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestAccountProjectionDeletionRemovesOnlyDeletedAccountPolicyReferences(t *testing.T) {
	for _, cleanup := range []struct {
		name string
		run  func(context.Context, *business.Store) error
	}{
		{name: "missing bindings", run: func(ctx context.Context, store *business.Store) error {
			_, err := store.CleanupMissingBindings(ctx, []string{"41"}, "operator")
			return err
		}},
		{name: "upstream cascade", run: func(ctx context.Context, store *business.Store) error {
			_, err := store.DeleteUpstreamProjection(ctx, "removed.example", []string{"41"}, business.UpstreamDeleteAudit{})
			return err
		}},
	} {
		t.Run(cleanup.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "deletion-policy.db")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			ctx := context.Background()
			if err := store.Bootstrap(ctx); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at)
				VALUES('removed.example','https://removed.example','sub2api','已鉴权','original');
				INSERT INTO accounts(id,name,upstream_host,updated_at) VALUES('41','removed','removed.example','original'),('42','kept',NULL,'original');
				INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,status,updated_at)
				VALUES('41','removed.example','key-1','bound','fixture-group','missing','original')`); err != nil {
				t.Fatal(err)
			}
			if err := store.SetAccountTestModels(ctx, "41", []string{"removed-model"}, "operator"); err != nil {
				t.Fatal(err)
			}
			if err := store.SetAccountTestModels(ctx, "42", []string{"kept-model"}, "operator"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(ctx, map[string]any{"advanced_policy": map[string]any{
				"scope": map[string]any{
					"paused_account_ids":       []any{"41", "42"},
					"excluded_account_ids":     []any{"41", "42"},
					"manual_fused_account_ids": []any{"41", "42"},
				},
			}}, "operator"); err != nil {
				t.Fatal(err)
			}
			if err := cleanup.run(ctx, store); err != nil {
				t.Fatal(err)
			}
			policy, err := store.ControlPolicy(ctx)
			if err != nil {
				t.Fatal(err)
			}
			models := policy["account_test_models"].(map[string]any)
			if _, retained := models["41"]; retained || !reflect.DeepEqual(models["42"], []any{"kept-model"}) {
				t.Errorf("deleted account model references retained or unrelated account changed: %#v", models)
			}
			scope := policy["scope"].(map[string]any)
			for _, field := range []string{"paused_account_ids", "excluded_account_ids", "manual_fused_account_ids"} {
				if !reflect.DeepEqual(scope[field], []any{"42"}) {
					t.Errorf("%s = %#v, want only kept account 42", field, scope[field])
				}
			}
		})
	}
}
