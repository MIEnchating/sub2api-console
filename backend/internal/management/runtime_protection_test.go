package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type maintenanceModeRepository struct {
	*protectedCaptureRepository
	mode string
}

func (r *maintenanceModeRepository) Mode(context.Context) (string, error) { return r.mode, nil }

func TestQueuedRemoteMaintenanceRejectsRestrictedMode(t *testing.T) {
	for _, operation := range []string{"account-base-url-sync", "account-base-url-repair", "account-name-repair", "account-defaults-repair", "account-configuration-check"} {
		t.Run(operation, func(t *testing.T) {
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes.Add(1)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 11, "name": "old-name", "platform": "openai", "type": "apikey", "priority": 0, "concurrency": 0, "schedulable": false, "status": "error"}})
			}))
			defer server.Close()
			repository := &maintenanceModeRepository{protectedCaptureRepository: &protectedCaptureRepository{captureRepository: &captureRepository{maintenance: []business.BoundAccountMaintenance{{AccountID: "11", AccountName: "old-name", ExpectedName: "new-name", UpstreamHost: "upstream.example", NamingBaseURL: "https://relay.example", ConsoleOnboarded: true}}}}, mode: runtimepolicy.Full}
			tasks := &memoryTasks{}
			runner := &deferredManagementRunner{}
			service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 1}}, repository, tasks)
			service.UseTaskRunner(runner)
			if _, err := service.enqueueMaintenance(context.Background(), operation, "queued", []string{"11"}, "operator"); err != nil {
				t.Fatal(err)
			}
			repository.mode = runtimepolicy.Monitoring
			runner.Run(context.Background())
			if writes.Load() != 0 {
				t.Fatalf("restricted mode still performed %d remote writes", writes.Load())
			}
			if len(tasks.values) == 0 || tasks.values[len(tasks.values)-1].Status != "failed" {
				t.Fatalf("restricted task not failed: %#v", tasks.values)
			}
		})
	}
}

func TestBaseURLSyncPreservesManuallyProtectedAccounts(t *testing.T) {
	for _, protection := range []business.AccountMutationProtection{{ManualPriority: true}, {Paused: true}, {Excluded: true}, {ManualFused: true}} {
		t.Run(protection.Reasons()[0], func(t *testing.T) {
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes.Add(1)
				}
				_, _ = w.Write([]byte(`{"data":{"id":11,"credentials":{"base_url":"https://old.example"}}}`))
			}))
			defer server.Close()
			repository := &protectedCaptureRepository{captureRepository: &captureRepository{maintenance: []business.BoundAccountMaintenance{{AccountID: "11", UpstreamHost: "upstream.example", NamingBaseURL: "https://relay.example"}}}, protections: map[string]business.AccountMutationProtection{"11": protection}}
			service := New(staticTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test", TimeoutSeconds: 1}}, repository, &memoryTasks{})
			result, err := service.syncAccountBaseURLs(context.Background(), []string{"11"}, "operator")
			if err != nil || writes.Load() != 0 || result["skipped"] != 1 {
				t.Fatalf("protected account changed: writes=%d result=%#v error=%v", writes.Load(), result, err)
			}
		})
	}
}
