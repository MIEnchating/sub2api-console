package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func reloadExportService(t *testing.T, f *importFixture, directory string) error {
	t.Helper()
	if err := f.service.CloseExports(); err != nil {
		t.Fatal(err)
	}
	f.service = accountworkbench.New(f.private, f.tasks, f.business, nil, f.runner)
	return f.service.UseExportDirectory(directory)
}

func TestPrivateExportSurvivesRestartAndRemainsBoundToOwnerAndTarget(t *testing.T) {
	f, directory := exportFixture(t)
	metadata := completeExport(t, f, previewExport(t, f, "101"))
	if err := reloadExportService(t, f, directory); err != nil {
		t.Fatal(err)
	}
	list, err := f.service.Exports(context.Background(), "export-owner")
	if err != nil || len(list) != 1 || list[0] != metadata {
		t.Fatalf("restored metadata = %#v, %v", list, err)
	}
	list, err = f.service.Exports(context.Background(), "other-owner")
	if err != nil || len(list) != 0 {
		t.Fatal("another session could enumerate private artifacts")
	}
	if err := f.service.DeleteExport(context.Background(), "other-owner", metadata.ID); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("another session delete = %v", err)
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "different-target-key", 3); err != nil {
		t.Fatal(err)
	}
	list, err = f.service.Exports(context.Background(), "export-owner")
	if err != nil || len(list) != 0 {
		t.Fatal("changed target could enumerate previous artifacts")
	}
	if err := f.service.DeleteExport(context.Background(), "export-owner", metadata.ID); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("changed target delete = %v", err)
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "test-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteExport(context.Background(), "export-owner", metadata.ID); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("deleted artifact retained private payload or metadata")
	}
}

func TestPrivateExportRestartRemovesExpiredArtifactAndOrphanPayload(t *testing.T) {
	f, directory := exportFixture(t)
	metadata := completeExport(t, f, previewExport(t, f, "101"))
	if err := f.service.CloseExports(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, metadata.ID+".meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sidecar map[string]any
	if err := json.Unmarshal(data, &sidecar); err != nil {
		t.Fatal(err)
	}
	privateMetadata := sidecar["metadata"].(map[string]any)
	old := time.Now().UTC().Add(-25 * time.Hour)
	privateMetadata["created_at"], privateMetadata["expires_at"] = old.Format(time.RFC3339Nano), old.Add(24*time.Hour).Format(time.RFC3339Nano)
	data, err = json.Marshal(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	orphanPath := filepath.Join(directory, strings.Repeat("a", 32)+".json")
	if err := os.WriteFile(orphanPath, []byte(`{"interrupted":"private"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(orphanPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := reloadExportService(t, f, directory); err != nil {
		t.Fatal(err)
	}
	list, err := f.service.Exports(context.Background(), "export-owner")
	if err != nil || len(list) != 0 {
		t.Fatalf("expired list = %#v, %v", list, err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("expired or interrupted private files survived restart cleanup")
	}
}

func TestPrivateExportRejectsSymlinkDirectoryComponentsAndPublicPermissions(t *testing.T) {
	for _, mode := range []string{"direct-link", "ancestor-link", "public-directory", "relative-path"} {
		t.Run(mode, func(t *testing.T) {
			f := newImportFixture(t, nil)
			base := t.TempDir()
			actual := filepath.Join(base, "actual")
			if err := os.Mkdir(actual, 0700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(base, "linked")
			if err := os.Symlink(actual, link); err != nil {
				t.Fatal(err)
			}
			path := link
			switch mode {
			case "ancestor-link":
				path = filepath.Join(link, "exports")
			case "public-directory":
				path = actual
				if err := os.Chmod(actual, 0755); err != nil {
					t.Fatal(err)
				}
			case "relative-path":
				path = "relative-exports"
			}
			if err := f.service.UseExportDirectory(path); !errors.Is(err, accountworkbench.ErrExportUnavailable) {
				t.Fatalf("unsafe directory = %v", err)
			}
			entries, _ := os.ReadDir(actual)
			if len(entries) != 0 {
				t.Fatal("unsafe directory validation created files through a symlink")
			}
		})
	}
}

func TestPrivateExportRejectsLinkedPayloadOnRestartWithoutFollowingIt(t *testing.T) {
	f, directory := exportFixture(t)
	metadata := completeExport(t, f, previewExport(t, f, "101"))
	if err := f.service.CloseExports(); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("outside-sensitive-file"), 0600); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(directory, metadata.ID+".json")
	if err := os.Remove(payload); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, payload); err != nil {
		t.Fatal(err)
	}
	if err := reloadExportService(t, f, directory); !errors.Is(err, accountworkbench.ErrExportUnavailable) {
		t.Fatalf("linked artifact loaded = %v", err)
	}
	content, err := os.ReadFile(outside)
	if err != nil || string(content) != "outside-sensitive-file" {
		t.Fatal("restart followed or modified a linked payload")
	}
}

func TestPrivateExportOpenedDirectoryRemainsConfinedAfterPathReplacement(t *testing.T) {
	f, directory := exportFixture(t)
	view := previewExport(t, f, "101")
	moved := directory + "-original"
	if err := os.Rename(directory, moved); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, directory); err != nil {
		t.Fatal(err)
	}
	metadata := completeExport(t, f, view)
	if _, err := os.Stat(filepath.Join(moved, metadata.ID+".json")); err != nil {
		t.Fatal("export escaped its opened private directory")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("export followed a replaced directory path")
	}
	if err := f.service.DeleteExport(context.Background(), "export-owner", "../outside.json"); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("path traversal delete = %v", err)
	}
}

func TestCancelledExportDoesNotWriteCredentialArtifact(t *testing.T) {
	f, directory := exportFixture(t)
	var reading atomic.Bool
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reading.Load() {
			close(started)
			<-r.Context().Done()
			return
		}
		f.remote.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	if err := f.private.ConfigureTarget(context.Background(), server.URL, "test-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	view := previewExport(t, f, "101")
	reading.Store(true)
	task, err := f.service.Export(context.Background(), "export-owner", view.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("isolated export did not start readback")
	}
	if !f.runner.CancelTask(task.ID) {
		t.Fatal("export task was not registered for cancellation")
	}
	if task := f.await(t); task.Status != "cancelled" {
		t.Fatalf("cancelled export task = %s", task.Status)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("cancelled export wrote a credential artifact")
	}
}
