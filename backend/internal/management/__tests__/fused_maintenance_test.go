package management_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestManuallyFusedAccountMaintenanceUpdatesInformationWithoutResuming(t *testing.T) {
	for _, scenario := range []struct {
		operation      string
		variant        string
		automaticFuse  bool
		matchingURL    bool
		failedReadback bool
	}{
		{operation: "base-url-sync"},
		{operation: "base-url-repair"},
		{operation: "base-url-repair", variant: "automatic fuse", automaticFuse: true},
		{operation: "base-url-repair", variant: "already current", matchingURL: true},
		{operation: "base-url-repair", variant: "write not confirmed", failedReadback: true},
		{operation: "base-url-validation"},
		{operation: "host-repair"},
		{operation: "name-repair"},
		{operation: "defaults-repair"},
		{operation: "rate-sync"},
		{operation: "revalidation"},
		{operation: "snapshot"},
	} {
		t.Run(scenario.operation+"/"+scenario.variant, func(t *testing.T) {
			operation := scenario.operation
			store, db := rateCollectionStore(t)
			if _, err := db.Exec(`UPDATE accounts SET schedulable=0,routing_state='fused',upstream_host='old.example',
				metadata_json='{"platform":"openai","type":"apikey"}' WHERE id='11';
				INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,created_at,object_id)
				VALUES(-1,'onboard','account.onboarding','succeeded','done','now','11')`); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			remote := map[string]any{"id": 11, "name": "old-name", "platform": "openai", "type": "apikey",
				"schedulable": false, "status": "error", "rate_multiplier": 1, "balance": "12.5", "concurrency": 0, "priority": 0,
				"credentials": map[string]any{}}
			if operation != "base-url-repair" {
				remote["credentials"] = map[string]any{"base_url": "https://old.example"}
			}
			if scenario.matchingURL {
				remote["credentials"] = map[string]any{"base_url": "https://upstream.example"}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/admin/accounts/11":
					if r.Method == http.MethodPut {
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						for key, value := range body {
							if key == "schedulable" || key == "status" {
								t.Errorf("information maintenance changed control field %s", key)
							}
							if scenario.failedReadback {
								continue
							}
							remote[key] = value
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": remote})
				case "/api/v1/admin/accounts":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{remote}, "total": 1}})
				case "/api/v1/admin/groups":
					_, _ = w.Write([]byte(`{"data":{"items":[],"total":0}}`))
				case "/api/v1/admin/accounts/upstream-billing-probe/batch":
					_, _ = w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":0.5}}}]}}`))
				default:
					t.Errorf("unexpected maintenance request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tasks.Close() })
			target := snapshotTarget{server.URL}
			service := management.New(target, store, tasks, accountops.New(target, store, tasks))
			runner := &recoveryRunner{}
			service.UseTaskRunner(runner)
			var task taskstore.Task
			var countField string
			switch operation {
			case "base-url-sync":
				var id string
				id, err = service.EnqueueHostAccountBaseURLSync(t.Context(), "upstream.example", "test")
				task.ID, countField = id, "updated"
			case "base-url-repair":
				task, err = service.EnqueueAccountBaseURLRepair(t.Context(), []string{"11"}, "test")
				countField = "repaired"
			case "base-url-validation":
				task, err = service.EnqueueAccountBaseURLValidation(t.Context(), []string{"11"}, "test")
				countField = "resolved"
			case "host-repair":
				task, err = service.EnqueueAccountUpstreamHostRepair(t.Context(), []string{"11"}, "test")
				countField = "repaired"
			case "name-repair":
				task, err = service.EnqueueAccountNameRepair(t.Context(), []string{"11"}, "test")
				countField = "renamed"
			case "defaults-repair":
				task, err = service.EnqueueAccountDefaultsRepair(t.Context(), []string{"11"}, "test")
				countField = "repaired"
			case "rate-sync":
				task, err = service.EnqueueAccountRateSync(t.Context(), []string{"11"}, "test")
				countField = "updated"
			case "revalidation":
				task, err = service.EnqueueAccountRevalidation(t.Context(), []string{"11"}, "test")
				countField = "verified"
			case "snapshot":
				task, err = service.EnqueueSync(t.Context(), "test")
				countField = "accounts"
			}
			if err != nil {
				t.Fatal(err)
			}
			// Fuse after enqueue to exercise the execution-time protection check.
			if !scenario.automaticFuse {
				if err := store.CommitAccountControlReadback(t.Context(), "11", "fuse", "test", false, business.AccountOperation{
					OperationID: "test-fuse", OperationType: "account.control", State: "succeeded", Phase: "readback",
					RemoteConfirmed: true, ReadbackConfirmed: true,
				}); err != nil {
					t.Fatal(err)
				}
			}
			runner.runs[0](context.Background())
			final, err := tasks.Get(t.Context(), task.ID)
			wantStatus := "succeeded"
			if scenario.failedReadback {
				wantStatus, countField = "failed", "failed"
			}
			if err != nil || final.Status != wantStatus || final.Result[countField] != json.Number("1") {
				t.Fatalf("maintenance did not update the fused account: task=%+v err=%v", final, err)
			}
			protection, err := store.AccountMutationProtection(t.Context(), "11")
			if err != nil || protection.ManualFused == scenario.automaticFuse {
				t.Fatalf("maintenance removed the fuse: %+v %v", protection, err)
			}
			var schedulable bool
			if err := db.QueryRow(`SELECT schedulable FROM accounts WHERE id='11'`).Scan(&schedulable); err != nil || schedulable {
				t.Fatalf("maintenance resumed local scheduling: %v %v", schedulable, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if remote["schedulable"] != false || remote["status"] != "error" {
				t.Fatalf("maintenance changed remote control state: %+v", remote)
			}
			if scenario.failedReadback {
				if final.Result["remote_write"] != true {
					t.Fatal("failed confirmation hid the attempted remote write")
				}
				return
			}
			switch operation {
			case "base-url-sync", "base-url-repair":
				if remote["credentials"].(map[string]any)["base_url"] != "https://upstream.example" {
					t.Fatalf("address not updated: %+v", remote)
				}
			case "name-repair":
				if remote["name"] != "Relay-1" {
					t.Fatalf("name not updated: %+v", remote)
				}
			case "rate-sync":
				if remote["rate_multiplier"] != float64(0.5) || remote["name"] != "Relay-0.5" {
					t.Fatalf("rate and name not updated: %+v", remote)
				}
			case "snapshot":
				var balance string
				if err := db.QueryRow(`SELECT balance FROM accounts WHERE id='11'`).Scan(&balance); err != nil || balance != "12.5" {
					t.Fatalf("fused account balance not refreshed: %q %v", balance, err)
				}
			}
		})
	}
}
