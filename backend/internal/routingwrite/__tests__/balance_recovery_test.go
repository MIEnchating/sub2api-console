package routingwrite_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestBalanceRecoveryPreservesSchedulingAndRejectsUnsafeRecovery(t *testing.T) {
	for _, scenario := range []string{"enabled", "disabled", "still-empty", "missing", "failed", "unrelated", "monitoring", "rebound", "no-baseline", "clear-failed", "schedule-failed", "restart", "paused", "write-disabled", "remote-rebound", "target-changed", "healthy-baseline", "capacity-unknown", "enabled-before-depletion", "clear-readback-failed", "generic-403"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "balance.db")
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
			if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "balance.example", BaseURL: "https://balance.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
				t.Fatal(err)
			}
			enabled := scenario != "disabled"
			account := map[string]any{"id": "41", "name": "balance-account", "status": "active", "schedulable": enabled, "priority": 1, "concurrency": 1, "load_factor": 1, "type": "apikey", "credentials": map[string]any{"base_url": "https://balance.example", "api_key": "isolated-key"}, "groups": []any{json.Number("7")}}
			if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{account}, []map[string]any{{"id": "7", "name": "group"}}, "test"); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at) VALUES(1,'41','balance.example','10','key','65','group','now'); INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at) SELECT 1,upstream_id,'10','65','now' FROM upstream_identity_hosts WHERE host='balance.example'`); err != nil {
				t.Fatal(err)
			}
			clears, failClear, failSchedule, ignoreClear := 0, false, false, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					switch {
					case strings.HasSuffix(r.URL.Path, "/clear-error"):
						clears++
						if failClear {
							http.Error(w, "rejected", 422)
							return
						}
						if !ignoreClear {
							account["status"] = "active"
							account["error_message"] = ""
						}
					case strings.HasSuffix(r.URL.Path, "/schedulable"):
						if failSchedule {
							http.Error(w, "rejected", 422)
							return
						}
						var body struct {
							Schedulable bool `json:"schedulable"`
						}
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						account["schedulable"] = body.Schedulable
					default:
						t.Errorf("unexpected mutation: %s", r.URL.Path)
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": account})
			}))
			t.Cleanup(server.Close)
			recovery := routingwrite.New(routingTarget{url: server.URL}, store)
			syncBalance := func(balance string) {
				t.Helper()
				if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "balance.example", AuthenticationOK: true, Balance: &business.UpstreamBalanceObservation{RawBalance: &balance, Status: "已读取"}}); err != nil {
					t.Fatal(err)
				}
			}
			run := func() {
				t.Helper()
				if _, err := recovery.ReconcileBalanceRecovery(t.Context(), "balance.example", "test"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "enabled-before-depletion" {
				account["schedulable"] = false
				syncBalance("10")
				run()
				account["schedulable"] = true
			}
			if scenario == "healthy-baseline" {
				syncBalance("10")
				run()
			}
			if scenario != "no-baseline" && scenario != "healthy-baseline" {
				syncBalance("0")
				run()
			}
			account["schedulable"] = false
			account["status"] = "error"
			account["error_message"] = "Access forbidden (403): insufficient balance"
			if scenario != "no-baseline" {
				syncBalance("0")
				run()
			}
			syncBalance("10")
			switch scenario {
			case "still-empty":
				syncBalance("0")
			case "missing":
				_, err = store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "balance.example", Balance: &business.UpstreamBalanceObservation{Status: "未返回余额"}})
			case "failed":
				err = store.RecordUpstreamSyncFailure(t.Context(), "balance.example", "balance", "读取失败", false)
			case "generic-403":
				account["error_message"] = "Access forbidden (403)"
			case "unrelated":
				account["error_message"] = "Access forbidden (403): IP not allowed"
			case "monitoring":
				_, err = store.SetMode(t.Context(), runtimepolicy.Monitoring)
			case "rebound":
				_, err = db.Exec(`UPDATE bindings SET upstream_key_id='11'; UPDATE binding_identities SET upstream_key_id='11'`)
			case "clear-failed":
				failClear = true
			case "schedule-failed":
				failSchedule = true
			case "clear-readback-failed":
				ignoreClear = true
			case "restart":
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				store, err = business.Open(path)
				recovery = routingwrite.New(routingTarget{url: server.URL}, store)
			case "paused":
				_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"paused_account_ids": []any{"41"}}}}, "test")
			case "write-disabled":
				_, err = store.UpdatePolicy(t.Context(), map[string]any{"auto_apply": map[string]any{"schedulable": false}}, "test")
			case "remote-rebound":
				account["credentials"] = map[string]any{"base_url": "https://different.example", "api_key": "different-isolated-key"}
			case "target-changed":
				recovery = routingwrite.New(routingTarget{url: server.URL + "/changed"}, store)
			case "capacity-unknown":
				_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true}}}, "test")
			}
			if err != nil {
				t.Fatal(err)
			}
			remoteWrite, recoveryErr := recovery.ReconcileBalanceRecovery(t.Context(), "balance.example", "test")
			err = recoveryErr
			if remoteWrite != (clears > 0) {
				t.Fatal("remote-write result does not match attempted mutations")
			}
			if scenario == "clear-failed" || scenario == "schedule-failed" || scenario == "capacity-unknown" || scenario == "clear-readback-failed" {
				if err == nil {
					t.Fatal("remote failure was hidden")
				}
				failClear, failSchedule, ignoreClear = false, false, false
				if scenario == "capacity-unknown" {
					if clears != 0 {
						t.Fatal("capacity check allowed remote write")
					}
					return
				}
				run()
			} else if err != nil {
				t.Fatal(err)
			}
			if scenario == "enabled" || scenario == "disabled" || scenario == "clear-failed" || scenario == "schedule-failed" || scenario == "restart" || scenario == "healthy-baseline" || scenario == "enabled-before-depletion" || scenario == "clear-readback-failed" || scenario == "generic-403" {
				if account["status"] != "active" || account["schedulable"] != enabled {
					t.Fatalf("lost scheduling intent: %v", account)
				}
				before := clears
				run()
				if clears != before {
					t.Fatal("completed recovery repeated")
				}
			} else if clears != 0 {
				t.Fatalf("unsafe recovery sent %d writes", clears)
			}
		})
	}
}
