package accountworkbench_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestMaintenancePendingUploadPartialBatchRetriesOnlyRejectedAccount(t *testing.T) {
	f, config, owner, _ := maintenanceProfileFixture(t, nil)
	f.remote.accounts["102"]["error_message"] = "invalid_grant"
	saveProfile(t, f, "102")
	f.service.UseTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPut && request.URL.Path == "/api/v1/admin/accounts/101" {
			return &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":400,"message":"isolated upload rejection"}`))}, nil
		}
		return http.DefaultTransport.RoundTrip(request)
	}))
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		child := f.awaitTask(t, "account-workbench-oauth", "waiting")
		current, err := f.service.MaintenanceAuthorization(context.Background(), owner)
		if err != nil {
			t.Fatal(err)
		}
		batch, err := f.service.ReadOAuthBatch(owner, current.CurrentReauthorizationID)
		if err != nil {
			t.Fatal(err)
		}
		accountID := ""
		for _, row := range batch.Items {
			if row.Status == "running" {
				accountID = row.AccountID
			}
		}
		if accountID == "" {
			t.Fatal("running authorization has no stable account")
		}
		profileExchange(f, "user-"+accountID, "workspace-"+accountID)
		if err := f.service.FinishOAuth(owner, child.ID); err != nil {
			t.Fatal(err)
		}
	}
	f.awaitDone(t, task.ID)
	pending := maintenanceUploadSummaries(t, f)
	if len(pending) != 1 || pending[0].AccountID != "101" {
		t.Fatal("partial upload retained a succeeded account or lost the rejected account")
	}
	restartMaintenanceUploads(t, f, config.Revision)
	puts := []string{}
	f.service.UseTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPut {
			puts = append(puts, request.URL.Path)
		}
		return http.DefaultTransport.RoundTrip(request)
	}))
	// The successful peer is no longer a maintenance candidate.
	f.remote.mu.Lock()
	delete(f.remote.accounts["102"], "error_message")
	f.remote.mu.Unlock()
	continued, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, continued.ID)
	if len(puts) != 1 || puts[0] != "/api/v1/admin/accounts/101" {
		t.Fatal("retry did not limit writes to the rejected stable account")
	}
	if len(maintenanceUploadSummaries(t, f)) != 0 {
		t.Fatal("completed partial batch still has pending uploads")
	}
}

func TestMaintenancePendingUploadDisabledConfigurationPreservesPrivateResult(t *testing.T) {
	f, config, _ := failMaintenanceUpload(t, "rejected")
	config.Enabled = false
	view, err := f.service.SaveMaintenance(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	f.service = accountworkbench.New(f.private, f.taskBoundary, f.business, nil, f.runBoundary)
	task, err := f.service.CheckMaintenance(context.Background(), view.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	pending := maintenanceUploadSummaries(t, f)
	if len(pending) != 1 || pending[0].Status != "waiting_session" {
		t.Fatal("disabled maintenance discarded or uploaded the private result")
	}
}

func TestMaintenancePendingSuccessfulFirstAuthorizationSurvivesCancellationOfLaterAccount(t *testing.T) {
	f, config, owner, _ := maintenanceProfileFixture(t, nil)
	f.remote.accounts["102"]["error_message"] = "invalid_grant"
	saveProfile(t, f, "102")
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	first := f.awaitTask(t, "account-workbench-oauth", "waiting")
	authorization, err := f.service.MaintenanceAuthorization(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := f.service.ReadOAuthBatch(owner, authorization.CurrentReauthorizationID)
	if err != nil {
		t.Fatal(err)
	}
	accountID := ""
	for _, item := range batch.Items {
		if item.Status == "running" {
			accountID = item.AccountID
		}
	}
	if accountID == "" {
		t.Fatal("active authorization has no account binding")
	}
	profileExchange(f, "user-"+accountID, "workspace-"+accountID)
	if err := f.service.FinishOAuth(owner, first.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	if pending := maintenanceUploadSummaries(t, f); len(pending) != 1 || pending[0].SourceTaskID != first.ID {
		t.Fatal("successful authorization remained only in batch memory")
	}
	if !f.runBoundary.CancelTask(task.ID) {
		t.Fatal("active maintenance task could not be cancelled")
	}
	f.awaitDone(t, task.ID)
	if pending := maintenanceUploadSummaries(t, f); len(pending) != 1 || pending[0].AccountID != accountID {
		t.Fatal("cancelling later authorization discarded an earlier successful result")
	}
}
