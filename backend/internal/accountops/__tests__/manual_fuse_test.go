package accountops_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestManualPriorityAllowsConfirmedFuseAndRecoveryWithoutChangingAssignment(t *testing.T) {
	for _, syncBalance := range []bool{false, true} {
		t.Run(map[bool]string{false: "without rate sync", true: "with rate sync"}[syncBalance], func(t *testing.T) {
			f := newSettingsFixture(t)
			if _, err := f.repository.SetMode(t.Context(), runtimepolicy.Full); err != nil {
				t.Fatal(err)
			}
			if _, err := f.repository.AssignManualPriority(t.Context(), "41", 3, "100", 100, syncBalance, "test"); err != nil {
				t.Fatal(err)
			}
			before, err := f.repository.Account(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			schedulable := true
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/41/schedulable" {
					var body struct {
						Schedulable bool `json:"schedulable"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					schedulable = body.Schedulable
				} else if r.Method == http.MethodPut {
					t.Error("control must not overwrite manual parameters")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "name": "test-account", "schedulable": schedulable}})
			}))
			t.Cleanup(server.Close)
			service := accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
			service.UseTaskRunner(f.runner)
			for _, action := range []string{"fuse", "recover", "resume"} {
				if action == "resume" {
					if err := f.repository.CommitAccountControlReadback(t.Context(), "41", "pause", "test", false, business.AccountOperation{OperationID: "seed-pause", OperationType: "pause", State: "succeeded", Actor: "test", Phase: "readback"}); err != nil {
						t.Fatal(err)
					}
					schedulable = false
				}
				if _, err := service.EnqueueControl(t.Context(), "41", action, "test"); err != nil {
					t.Fatal(err)
				}
				f.runner.run(t.Context())
				if f.tasks.last.Status != "succeeded" {
					t.Fatalf("%s failed: %+v", action, f.tasks.last)
				}
				after, err := f.repository.Account(t.Context(), "41")
				if err != nil {
					t.Fatal(err)
				}
				wantHealth := "manual_priority"
				if action == "fuse" {
					wantHealth = "fused"
				}
				if after.Health != wantHealth || after.Schedulable == nil || *after.Schedulable != (action != "fuse") {
					t.Fatalf("%s state: %+v", action, after.AccountStatus)
				}
				if !reflect.DeepEqual(before.ManualPriority, after.ManualPriority) || !reflect.DeepEqual(before.Priority, after.Priority) || !reflect.DeepEqual(before.LoadFactor, after.LoadFactor) || !reflect.DeepEqual(before.Concurrency, after.Concurrency) || after.ManualSyncBalanceMultiplier != syncBalance {
					t.Fatalf("%s changed manual settings: %+v", action, after.AccountStatus)
				}
				if action == "fuse" && (after.DecisionReason == nil || *after.DecisionReason != "人工熔断，等待手动解除") {
					t.Fatal("manual fuse reason missing")
				}
			}
		})
	}
}

func TestConfirmedManualPriorityFuseRemainsVisibleInEveryRuntimeMode(t *testing.T) {
	for _, mode := range []string{runtimepolicy.Full, runtimepolicy.Monitoring} {
		t.Run(mode, func(t *testing.T) {
			f := newSettingsFixture(t)
			if _, err := f.repository.AssignManualPriority(t.Context(), "41", 3, "100", 100, true, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.repository.SetMode(t.Context(), mode); err != nil {
				t.Fatal(err)
			}
			if err := f.repository.CommitAccountControlReadback(t.Context(), "41", "fuse", "test", false, business.AccountOperation{OperationID: "fuse", OperationType: "account-control", State: "succeeded", Actor: "test", Phase: "readback"}); err != nil {
				t.Fatal(err)
			}
			state, err := f.repository.AccountControlState(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if state.Health != "fused" || state.ManualPriority == nil || *state.ManualPriority != 3 {
				t.Fatalf("manual fuse hidden: %+v", state)
			}
		})
	}
}

func TestManualPriorityControlFailurePreservesLocalSchedulingAndAssignment(t *testing.T) {
	for _, failure := range []string{"monitoring", "write", "readback"} {
		for _, action := range []string{"fuse", "recover", "resume"} {
			t.Run(failure+"/"+action, func(t *testing.T) {
				f := newSettingsFixture(t)
				mode := runtimepolicy.Full
				if failure == "monitoring" {
					mode = runtimepolicy.Monitoring
				}
				if _, err := f.repository.SetMode(t.Context(), mode); err != nil {
					t.Fatal(err)
				}
				if _, err := f.repository.AssignManualPriority(t.Context(), "41", 3, "100", 100, false, "test"); err != nil {
					t.Fatal(err)
				}
				initial := action == "fuse"
				if !initial {
					seed := "fuse"
					if action == "resume" {
						seed = "pause"
					}
					if err := f.repository.CommitAccountControlReadback(t.Context(), "41", seed, "test", false, business.AccountOperation{OperationID: "seed", OperationType: "account.control", State: "succeeded", Phase: "readback"}); err != nil {
						t.Fatal(err)
					}
				}
				before, err := f.repository.AccountControlState(t.Context(), "41")
				if err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if failure == "monitoring" {
						t.Error("monitoring mode reached remote")
					}
					if r.Method == http.MethodPost && failure == "write" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "name": "test-account", "schedulable": initial}})
				}))
				t.Cleanup(server.Close)
				service := accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
				if _, err := service.Control(t.Context(), "41", action, "test"); err == nil {
					t.Fatal("unconfirmed control succeeded")
				}
				after, err := f.repository.AccountControlState(t.Context(), "41")
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("failed control changed local state: before=%+v after=%+v", before, after)
				}
			})
		}
	}
}
