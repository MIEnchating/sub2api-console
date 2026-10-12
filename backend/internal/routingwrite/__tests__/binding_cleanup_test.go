package routingwrite_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestMissingBindingDeletionRechecksEvidenceAndRecordsResult(t *testing.T) {
	for _, scenario := range []string{"delete", "recovered", "rebound", "disabled", "monitoring", "remote-failure"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cleanup.db")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"binding_cleanup": map[string]any{"enabled": true}}}, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "cleanup.example", BaseURL: "https://cleanup.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
				t.Fatal(err)
			}
			account := map[string]any{"id": "41", "name": "missing-group", "schedulable": false, "priority": 1000, "load_factor": 10, "concurrency": 1, "status": "active", "groups": []any{json.Number("7")}, "rate_multiplier": "1"}
			if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{account}, []map[string]any{{"id": "7", "name": "group"}}, "test"); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			_, err = db.Exec(`UPDATE upstreams SET metadata_json=json_object('catalog_status','已同步');
    INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at) VALUES(1,'41','cleanup.example','10','key','65','group','now');
    INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at) SELECT 1,upstream_id,'10','65','now' FROM upstream_identity_hosts;
    INSERT INTO upstream_catalog_entities(upstream_id,entity_kind,entity_id,parent_entity_id,name,lifecycle_state,missing_observations,confirmed_missing_at,updated_at) SELECT upstream_id,'key','10','65','key','missing',4,'confirmed','now' FROM upstream_identity_hosts;`)
			if err != nil {
				t.Fatal(err)
			}
			calculated, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if calculated.AccountTargets["41"].CleanupAction == nil {
				t.Fatalf("missing cleanup target: %+v", calculated)
			}
			switch scenario {
			case "recovered":
				_, err = db.Exec(`UPDATE upstream_catalog_entities SET lifecycle_state='active'`)
			case "rebound":
				_, err = db.Exec(`UPDATE bindings SET upstream_group_id='66'; UPDATE binding_identities SET upstream_group_id='66'`)
			case "disabled":
				_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"binding_cleanup": map[string]any{"enabled": false}}}, "test")
			case "monitoring":
				_, err = store.SetMode(t.Context(), runtimepolicy.Monitoring)
			}
			if err != nil {
				t.Fatal(err)
			}
			var deletes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodDelete {
					deletes.Add(1)
					if scenario == "remote-failure" {
						http.Error(w, "delete rejected", 422)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"deleted": true}})
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write: %s", r.Method)
					w.WriteHeader(405)
					return
				}
				if deletes.Load() > 0 && scenario == "delete" && r.URL.Path == "/api/v1/admin/accounts/41" {
					http.NotFound(w, r)
					return
				}
				var data any = account
				if r.URL.Path == "/api/v1/admin/accounts" {
					data = map[string]any{"items": []any{account}, "total": 1}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
			}))
			t.Cleanup(server.Close)
			result, err := routingwrite.New(routingTarget{url: server.URL}, store).Apply(t.Context(), calculated.AccountTargets, "test")
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE id='41'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "delete":
				if deletes.Load() != 1 || count != 0 || result.Failed != 0 || result.Changed != 1 {
					t.Fatalf("deletion not completed: %+v count=%d requests=%d", result, count, deletes.Load())
				}
				audit, err := store.AuditEvents(t.Context(), nil, false)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, event := range audit {
					if event.OperationType == "cleanup.delete" && event.State == "succeeded" {
						found = true
					}
				}
				if !found {
					t.Fatal("missing successful deletion audit")
				}
			case "remote-failure":
				if deletes.Load() != 1 || count != 1 || result.Failed != 1 {
					t.Fatalf("failed deletion lost account: %+v", result)
				}
			default:
				if deletes.Load() != 0 || count != 1 || result.RemoteWrite {
					t.Fatalf("stale deletion was sent: %+v", result)
				}
			}
		})
	}
}
