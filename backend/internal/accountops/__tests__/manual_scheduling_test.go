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

func TestManualControlUsesDedicatedSchedulingEndpoint(t *testing.T) {
	for _, scenario := range []struct {
		name             string
		initial, desired bool
		failure          string
	}{
		{name: "enable paused account", desired: true},
		{name: "disable scheduling", initial: true},
		{name: "already enabled", initial: true, desired: true},
		{name: "scheduling write rejected", desired: true, failure: "write"},
		{name: "scheduling readback mismatch", desired: true, failure: "readback"},
		{name: "final account read fails", desired: true, failure: "read-error"},
		{name: "final account identity differs", desired: true, failure: "identity"},
		{name: "parameters change after scheduling", desired: true, failure: "changed-parameters"},
		{name: "parameter mismatch prevents enabling", desired: true, failure: "parameters"},
		{name: "missing scheduling state prevents enabling", desired: true, failure: "missing"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newSettingsFixture(t)
			state := map[string]any{"id": 41, "name": "test-account", "priority": 20, "load_factor": 1, "concurrency": 3, "schedulable": scenario.initial}
			writes, schedulingWrites := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/41":
					writes++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if _, present := body["schedulable"]; present {
						t.Error("ordinary update must not carry unsupported schedulable")
					}
					// Match Sub2API UpdateAccountRequest: unknown schedulable is ignored.
					for _, key := range []string{"priority", "load_factor", "concurrency"} {
						if v, ok := body[key]; ok {
							state[key] = v
						}
					}
					if scenario.failure == "parameters" {
						state["concurrency"] = 2
					}
					if scenario.failure == "missing" {
						delete(state, "schedulable")
					}
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/41/schedulable":
					schedulingWrites++
					if writes != 1 || fmt.Sprint(state["priority"]) != "3" || fmt.Sprint(state["concurrency"]) != "100" {
						t.Error("scheduling before confirmed account parameters")
					}
					var body map[string]bool
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["schedulable"] != scenario.desired {
						t.Error("wrong scheduling request")
					}
					if scenario.failure == "write" {
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"message":"scheduling denied"}`))
						return
					}
					if scenario.failure != "readback" {
						state["schedulable"] = body["schedulable"]
					}
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41":
					if schedulingWrites > 0 {
						switch scenario.failure {
						case "read-error":
							w.WriteHeader(http.StatusForbidden)
							_, _ = w.Write([]byte(`{"message":"read denied"}`))
							return
						case "identity":
							state["id"] = 42
						case "changed-parameters":
							state["concurrency"] = 2
						}
					}
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": state})
			}))
			defer server.Close()
			service := accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
			service.UseTaskRunner(f.runner)
			if _, err := service.EnqueueManualPriority(context.Background(), "41", 3, "100", 100, scenario.desired, false, "test"); err != nil {
				t.Fatal(err)
			}
			f.runner.run(context.Background())
			if scenario.failure == "" {
				if f.tasks.last.Status != "succeeded" {
					t.Fatalf("manual control failed: %+v", f.tasks.last)
				}
				account, err := f.repository.Account(context.Background(), "41")
				if err != nil {
					t.Fatal(err)
				}
				if account.Schedulable == nil || *account.Schedulable != scenario.desired || account.Priority == nil || *account.Priority != 3 {
					t.Fatalf("incorrect committed state: %+v", account.AccountStatus)
				}
			} else {
				if f.tasks.last.Status != "failed" {
					t.Fatalf("unsafe success: %+v", f.tasks.last)
				}
				account, err := f.repository.Account(context.Background(), "41")
				if err != nil {
					t.Fatal(err)
				}
				if account.ManualPriority == nil {
					t.Fatal("partial remote write released manual protection")
				}
				if !strings.Contains(fmt.Sprint(f.tasks.last.Result), "管理平台") {
					t.Fatalf("partial failure lacks explanation: %+v", f.tasks.last)
				}
			}
			expectedSchedulingWrites := 1
			if scenario.initial == scenario.desired || scenario.failure == "parameters" || scenario.failure == "missing" {
				expectedSchedulingWrites = 0
			}
			if schedulingWrites != expectedSchedulingWrites || writes != 1 {
				t.Fatalf("writes=%d scheduling=%d expected=%d", writes, schedulingWrites, expectedSchedulingWrites)
			}
		})
	}
}
