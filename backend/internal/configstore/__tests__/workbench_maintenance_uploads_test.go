package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestMaintenanceUploadExecutionReopenPreservesPrivatePayloadAndFiltersManagementTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, target := range []string{"target-a", "target-b"} {
		record := configstore.WorkbenchExecution{ID: "upload-" + target, TargetURL: "https://isolated.invalid", TargetFingerprint: target, Maintenance: &configstore.WorkbenchExecutionMaintenance{Revision: 7, Model: "test-model", CooldownMinutes: 10}, Items: []configstore.WorkbenchExecutionItem{{Index: 0, OriginalAccountID: "101", LoginProfileID: "profile-101", LoginProfileRevision: 4, UploadStatus: "pending", CredentialWriteOutcome: "rejected", Credentials: json.RawMessage(`{"access_token":"private-pending-token"}`)}}}
		if err := store.SaveWorkbenchExecution(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	values, err := store.WorkbenchMaintenanceExecutions(ctx, "target-a")
	if err != nil || len(values) != 1 || values[0].ID != "upload-target-a" {
		t.Fatal("reopened uploads crossed management target", err)
	}
	record := values[0]
	if record.Maintenance.Revision != 7 || record.Items[0].LoginProfileRevision != 4 || string(record.Items[0].Credentials) != `{"access_token":"private-pending-token"}` || record.Items[0].CredentialWriteOutcome != "rejected" {
		t.Fatal("private upload binding or payload was not preserved")
	}
	if err := store.SaveWorkbenchExecution(ctx, record); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveWorkbenchExecution(ctx, record); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatal("stale upload state overwrote a newer revision", err)
	}
}
