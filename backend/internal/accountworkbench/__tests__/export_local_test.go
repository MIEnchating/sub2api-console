package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func unconfiguredWorkbenchStore(t *testing.T) *configstore.Store {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.TargetSettings(context.Background()); err == nil {
		t.Fatal("local-export fixture unexpectedly configured a management target")
	}
	return store
}

func localExportFixture(t *testing.T) (*importFixture, string, *atomic.Int32) {
	t.Helper()
	f := newImportFixture(t, nil)
	if err := f.private.Close(); err != nil {
		t.Fatal(err)
	}
	f.private = unconfiguredWorkbenchStore(t)
	f.service = accountworkbench.New(f.private, f.tasks, f.business, nil, f.runner)
	directory := filepath.Join(t.TempDir(), "exports")
	if err := f.service.UseExportDirectory(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	requests := &atomic.Int32{}
	f.service.UseTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("management request forbidden in isolated local export")
	}))
	return f, directory, requests
}

func TestLocalExportWithoutManagementCreatesPrivateFileAndNeverPermitsImport(t *testing.T) {
	f, directory, requests := localExportFixture(t)
	view, err := f.service.Preview(context.Background(), "local-owner", accountworkbench.PreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: importedJSON})
	if err != nil || view.ID == "" || view.Target != "" || view.Scope != accountworkbench.ScopeLocalExport || !view.ExportOnly || len(view.Items) != 1 || len(view.Errors) != 0 {
		t.Fatalf("local preview = %+v, %v", view, err)
	}
	if _, err := f.service.Import(context.Background(), "local-owner", view.ID, true); err == nil {
		t.Fatal("local export preview was accepted by managed import")
	}
	if _, err := f.service.ExportInput(context.Background(), "other-owner", view.ID, true); err == nil {
		t.Fatal("another session consumed local credentials")
	}
	if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, false); err == nil {
		t.Fatal("local export accepted missing confirmation")
	}
	if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status != "succeeded" {
		t.Fatalf("local conversion task = %+v", task)
	}
	if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, true); err == nil {
		t.Fatal("local conversion preview could be replayed")
	}
	files, err := f.service.LocalExports(context.Background(), "local-owner")
	if err != nil || len(files) != 1 || files[0].Count != 1 {
		t.Fatalf("local artifacts = %+v, %v", files, err)
	}
	raw, err := os.ReadFile(filepath.Join(directory, files[0].ID+".json"))
	if err != nil || !strings.Contains(string(raw), `"refresh_token":"rt_new_private"`) {
		t.Fatalf("private credential artifact unavailable: %v", err)
	}
	public, _ := json.Marshal([]any{view, files, task})
	if strings.Contains(string(public), "rt_new_private") || strings.Contains(string(public), "access-new-private") {
		t.Fatal("local export exposed credentials in public metadata")
	}
	if requests.Load() != 0 {
		t.Fatalf("local export attempted %d management requests", requests.Load())
	}
}

func TestLocalExportAfterTargetConfigurationRemainsSeparateAndOwnerBound(t *testing.T) {
	f, _, _ := localExportFixture(t)
	view, err := f.service.Preview(context.Background(), "local-owner", accountworkbench.PreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: importedJSON})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ExportInput(context.Background(), "local-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	if task := f.await(t); task.Status != "succeeded" {
		t.Fatal(task.Message)
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "test-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	local, err := f.service.LocalExports(context.Background(), "local-owner")
	if err != nil || len(local) != 1 {
		t.Fatalf("configured local namespace = %+v, %v", local, err)
	}
	managed, err := f.service.Exports(context.Background(), "local-owner")
	if err != nil || len(managed) != 0 {
		t.Fatalf("managed list exposed local artifacts = %+v, %v", managed, err)
	}
	other, err := f.service.LocalExports(context.Background(), "other-owner")
	if err != nil || len(other) != 0 {
		t.Fatal("another session listed local private artifacts")
	}
	if err := f.service.DeleteExport(context.Background(), "local-owner", local[0].ID); err == nil {
		t.Fatal("managed delete removed a local artifact")
	}
	if err := f.service.DeleteLocalExport(context.Background(), "other-owner", local[0].ID); err == nil {
		t.Fatal("another session deleted local private artifact")
	}
	if err := f.service.DeleteLocalExport(context.Background(), "local-owner", local[0].ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := f.service.LocalExports(context.Background(), "local-owner")
	if err != nil || len(remaining) != 0 {
		t.Fatal("confirmed local deletion left artifact metadata")
	}
}

func TestLocalExportRejectsManagedOptionsAndPreviewsRefreshOnlyWithoutRemoteRequests(t *testing.T) {
	f, directory, requests := localExportFixture(t)
	for _, input := range []accountworkbench.PreviewInput{
		{Scope: "unsupported", ExportOnly: true, Content: importedJSON},
		{Scope: accountworkbench.ScopeLocalExport, Content: importedJSON},
		{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: importedJSON, TemplateID: "template"},
		{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: importedJSON, CheckAfterImport: true},
		{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: `[` + importedJSON + `,` + importedJSON + `]`},
	} {
		view, err := f.service.Preview(context.Background(), "local-owner", input)
		if err == nil || view.ID != "" {
			t.Fatalf("invalid local input retained executable preview: %+v, %v", view, err)
		}
	}
	view, err := f.service.Preview(context.Background(), "local-owner", accountworkbench.PreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, Content: `["rt_local_private",` + importedJSON + `]`})
	if err != nil || len(view.Errors) != 0 || view.ID == "" || len(view.Items) != 2 || !view.Items[0].RefreshRequired || view.Items[1].RefreshRequired {
		t.Fatalf("refresh-only local input = %+v, %v", view, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 || requests.Load() != 0 {
		t.Fatal("rejected local input made a request or wrote private artifacts")
	}
}
