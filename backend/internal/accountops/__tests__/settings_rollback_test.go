package accountops_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
)

func TestSettingsRollbackReportsWhetherOriginalRemoteValuesWereConfirmed(t *testing.T) {
	for _, scenario := range []string{"restored", "null-load", "omitted-load", "priority", "load_factor", "concurrency", "schedulable", "identity", "read-failed", "write-failed"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSettingsFixture(t)
			state := map[string]any{"id": 41, "priority": 20, "load_factor": 1, "concurrency": 3, "schedulable": true}
			if scenario == "null-load" {
				state["load_factor"] = nil
			}
			if scenario == "omitted-load" {
				delete(state, "load_factor")
			}
			puts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/api/v1/admin/accounts/41" {
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				switch r.Method {
				case http.MethodPut:
					puts++
					var body map[string]any
					decoder := json.NewDecoder(r.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&body); err != nil {
						t.Error(err)
						return
					}
					if puts == 2 && scenario == "write-failed" {
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"message":"rollback denied"}`))
						return
					}
					for key, value := range body {
						if puts == 2 && key == scenario {
							continue
						}
						state[key] = value
					}
					if puts == 1 {
						state["concurrency"] = 4
					} // Trigger parameter readback failure.
					if puts == 2 && scenario == "null-load" {
						state["load_factor"] = nil
					}
					if puts == 2 && scenario == "omitted-load" {
						delete(state, "load_factor")
					}
				case http.MethodGet:
					if puts == 2 && scenario == "identity" {
						state["id"] = 42
					}
					if puts == 2 && scenario == "read-failed" {
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"message":"readback denied"}`))
						return
					}
				default:
					t.Errorf("unexpected method: %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": state})
			}))
			t.Cleanup(server.Close)
			f.service = accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
			f.service.UseTaskRunner(f.runner)
			if _, err := f.service.EnqueueSettings(t.Context(), "41", accountops.SettingsInput{
				Priority: 21, LoadFactor: "7", Concurrency: 5, Paused: true,
			}, "test"); err != nil {
				t.Fatal(err)
			}
			f.runner.run(context.Background())
			confirmed := scenario == "restored" || scenario == "null-load" || scenario == "omitted-load"
			if f.tasks.last.Status != "failed" || puts != 2 {
				t.Fatalf("failed settings must attempt one rollback: task=%+v writes=%d", f.tasks.last, puts)
			}
			message := fmt.Sprint(f.tasks.last.Result["error"])
			if !strings.Contains(message, "并发上限读回不一致") {
				t.Fatalf("lost original failure: %s", message)
			}
			if strings.Contains(message, "回滚未确认") == confirmed || f.tasks.last.Result["remote_write"] != !confirmed {
				t.Fatalf("rollback result misreported: confirmed=%t result=%+v", confirmed, f.tasks.last.Result)
			}
			if !confirmed && !strings.Contains(message, "核对管理端") {
				t.Fatalf("missing next step: %s", message)
			}
			account, err := f.repository.Account(t.Context(), "41")
			if err != nil || account.Concurrency == nil || *account.Concurrency != 3 {
				t.Fatalf("failed settings changed local values: account=%+v err=%v", account, err)
			}
			events, err := f.repository.AuditEvents(t.Context(), nil, true)
			if err != nil || len(events) != 0 {
				t.Fatalf("failed settings recorded success: events=%+v err=%v", events, err)
			}
		})
	}
}

func TestSettingsRollbackVerifiesDedicatedSchedulingRestore(t *testing.T) {
	for _, restoreScheduling := range []bool{true, false} {
		t.Run(fmt.Sprintf("restore=%t", restoreScheduling), func(t *testing.T) {
			f := newSettingsFixture(t)
			state := map[string]any{"id": 41, "priority": 20, "load_factor": 1, "concurrency": 3, "schedulable": false}
			schedulingWrites := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/41":
					var body map[string]any
					decoder := json.NewDecoder(r.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&body); err != nil {
						t.Error(err)
						return
					}
					for _, key := range []string{"priority", "load_factor", "concurrency"} {
						state[key] = body[key]
					}
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/41/schedulable":
					schedulingWrites++
					var body struct {
						Schedulable bool `json:"schedulable"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					if schedulingWrites == 1 || restoreScheduling {
						state["schedulable"] = body.Schedulable
					}
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41":
					if schedulingWrites == 1 {
						state["concurrency"] = 4
					}
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": state})
			}))
			t.Cleanup(server.Close)
			f.service = accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
			f.service.UseTaskRunner(f.runner)
			runSchedulingSettings(t, f, true)
			if f.tasks.last.Status != "failed" || schedulingWrites != 2 {
				t.Fatalf("expected failed save with scheduling restore: task=%+v writes=%d", f.tasks.last, schedulingWrites)
			}
			if f.tasks.last.Result["remote_write"] != !restoreScheduling || strings.Contains(f.tasks.last.Message, "回滚未确认") == restoreScheduling {
				t.Fatalf("scheduling restore misreported: %+v", f.tasks.last)
			}
		})
	}
}
