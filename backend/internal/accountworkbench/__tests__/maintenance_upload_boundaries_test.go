package accountworkbench_test

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestMaintenancePendingUploadWithoutNewDelegationSurvivesRestartWithoutWrites(t *testing.T) {
	f, config, _ := failMaintenanceUpload(t, "rejected")
	f.service = accountworkbench.New(f.private, f.taskBoundary, f.business, nil, f.runBoundary)
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
	if len(pending) != 1 || pending[0].Status != "waiting_session" || writes.Load() != 0 {
		t.Fatal("restart reused old delegation or removed pending credentials")
	}
}

func TestMaintenancePendingUploadChangedBindingRequiresReviewWithoutWrites(t *testing.T) {
	for _, boundary := range []string{"profile", "configuration", "account"} {
		t.Run(boundary, func(t *testing.T) {
			f, config, _ := failMaintenanceUpload(t, "rejected")
			if boundary == "profile" {
				profiles, err := f.service.LoginProfiles(context.Background())
				if err != nil || len(profiles) != 1 {
					t.Fatal(err)
				}
				profile := profiles[0]
				if _, err := f.service.SaveLoginProfile(context.Background(), "owner", accountworkbench.LoginProfileSaveInput{ID: profile.ID, Revision: profile.Revision, AccountID: "101", Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: profile.Email, Password: "changed-profile-private"}}); err != nil {
					t.Fatal(err)
				}
			}
			if boundary == "configuration" {
				config.Model = "changed-model"
				if err := f.private.SaveWorkbenchMaintenance(context.Background(), f.server.URL, config); err != nil {
					t.Fatal(err)
				}
				config.Revision++
			}
			if boundary == "account" {
				f.remote.mu.Lock()
				f.remote.accounts["101"]["priority"] = 999
				f.remote.mu.Unlock()
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
				t.Fatal("changed upload binding was automatically applied")
			}
		})
	}
}

func TestMaintenancePendingUploadCooldownSkipsUploadRefreshAndLogin(t *testing.T) {
	f, config, _ := failMaintenanceUpload(t, "rejected")
	pending := maintenanceUploadSummaries(t, f)
	if len(pending) != 1 {
		t.Fatal("missing pending upload")
	}
	record, err := f.private.WorkbenchExecution(context.Background(), pending[0].SourceTaskID)
	if err != nil {
		t.Fatal(err)
	}
	record.Items[0].UploadNextRetryAt = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	if err := f.private.SaveWorkbenchExecution(context.Background(), record); err != nil {
		t.Fatal(err)
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
	pending = maintenanceUploadSummaries(t, f)
	if len(pending) != 1 || pending[0].Status != "cooldown" || writes.Load() != 0 {
		t.Fatal("cooldown was bypassed")
	}
}

func TestMaintenanceAuthorizationResultPersistsBeforeFirstUploadReadFails(t *testing.T) {
	f, config, owner, _ := maintenanceProfileFixture(t, nil)
	var offline atomic.Bool
	f.taskBoundary.onSaved = func(task taskstore.Task) {
		if task.Operation == "account-workbench-oauth-batch" && task.Status == "succeeded" {
			offline.Store(true)
		}
	}
	f.service.UseTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		if offline.Load() {
			return nil, io.ErrUnexpectedEOF
		}
		return http.DefaultTransport.RoundTrip(request)
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
	if len(maintenanceUploadSummaries(t, f)) != 1 {
		t.Fatal("new OAuth credentials were not persisted before upload read")
	}
	restartMaintenanceUploads(t, f, config.Revision)
	continued, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, continued.ID)
	if len(maintenanceUploadSummaries(t, f)) != 0 {
		t.Fatal("restored connectivity did not upload saved OAuth result")
	}
}

func TestMaintenancePendingUploadExpiryRemovesPrivatePayload(t *testing.T) {
	f, _, _ := failMaintenanceUpload(t, "rejected")
	pending := maintenanceUploadSummaries(t, f)
	if len(pending) != 1 {
		t.Fatal("missing pending upload")
	}
	f.private.UseWorkbenchExecutionClock(func() time.Time { return time.Now().Add(25 * time.Hour) })
	if len(maintenanceUploadSummaries(t, f)) != 0 {
		t.Fatal("expired private upload was still exposed")
	}
	if _, err := f.private.WorkbenchExecution(context.Background(), pending[0].SourceTaskID); err != configstore.ErrWorkbenchExecution {
		t.Fatal("expired upload payload remained available")
	}
}
