package accountops_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
)

func TestSettingsSchedulingMismatchSyncsBeforeSavingConsoleState(t *testing.T) {
	for _, scenario := range []struct {
		name             string
		initial, desired bool
		stale            bool
	}{
		{name: "pause syncs dedicated endpoint", initial: true},
		{name: "resume syncs dedicated endpoint", desired: true},
		{name: "stale pause readback keeps console pause", initial: true, stale: true},
		{name: "stale resume readback keeps console resume", desired: true, stale: true},
		{name: "matching state does not write scheduling", initial: true, desired: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newSettingsFixture(t)
			remote := newSettingsSchedulingRemote(t, f, scenario.initial, scenario.stale, "")
			runSchedulingSettings(t, f, scenario.desired)
			if f.tasks.last.Status != "succeeded" {
				t.Fatalf("settings failed: %+v", f.tasks.last)
			}
			wantSyncs := 1
			if scenario.initial == scenario.desired {
				wantSyncs = 0
			}
			if len(remote.schedulingWrites) != wantSyncs || remote.puts != 1 {
				t.Fatalf("expected one settings write and %d scheduling syncs, got puts=%d syncs=%v", wantSyncs, remote.puts, remote.schedulingWrites)
			}
			if wantSyncs == 1 && remote.schedulingWrites[0] != scenario.desired {
				t.Fatalf("sync used wrong state: %v", remote.schedulingWrites)
			}
			account, err := f.repository.Account(context.Background(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if account.Schedulable == nil || *account.Schedulable != scenario.desired || account.Paused == nil || *account.Paused != !scenario.desired || account.Concurrency == nil || *account.Concurrency != 5 {
				t.Fatalf("console settings were not committed: %+v", account.AccountStatus)
			}
			if f.tasks.last.Result["readback_confirmed"] != !scenario.stale {
				t.Fatalf("task misreported readback: %+v", f.tasks.last.Result)
			}
			events, err := f.repository.AuditEvents(context.Background(), nil, true)
			if err != nil || len(events) != 1 {
				t.Fatalf("missing settings audit: events=%+v err=%v", events, err)
			}
			if events[0].ReadbackConfirmed == nil || *events[0].ReadbackConfirmed != !scenario.stale {
				t.Fatalf("audit misreported readback: %+v", events[0])
			}
		})
	}
}

func TestSettingsSchedulingErrorsRollbackInsteadOfUsingConsoleFallback(t *testing.T) {
	for _, failure := range []string{"write", "read", "identity", "parameters", "missing", "invalid", "initial-parameters"} {
		t.Run(failure, func(t *testing.T) {
			f := newSettingsFixture(t)
			remote := newSettingsSchedulingRemote(t, f, false, false, failure)
			runSchedulingSettings(t, f, true)
			if f.tasks.last.Status != "failed" {
				t.Fatalf("expected failure, got %+v", f.tasks.last)
			}
			account, err := f.repository.Account(context.Background(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if account.Concurrency == nil || *account.Concurrency != 3 {
				t.Fatalf("failed save changed local settings: %+v", account.AccountStatus)
			}
			if remote.puts != 2 || fmt.Sprint(remote.state["concurrency"]) != "3" || remote.state["schedulable"] != false {
				t.Fatalf("remote rollback incomplete: puts=%d state=%v", remote.puts, remote.state)
			}
			if failure == "initial-parameters" && len(remote.schedulingWrites) != 0 {
				t.Fatalf("enabled account before parameter verification: %v", remote.schedulingWrites)
			}
			if failure != "initial-parameters" && (len(remote.schedulingWrites) != 2 || !remote.schedulingWrites[0] || remote.schedulingWrites[1]) {
				t.Fatalf("expected scheduling sync followed by rollback: %v", remote.schedulingWrites)
			}
		})
	}
}

type settingsSchedulingRemote struct {
	state            map[string]any
	puts             int
	schedulingWrites []bool
}

func newSettingsSchedulingRemote(t *testing.T, f *settingsFixture, initial, stale bool, failure string) *settingsSchedulingRemote {
	t.Helper()
	remote := &settingsSchedulingRemote{state: map[string]any{
		"id": 41, "name": "test-account", "priority": 20,
		"load_factor": 1, "concurrency": 3, "schedulable": initial,
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/41":
			remote.puts++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			// The ordinary Sub2API update ignores schedulable.
			for _, key := range []string{"priority", "load_factor", "concurrency"} {
				if value, ok := body[key]; ok {
					remote.state[key] = value
				}
			}
			if failure == "initial-parameters" && remote.puts == 1 {
				remote.state["concurrency"] = 2
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts/41/schedulable":
			var body struct {
				Schedulable bool `json:"schedulable"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			remote.schedulingWrites = append(remote.schedulingWrites, body.Schedulable)
			if failure == "write" && len(remote.schedulingWrites) == 1 {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"scheduling denied"}`))
				return
			}
			if !stale {
				remote.state["schedulable"] = body.Schedulable
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41":
			if len(remote.schedulingWrites) == 1 {
				switch failure {
				case "read":
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message":"read denied"}`))
					return
				case "identity":
					remote.state["id"] = 42
				case "parameters":
					remote.state["concurrency"] = 2
				case "missing":
					delete(remote.state, "schedulable")
				case "invalid":
					remote.state["schedulable"] = "true"
				}
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": remote.state})
	}))
	t.Cleanup(server.Close)
	f.service = accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
	f.service.UseTaskRunner(f.runner)
	return remote
}

func runSchedulingSettings(t *testing.T, f *settingsFixture, desired bool) {
	t.Helper()
	_, err := f.service.EnqueueSettings(context.Background(), "41", accountops.SettingsInput{
		Priority: 20, LoadFactor: "7", Concurrency: 5, Paused: !desired,
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	f.runner.run(context.Background())
}
