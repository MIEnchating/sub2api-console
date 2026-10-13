package business_test

import (
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractKeyProtectionAndDeleteScope(t *testing.T) {
	for _, cfg := range dbtest.Configs(t) {
		t.Run(string(cfg.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			db, err := dbconn.Open(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			const host = "key-protection.contract.test"
			created, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: host, BaseURL: "https://" + host, UpstreamType: "sub2api", AuthMode: "apikey", RechargeRate: "1"})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				for _, q := range []string{`DELETE FROM onboarding_pending WHERE upstream_host='key-protection.contract.test'`, `DELETE FROM bindings WHERE upstream_host='key-protection.contract.test'`, `DELETE FROM upstream_keys WHERE host='key-protection.contract.test'`, `DELETE FROM recharge_rates WHERE host='key-protection.contract.test'`, `DELETE FROM upstreams WHERE host='key-protection.contract.test'`} {
					if _, err := db.ExecContext(t.Context(), q); err != nil {
						t.Error(err)
					}
				}
				if _, err := db.ExecContext(t.Context(), `DELETE FROM upstream_identities WHERE upstream_id=?`, created.UpstreamID); err != nil {
					t.Error(err)
				}
			}()
			for _, q := range []string{
				`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) VALUES('937001','key-protection.contract.test','key-1','bound','default','now')`,
				`INSERT INTO upstream_keys(host,key_id,name,updated_at) VALUES('key-protection.contract.test','unused','unused','now')`,
			} {
				if _, err := db.ExecContext(t.Context(), q); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(t.Context(), `INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at) SELECT id,?,upstream_key_id,'now' FROM bindings WHERE upstream_host=?`, created.UpstreamID, host); err != nil {
				t.Fatal(err)
			}
			var bindingID int64
			if err := db.QueryRowContext(t.Context(), `SELECT id FROM bindings WHERE upstream_host=?`, host).Scan(&bindingID); err != nil {
				t.Fatal(err)
			}
			scope := business.AccountDeleteScope{BindingID: bindingID, UpstreamID: created.UpstreamID, UpstreamKeyID: "key-1"}
			t.Run("unique stable binding allows delete preview", func(t *testing.T) {
				if err := store.ConfirmAccountDeleteScope(t.Context(), "937001", scope); err != nil {
					t.Fatal(err)
				}
			})
			t.Run("changed key rejects delete with a domain reason", func(t *testing.T) {
				changed := scope
				changed.UpstreamKeyID = "changed"
				err := store.ConfirmAccountDeleteScope(t.Context(), "937001", changed)
				if err == nil || !strings.Contains(err.Error(), "身份或 Key ID 已变化") {
					t.Fatalf("unexpected rejection: %v", err)
				}
			})
			for _, tc := range []struct {
				key  string
				want bool
			}{{"key-1", true}, {"unused", false}} {
				t.Run("key protection/"+tc.key, func(t *testing.T) {
					got, err := store.UpstreamKeyProtected(t.Context(), host, tc.key)
					if err != nil || got != tc.want {
						t.Fatalf("protected=%v want=%v error=%v", got, tc.want, err)
					}
				})
			}
			t.Run("unbound key reconciliation removes its projection", func(t *testing.T) {
				if err := store.ReconcileDeletedUnboundUpstreamKeyProjection(t.Context(), host, "unused"); err != nil {
					t.Fatal(err)
				}
				var n int
				if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM upstream_keys WHERE host=? AND key_id='unused'`, host).Scan(&n); err != nil || n != 0 {
					t.Fatalf("projection count=%d error=%v", n, err)
				}
			})
			t.Run("pending onboarding protects key and rejects account deletion", func(t *testing.T) {
				_, err := db.ExecContext(t.Context(), `INSERT INTO onboarding_pending(operation_id,upstream_id,upstream_host,upstream_type,upstream_key_id,upstream_account_id,upstream_group_id,upstream_group_name,local_group_id,local_group_name,multiplier,reason,created_at,updated_at) VALUES('key-protection-pending',? ,?,'sub2api','key-1','','7','group','1','local','1','pending','now','now')`, created.UpstreamID, host)
				if err != nil {
					t.Fatal(err)
				}
				err = store.ConfirmAccountDeleteScope(t.Context(), "937001", scope)
				if err == nil || !strings.Contains(err.Error(), "开户待续") {
					t.Fatalf("unexpected deletion result: %v", err)
				}
			})
		})
	}
}
