package accountworkbench_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"net/http"
	"testing"
)

type excludedWorkbenchAccounts struct{}

func (excludedWorkbenchAccounts) AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error) {
	return business.AccountMutationProtection{UpstreamExcluded: true}, nil
}

func TestExcludedAccountsAreAbsentFromMaintenancePreview(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	service.UseAccountGroupProtection(excludedWorkbenchAccounts{})
	service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatal("excluded account mutated")
		}
		return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{map[string]any{"id": "41", "platform": "openai", "type": "oauth", "status": "error", "error_message": "HTTP 401"}}, "total": 1}}), nil
	}))
	preview, err := service.PreviewMaintenance(t.Context(), owner, accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 5, CooldownMinutes: 10, GroupIDs: []string{}})
	if err != nil || len(preview.Accounts) != 0 {
		t.Fatalf("excluded preview: %+v %v", preview, err)
	}
}

func TestMaintenanceExclusionAfterConfigurationDoesNotReadAccountOrFail(t *testing.T) {
	service, store := fixture(t, `{"data":{"items":[{"id":41,"platform":"openai","type":"oauth","status":"active"}],"total":1}}`)
	owner, _ := previewOwner(t, store)
	runner, tasks := runTasks(t, service)
	preview, err := service.PreviewMaintenance(t.Context(), owner, accountworkbench.MaintenanceSettings{Enabled: true, IntervalMinutes: 5, CooldownMinutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	configured, err := service.ConfigureMaintenance(t.Context(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	service.UseAccountGroupProtection(excludedWorkbenchAccounts{})
	service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts" {
			t.Errorf("excluded account accessed: %s %s", r.Method, r.URL.Path)
		}
		return upstreamResponse(map[string]any{"data": map[string]any{"items": []any{map[string]any{"id": "41", "platform": "openai", "type": "oauth", "status": "error", "error_message": "HTTP 401"}}, "total": 1}}), nil
	}))
	queued, err := service.CheckMaintenance(t.Context(), owner, configured.Revision)
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	task, err := tasks.Get(t.Context(), queued.TaskID)
	if err != nil || task.Status != "succeeded" {
		t.Fatalf("excluded maintenance: %+v %v", task, err)
	}
}
