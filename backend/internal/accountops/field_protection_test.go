package accountops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestFieldSyncRejectsEndpointChangesAlongsideMultiplierUnderProtection(t *testing.T) {
	for _, protection := range []string{"monitoring", "manual-priority"} {
		for _, field := range []string{"base-url", "upstream-host"} {
			t.Run(protection+"/"+field, func(t *testing.T) {
				repository, _, _ := accountRepository(t)
				reason := "监控模式"
				if protection == "monitoring" {
					if _, err := repository.SetMode(context.Background(), runtimepolicy.Monitoring); err != nil {
						t.Fatal(err)
					}
				} else {
					reason = "人工优先位"
					if _, err := repository.AssignManualPriority(context.Background(), "41", 3, "100", 100, true, "operator"); err != nil {
						t.Fatal(err)
					}
				}
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					http.Error(w, "unexpected remote access", http.StatusBadRequest)
				}))
				defer server.Close()
				service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 2}}, repository, nil)
				multiplier, endpoint, host := "0.2", "https://changed.example", "changed.example"
				patch := FieldPatch{MultiplierPresent: true, Multiplier: &multiplier}
				if field == "base-url" {
					patch.BaseURLPresent, patch.BaseURL = true, &endpoint
				} else {
					patch.UpstreamHostPresent, patch.UpstreamHost = true, &host
				}
				_, err := service.SyncFields(context.Background(), "41", patch, "operator")
				if err == nil || !strings.Contains(err.Error(), reason) || requests.Load() != 0 {
					t.Fatalf("protected endpoint change reached remote: err=%v requests=%d", err, requests.Load())
				}
			})
		}
	}
}

func TestFieldSyncAllowsStandaloneBaseURLChangeInFullMode(t *testing.T) {
	repository, db, _ := accountRepository(t)
	var written atomic.Bool
	newURL := "https://changed.example/v1"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			var body struct {
				Credentials struct {
					BaseURL string `json:"base_url"`
				} `json:"credentials"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Credentials.BaseURL != newURL {
				t.Errorf("standalone Base URL request=%#v err=%v", body, err)
			}
			written.Store(true)
			_, _ = writer.Write([]byte(`{"success":true}`))
			return
		}
		baseURL := "https://old.example"
		if written.Load() {
			baseURL = newURL
		}
		_, _ = fmt.Fprintf(writer, `{"data":{"id":41,"name":"alpha","credentials":{"base_url":%q}}}`, baseURL)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 2}}, repository, nil)
	_, err := service.SyncFields(context.Background(), "41", FieldPatch{BaseURLPresent: true, BaseURL: &newURL}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	var baseURL string
	if err := db.QueryRow(`SELECT json_extract(metadata_json,'$.base_url') FROM accounts WHERE id='41'`).Scan(&baseURL); err != nil {
		t.Fatal(err)
	}
	if !written.Load() || baseURL != newURL {
		t.Fatalf("standalone Base URL change not projected: wrote=%v url=%q", written.Load(), baseURL)
	}
}

func TestQueuedClearManualPriorityRejectsRuntimeModeChangeBeforeRemoteAccess(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := repository.AssignManualPriority(context.Background(), "41", 3, "100", 100, true, "operator"); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected remote access", http.StatusBadRequest)
	}))
	defer server.Close()
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	runner := &deferredAccountRunner{}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 2}}, repository, tasks)
	service.UseTaskRunner(runner)
	if _, err := service.EnqueueClearManualPriority(context.Background(), "41", "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SetMode(context.Background(), runtimepolicy.Monitoring); err != nil {
		t.Fatal(err)
	}
	runner.Run(context.Background())
	finished := <-tasks.updates
	if finished.Status != "failed" || !strings.Contains(fmt.Sprint(finished.Result["error"]), "完全模式") || requests.Load() != 0 {
		t.Fatalf("restricted clear task=%#v requests=%d", finished, requests.Load())
	}
	var assignments int
	if err := db.QueryRow(`SELECT COUNT(*) FROM manual_priority_accounts WHERE account_id='41'`).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if assignments != 1 {
		t.Fatalf("manual assignment count=%d, want 1", assignments)
	}
}
