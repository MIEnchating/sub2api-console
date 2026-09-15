package accountworkbench_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type maintenanceUploadSummary struct {
	ID           string `json:"id"`
	AccountID    string `json:"account_id"`
	Status       string `json:"status"`
	SourceTaskID string `json:"source_task_id"`
}

func maintenanceUploadSummaries(t *testing.T, f *batchFixture) []maintenanceUploadSummary {
	t.Helper()
	view, err := f.service.Maintenance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"rt_profile_private", "private-saved-password", "test-admin-key"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("pending upload summary exposed private credentials")
		}
	}
	var response struct {
		PendingUploads []maintenanceUploadSummary `json:"pending_uploads"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	return response.PendingUploads
}

func failMaintenanceUpload(t *testing.T, outcome string) (*batchFixture, configstore.WorkbenchMaintenance, *atomic.Int32) {
	t.Helper()
	f, config, owner, _ := maintenanceProfileFixture(t, nil)
	calls := &atomic.Int32{}
	f.service.UseTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/admin/accounts/101" {
			return http.DefaultTransport.RoundTrip(request)
		}
		calls.Add(1)
		if outcome == "rejected" {
			return &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":400,"message":"isolated temporary credential rejection"}`))}, nil
		}
		if outcome == "applied" {
			response, err := http.DefaultTransport.RoundTrip(request)
			if err != nil {
				return nil, err
			}
			_ = response.Body.Close()
		}
		return nil, io.ErrUnexpectedEOF
	}))
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth(owner, child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	if calls.Load() != 1 {
		t.Fatal("first credential update was replayed")
	}
	return f, config, calls
}

func restartMaintenanceUploads(t *testing.T, f *batchFixture, revision int64) {
	t.Helper()
	f.service = accountworkbench.New(f.private, f.taskBoundary, f.business, nil, f.runBoundary)
	bindMaintenance(t, f, revision)
}

func TestMaintenancePendingUploadAfterExplicitRejectionSurvivesRestartWithoutAnotherLogin(t *testing.T) {
	f, config, _ := failMaintenanceUpload(t, "rejected")
	before := maintenanceUploadSummaries(t, f)
	if len(before) != 1 || before[0].AccountID != "101" || before[0].SourceTaskID == "" {
		t.Fatal("failed maintenance upload lost its private retry source")
	}
	restartMaintenanceUploads(t, f, config.Revision)
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	if pending := maintenanceUploadSummaries(t, f); len(pending) != 0 {
		t.Fatal("confirmed upload remained pending")
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.accounts["101"]["credentials"].(map[string]any)["refresh_token"] != "rt_profile_private" || f.remote.refreshes != 0 {
		t.Fatal("pending credentials were lost or ordinary refresh ran first")
	}
	f.browsers.mu.Lock()
	defer f.browsers.mu.Unlock()
	if f.browsers.opened != 1 {
		t.Fatal("retry opened another authorization browser")
	}
}

func TestMaintenancePendingUnknownWriteRemainsForReviewWithoutAutomaticReplay(t *testing.T) {
	f, config, _ := failMaintenanceUpload(t, "unknown")
	if len(maintenanceUploadSummaries(t, f)) != 1 {
		t.Fatal("unknown maintenance write lost its retry source")
	}
	restartMaintenanceUploads(t, f, config.Revision)
	var writes atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			writes.Add(1)
		}
		return http.DefaultTransport.RoundTrip(request)
	}))
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	pending := maintenanceUploadSummaries(t, f)
	if len(pending) != 1 || pending[0].Status != "review" || writes.Load() != 0 {
		t.Fatal("unconfirmed credential write was replayed or dropped")
	}
}

func TestMaintenancePendingLostResponseReconcilesAppliedCredentialsWithoutAnotherPUT(t *testing.T) {
	f, config, _ := failMaintenanceUpload(t, "applied")
	if len(maintenanceUploadSummaries(t, f)) != 1 {
		t.Fatal("lost upload response was not retained for readback")
	}
	restartMaintenanceUploads(t, f, config.Revision)
	var puts atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPut {
			puts.Add(1)
		}
		return http.DefaultTransport.RoundTrip(request)
	}))
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	if len(maintenanceUploadSummaries(t, f)) != 0 || puts.Load() != 0 {
		t.Fatal("readback did not finish applied credentials without another PUT")
	}
}
