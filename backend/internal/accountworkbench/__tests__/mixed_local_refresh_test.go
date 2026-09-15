package accountworkbench_test

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestMixedLocalRefreshKeepsRTPrivateUntilFinalConversionConfirmation(t *testing.T) {
	f := newBatchFixture(t, nil)
	if err := f.private.Close(); err != nil {
		t.Fatal(err)
	}
	f.private = unconfiguredWorkbenchStore(t)
	f.service = accountworkbench.New(f.private, f.taskBoundary, f.business, nil, f.runBoundary)
	if err := f.service.UseExportDirectory(filepath.Join(t.TempDir(), "exports")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	var management, refreshes atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		management.Add(1)
		return nil, errors.New("management forbidden")
	}))
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		refreshes.Add(1)
		return localRefreshResponse("user-rt", "workspace-rt"), nil
	}))
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, RecoveryEnabled: true, Content: mixedJSON + "\nrt_mixed_private"})
	if err != nil || preview.ID == "" || len(preview.Items) != 2 || refreshes.Load() != 0 {
		t.Fatalf("local mixed RT preview = %+v, %v", preview, err)
	}
	view, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	prepared, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", view.ID)
	if err != nil || len(prepared.Items) != 2 || prepared.Items[0].RefreshRequired || !prepared.Items[1].RefreshRequired || refreshes.Load() != 0 || management.Load() != 0 {
		t.Fatalf("local mixed result before confirmation = %+v, %v", prepared, err)
	}
	if _, err := f.service.ExportInput(context.Background(), "owner", prepared.ID, false); err == nil || refreshes.Load() != 0 {
		t.Fatal("mixed local conversion refreshed before final confirmation")
	}
	task, err := f.service.ExportInput(context.Background(), "owner", prepared.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	result, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil || result.Status != "succeeded" || refreshes.Load() != 1 || management.Load() != 0 {
		t.Fatalf("confirmed mixed local conversion = %+v, %v", result, err)
	}
	files, err := f.service.LocalExports(context.Background(), "owner")
	if err != nil || len(files) != 2 {
		t.Fatalf("confirmed mixed local files = %+v, %v", files, err)
	}
}
