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

func TestAccountSettingsNullableLoadFactor(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		original       any
		follow         bool
		rejectReadback bool
	}{
		{name: "set fixed from null", original: nil},
		{name: "keep following concurrency", original: nil, follow: true},
		{name: "clear fixed load factor", original: "3", follow: true},
		{name: "failed readback restores null", original: nil, rejectReadback: true},
		{name: "failed clear restores fixed", original: "3", follow: true, rejectReadback: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newSettingsFixture(t)
			state := map[string]any{"id": 41, "name": "test-account", "priority": 20, "load_factor": scenario.original, "concurrency": 3, "schedulable": true}
			var puts int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPut {
					puts++
					var body map[string]any
					decoder := json.NewDecoder(r.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&body); err != nil {
						t.Error(err)
					}
					for key, value := range body {
						state[key] = value
					}
					if fmt.Sprint(state["load_factor"]) == "0" {
						state["load_factor"] = nil
					}
					if puts == 1 && scenario.rejectReadback {
						state["load_factor"] = "99"
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": state})
			}))
			defer server.Close()
			service := accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
			service.UseTaskRunner(f.runner)
			_, err := service.EnqueueSettings(context.Background(), "41", accountops.SettingsInput{Priority: 20, LoadFactor: "7", FollowConcurrency: scenario.follow, Concurrency: 5}, "test")
			if err != nil {
				t.Fatal(err)
			}
			f.runner.run(context.Background())
			if scenario.rejectReadback {
				if f.tasks.last.Status != "failed" || puts != 2 {
					t.Fatalf("expected failed task and rollback: status=%s puts=%d", f.tasks.last.Status, puts)
				}
				if fmt.Sprint(state["load_factor"]) != fmt.Sprint(scenario.original) {
					t.Fatalf("rollback changed original load: %v", state["load_factor"])
				}
				account, err := f.repository.Account(context.Background(), "41")
				if err != nil || account.Concurrency == nil || *account.Concurrency != 3 {
					t.Fatalf("failed write changed local state: %v", err)
				}
				return
			}
			if f.tasks.last.Status != "succeeded" || puts != 1 {
				t.Fatalf("settings failed: status=%s message=%s puts=%d", f.tasks.last.Status, f.tasks.last.Message, puts)
			}
			account, err := f.repository.Account(context.Background(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if scenario.follow {
				if account.LoadFactor != nil || state["load_factor"] != nil {
					t.Fatalf("following mode persisted a fixed value: %v", account.LoadFactor)
				}
			} else if account.LoadFactor == nil || *account.LoadFactor != "7" {
				t.Fatalf("fixed load not saved: %v", account.LoadFactor)
			}
			if account.Concurrency == nil || *account.Concurrency != 5 {
				t.Fatal("new concurrency not saved")
			}
		})
	}
}

func TestAccountSettingsMissingOriginalLoadFactorDoesNotWrite(t *testing.T) {
	f := newSettingsFixture(t)
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "priority": 20, "concurrency": 3, "schedulable": true}})
	}))
	defer server.Close()
	service := accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
	service.UseTaskRunner(f.runner)
	if _, err := service.EnqueueSettings(context.Background(), "41", accountops.SettingsInput{Priority: 20, FollowConcurrency: true, Concurrency: 5}, "test"); err != nil {
		t.Fatal(err)
	}
	f.runner.run(context.Background())
	if puts != 0 || f.tasks.last.Status != "failed" {
		t.Fatalf("missing field must not be treated as explicit null: puts=%d status=%s", puts, f.tasks.last.Status)
	}
}
