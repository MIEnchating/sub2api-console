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
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func exportAccount(id, name string) map[string]any {
	return map[string]any{
		"id": json.Number(id), "name": name, "platform": "openai", "type": "oauth",
		"credentials": map[string]any{"access_token": "export-access-private", "refresh_token": "rt_export_private", "email": "owner@example.com", "chatgpt_account_id": "workspace-" + id, "chatgpt_user_id": "user-" + id},
		"concurrency": json.Number("3"), "priority": json.Number("10"), "rate_multiplier": json.Number("0.123456789012345678901"),
		"group_ids": []any{json.Number("7")}, "last_used_at": "2026-09-14T00:00:00Z", "total_requests": json.Number("1"),
	}
}

func exportFixture(t *testing.T) (*importFixture, string) {
	t.Helper()
	f := newImportFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "owner@example.com")
	f.remote.accounts["102"] = exportAccount("102", "Second account")
	directory := filepath.Join(t.TempDir(), "exports")
	if err := f.service.UseExportDirectory(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	return f, directory
}

func previewExport(t *testing.T, f *importFixture, ids ...string) accountworkbench.ExportPreview {
	t.Helper()
	view, err := f.service.PreviewExport(context.Background(), "export-owner", accountworkbench.ExportPreviewInput{AccountIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func completeExport(t *testing.T, f *importFixture, view accountworkbench.ExportPreview) accountworkbench.ExportMetadata {
	t.Helper()
	if _, err := f.service.Export(context.Background(), "export-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status != "succeeded" {
		t.Fatalf("export task = %s: %s", task.Status, task.Message)
	}
	artifacts, err := f.service.Exports(context.Background(), "export-owner")
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("export artifacts = %#v, %v", artifacts, err)
	}
	return artifacts[0]
}

func TestConfirmedExportWritesReferenceFormatWithExactDecimalsAndOnlyPublicMetadata(t *testing.T) {
	f, directory := exportFixture(t)
	f.remote.accounts["102"]["name"] = "export-access-private rt_export_private test-admin-key"
	view := previewExport(t, f, "101", "102")
	if view.Items[0].Name != "owner@example.com" || len(view.Items) != 2 || view.Revision == "" || view.Items[0].Revision == "" {
		t.Fatalf("export preview = %#v", view)
	}
	metadata := completeExport(t, f, view)
	if metadata.Count != 2 {
		t.Fatalf("count = %d", metadata.Count)
	}
	payloadPath := filepath.Join(directory, metadata.ID+".json")
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Type       string           `json:"type"`
		Version    int              `json:"version"`
		ExportedAt string           `json:"exported_at"`
		Proxies    []any            `json:"proxies"`
		Accounts   []map[string]any `json:"accounts"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.Type != "sub2api-data" || data.Version != 1 || data.ExportedAt != metadata.CreatedAt || data.Proxies == nil || len(data.Proxies) != 0 || len(data.Accounts) != 2 {
		t.Fatal("private file did not use the reference Sub2API envelope")
	}
	if data.Accounts[0]["rate_multiplier"] != json.Number("0.123456789012345678901") {
		t.Fatal("export changed source decimal precision")
	}
	credentials, ok := data.Accounts[0]["credentials"].(map[string]any)
	if !ok || credentials["access_token"] != "export-access-private" || credentials["refresh_token"] != "rt_export_private" {
		t.Fatal("private export omitted OAuth credentials")
	}
	if _, exists := data.Accounts[0]["total_requests"]; exists {
		t.Fatal("export included runtime state absent from the reference transfer format")
	}
	for _, path := range []string{payloadPath, filepath.Join(directory, metadata.ID+".meta.json")} {
		stat, err := os.Stat(path)
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatalf("private file permissions = %v, %v", stat, err)
		}
	}
	stat, err := os.Stat(directory)
	if err != nil || stat.Mode().Perm() != 0700 {
		t.Fatalf("private directory permissions = %v, %v", stat, err)
	}
	history, err := f.service.History(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := os.ReadFile(filepath.Join(directory, metadata.ID+".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal([]any{view, metadata, history, string(sidecar)})
	for _, secret := range []string{"export-access-private", "rt_export_private", "test-admin-key", "export-owner"} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("public metadata or sidecar leaked %s", secret)
		}
	}
}

func TestExportRequiresConfirmationAndMatchingOwnerWithoutConsumingTheirPreview(t *testing.T) {
	f, _ := exportFixture(t)
	view := previewExport(t, f, "101")
	if _, err := f.service.Export(context.Background(), "export-owner", view.ID, false); err == nil {
		t.Fatal("unconfirmed export created a task")
	}
	if _, err := f.service.Export(context.Background(), "another-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("wrong owner = %v", err)
	}
	completeExport(t, f, view)
	if _, err := f.service.Export(context.Background(), "export-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("consumed preview reused = %v", err)
	}
}

func TestExportRejectsChangedTargetBeforeEnqueueAndLeavesNoFile(t *testing.T) {
	f, directory := exportFixture(t)
	view := previewExport(t, f, "101")
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "replacement-test-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Export(context.Background(), "export-owner", view.ID, true); !errors.Is(err, targetguard.ErrChanged) {
		t.Fatalf("changed target = %v", err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("changed target wrote a private file")
	}
}

func TestExportRereadsCredentialsAndRejectsChangedAccount(t *testing.T) {
	f, directory := exportFixture(t)
	view := previewExport(t, f, "101", "102")
	f.remote.mu.Lock()
	f.remote.accounts["102"]["credentials"].(map[string]any)["refresh_token"] = "rt_rotated_private"
	f.remote.mu.Unlock()
	if _, err := f.service.Export(context.Background(), "export-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	if task := f.await(t); task.Status != "failed" {
		t.Fatalf("changed credentials task = %s", task.Status)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("account change exported a partial batch")
	}
}

func TestExportIgnoresUnrelatedRuntimeCountersWhilePreservingTransferConfiguration(t *testing.T) {
	f, _ := exportFixture(t)
	view := previewExport(t, f, "101")
	f.remote.mu.Lock()
	f.remote.accounts["101"]["total_requests"] = json.Number("20")
	f.remote.accounts["101"]["last_used_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	f.remote.mu.Unlock()
	completeExport(t, f, view)
}

func TestExportRejectsTargetChangeDuringRemoteReadback(t *testing.T) {
	f, directory := exportFixture(t)
	var readback atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if readback.Load() {
			if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-during-readback", 3); err != nil {
				t.Error(err)
			}
		}
		f.remote.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	if err := f.private.ConfigureTarget(context.Background(), server.URL, "test-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	view := previewExport(t, f, "101")
	readback.Store(true)
	if _, err := f.service.Export(context.Background(), "export-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	if task := f.await(t); task.Status != "failed" {
		t.Fatalf("mid-readback target change = %s", task.Status)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("target change during readback wrote a credential file")
	}
}

func TestExportPreviewRejectsInvalidIDsAndUnsupportedAccountTypes(t *testing.T) {
	f, _ := exportFixture(t)
	for _, ids := range [][]string{nil, {"0"}, {"-1"}, {"01"}, {"101", "101"}, {"../101"}, {"9223372036854775808"}, make([]string, 501)} {
		if _, err := f.service.PreviewExport(context.Background(), "owner", accountworkbench.ExportPreviewInput{AccountIDs: ids}); err == nil {
			t.Fatalf("invalid IDs accepted: %v", ids)
		}
	}
	f.remote.accounts["101"]["type"] = "apikey"
	if _, err := f.service.PreviewExport(context.Background(), "owner", accountworkbench.ExportPreviewInput{AccountIDs: []string{"101"}}); err == nil {
		t.Fatal("non-OAuth account was exportable")
	}
	f.remote.accounts["101"]["type"] = "oauth"
	f.remote.accounts["101"]["id"] = json.Number("102")
	if _, err := f.service.PreviewExport(context.Background(), "owner", accountworkbench.ExportPreviewInput{AccountIDs: []string{"101"}}); err == nil {
		t.Fatal("upstream mismatched ID was exportable")
	}
}

func TestDiscardedExportPreviewCannotCreateTask(t *testing.T) {
	f, _ := exportFixture(t)
	view := previewExport(t, f, "101")
	f.service.DeleteExportPreview("export-owner", view.ID)
	if _, err := f.service.Export(context.Background(), "export-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("discarded preview = %v", err)
	}
}
