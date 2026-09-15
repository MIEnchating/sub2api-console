package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func cleanupInput(profile accountworkbench.LoginProfileView) accountworkbench.CleanupPreviewInput {
	return accountworkbench.CleanupPreviewInput{Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}}}
}

func cleanupHistoryTask(t *testing.T, f *batchFixture, id, status string, rows any) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := f.tasks.Store.Save(context.Background(), taskstore.Task{ID: id, Skill: accountworkbench.Skill, Operation: "account-workbench-oauth-batch", Status: status, Message: "Isolated cleanup fixture", CreatedAt: now, UpdatedAt: now, Result: map[string]any{"items": rows}}); err != nil {
		t.Fatal(err)
	}
}

func cleanupProfileRow(profile accountworkbench.LoginProfileView) accountworkbench.OAuthBatchRow {
	return accountworkbench.OAuthBatchRow{AccountID: profile.AccountID, ProfileID: profile.ID, ProfileRevision: profile.Revision, UserID: profile.UserID, WorkspaceID: profile.WorkspaceID, Email: profile.Email, Status: "succeeded"}
}

func createCleanupExport(t *testing.T, f *batchFixture, ids []string) accountworkbench.ExportMetadata {
	t.Helper()
	preview, err := f.service.PreviewExport(context.Background(), "owner", accountworkbench.ExportPreviewInput{AccountIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.service.Export(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	stored, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil || stored.Status != "succeeded" {
		t.Fatalf("cleanup fixture export = %s %s %v", stored.Status, stored.Message, err)
	}
	artifacts, err := f.service.Exports(context.Background(), "owner")
	if err != nil || len(artifacts) == 0 {
		t.Fatal(err)
	}
	return artifacts[0]
}

func createCleanupExecution(t *testing.T, f *batchFixture, id string, accountIDs []string) {
	t.Helper()
	raw, _ := json.Marshal([]string{f.server.URL, "test-admin-key"})
	fingerprint := sha256.Sum256(raw)
	items := []configstore.WorkbenchExecutionItem{}
	for i, accountID := range accountIDs {
		identity, _ := json.Marshal([]string{"workspace-" + accountID, "user-" + accountID})
		items = append(items, configstore.WorkbenchExecutionItem{Index: i, AccountID: accountID, OriginalAccountID: accountID, Identity: "identity:" + string(identity), Phase: "staged", Status: "review", Credentials: json.RawMessage(`{"refresh_token":"rt_private_execution"}`)})
	}
	if err := f.private.SaveWorkbenchExecution(context.Background(), configstore.WorkbenchExecution{ID: id, TargetURL: f.server.URL, TargetFingerprint: hex.EncodeToString(fingerprint[:]), Items: items}); err != nil {
		t.Fatal(err)
	}
}

func TestAccountCleanupDeletesSelectedPrivateDataAndRetainsMixedAccountsWithoutRemoteWrites(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	other := saveProfile(t, f, "102")
	exportsDir := filepath.Join(t.TempDir(), "exports")
	if err := f.service.UseExportDirectory(exportsDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	selectedExport := createCleanupExport(t, f, []string{"101"})
	mixedExport := createCleanupExport(t, f, []string{"101", "102"})
	cleanupHistoryTask(t, f, "selected-history", "succeeded", []accountworkbench.OAuthBatchRow{cleanupProfileRow(profile)})
	cleanupHistoryTask(t, f, "mixed-history", "succeeded", []accountworkbench.OAuthBatchRow{cleanupProfileRow(profile), cleanupProfileRow(other)})
	createCleanupExecution(t, f, "selected-execution", []string{"101"})
	createCleanupExecution(t, f, "mixed-execution", []string{"101", "102"})
	securityDir := filepath.Join(t.TempDir(), "security")
	if err := f.service.UseSecurityDirectory(securityDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseSecurity() })
	securityID := strings.Repeat("a", 32)
	targetJSON, _ := json.Marshal([]string{f.server.URL, "test-admin-key"})
	targetFingerprint := sha256.Sum256(targetJSON)
	securityRaw := `{"target_fingerprint":"` + hex.EncodeToString(targetFingerprint[:]) + `","version":1,"account_id":"101","user_id":"user-101","email":"owner@example.com","operation":"password","password":"private-security-password"}`
	if err := os.WriteFile(filepath.Join(securityDir, securityID+".json"), []byte(securityRaw), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile))
	if err != nil || preview.ID == "" || preview.Blocked {
		t.Fatalf("cleanup preview = %+v %v", preview, err)
	}
	actions := map[string]string{}
	for _, item := range preview.Items {
		actions[item.Kind+":"+item.ID] = item.Action
	}
	for _, key := range []string{"login_profile:" + profile.ID, "execution:selected-execution", "history:selected-history", "account_export:" + selectedExport.ID, "security_result:" + securityID} {
		if actions[key] != "delete" {
			t.Fatal("selected resource missing from delete scope", key)
		}
	}
	for _, key := range []string{"execution:mixed-execution", "history:mixed-history", "account_export:" + mixedExport.ID} {
		if actions[key] != "retain" {
			t.Fatal("mixed-account resource not retained", key)
		}
	}
	raw, _ := json.Marshal(preview)
	for _, secret := range []string{"private-saved-password", "private-security-password", "rt_private_execution", "export-access-private"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("cleanup preview leaked private data")
		}
	}
	task, err := f.service.CleanupAccounts(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	finished, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil || finished.Status != "succeeded" {
		t.Fatalf("cleanup task = %s %s %v", finished.Status, finished.Message, err)
	}
	profiles, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(profiles) != 1 || profiles[0].ID != other.ID {
		t.Fatal("cleanup deleted unrelated login profile", err)
	}
	if _, err := f.tasks.Get(context.Background(), "selected-history"); !errors.Is(err, taskstore.ErrNotFound) {
		t.Fatal("selected history retained", err)
	}
	if _, err := f.tasks.Get(context.Background(), "mixed-history"); err != nil {
		t.Fatal("mixed history deleted", err)
	}
	if _, err := f.private.WorkbenchExecution(context.Background(), "selected-execution"); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatal("selected execution retained", err)
	}
	if _, err := f.private.WorkbenchExecution(context.Background(), "mixed-execution"); err != nil {
		t.Fatal("mixed private execution deleted", err)
	}
	if _, err := os.Stat(filepath.Join(exportsDir, selectedExport.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("selected export remained", err)
	}
	if _, err := os.Stat(filepath.Join(exportsDir, mixedExport.ID+".json")); err != nil {
		t.Fatal("mixed export deleted", err)
	}
	if _, err := os.Stat(filepath.Join(securityDir, securityID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("security result remained", err)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if len(f.remote.accounts) != 2 || f.remote.updates != 0 || f.remote.created != 0 {
		t.Fatal("local cleanup mutated remote accounts")
	}
}
