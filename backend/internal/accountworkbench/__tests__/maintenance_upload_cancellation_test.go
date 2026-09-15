package accountworkbench_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestMaintenancePendingUploadPreservesVerifiedResultWhenConfigurationChangesBeforePrivateSave(t *testing.T) {
	f, config, owner, _ := maintenanceProfileFixture(t, nil)
	saved := make(chan error, 1)
	f.taskBoundary.onSaved = func(task taskstore.Task) {
		if task.Operation == "account-workbench-oauth" && task.Status == "succeeded" {
			changed := config
			changed.Model = "changed-after-authorization"
			saved <- f.private.SaveWorkbenchMaintenance(context.Background(), f.server.URL, changed)
		}
	}
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
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth(owner, child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	pending := maintenanceUploadSummaries(t, f)
	if len(pending) != 1 || pending[0].AccountID != "101" || writes.Load() != 0 {
		t.Fatal("configuration change discarded a completed authorization or authorized an upload")
	}
	restartMaintenanceUploads(t, f, config.Revision+1)
	continued, err := f.service.CheckMaintenance(context.Background(), config.Revision+1, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, continued.ID)
	if pending := maintenanceUploadSummaries(t, f); len(pending) != 1 || pending[0].Status != "review" {
		t.Fatal("old authorization configuration did not remain pending for review")
	}
}
