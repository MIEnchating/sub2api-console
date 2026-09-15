package accountworkbench_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type cleanupFailingStore struct{ *configstore.Store }

func (s *cleanupFailingStore) DeleteWorkbenchExecution(context.Context, string, string, int64) error {
	return errors.New("isolated private store deletion failure")
}

func TestAccountCleanupStorageFailureKeepsProfileAndMarksUnprocessedItemsStopped(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	createCleanupExecution(t, f, "cleanup-storage-failure", []string{"101"})
	f.service = accountworkbench.New(&cleanupFailingStore{Store: f.private}, f.taskBoundary, f.business, nil, f.runBoundary)
	preview, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile))
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.service.CleanupAccounts(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	finished, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil || finished.Status != "failed" {
		t.Fatal("storage failure did not fail cleanup", err)
	}
	for _, value := range finished.Result["items"].([]any) {
		status := value.(map[string]any)["status"]
		if status == "queued" || status == "running" {
			t.Fatal("finished cleanup still shows unprocessed items as active")
		}
	}
	profiles, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(profiles) != 1 {
		t.Fatal("cleanup removed profile despite incomplete private cleanup", err)
	}
}

func TestAccountCleanupRejectsUnconfirmedOrForeignOwnerAndChangedProfileBeforeDeleting(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	preview, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CleanupAccounts(context.Background(), "owner", preview.ID, false); err == nil {
		t.Fatal("unconfirmed cleanup started")
	}
	if _, err := f.service.CleanupAccounts(context.Background(), "other", preview.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatal("foreign owner claimed cleanup", err)
	}
	updated, err := f.service.SaveLoginProfile(context.Background(), "owner", accountworkbench.LoginProfileSaveInput{ID: profile.ID, AccountID: profile.AccountID, Revision: profile.Revision, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: profile.Email, Password: "updated-private-password"}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.service.CleanupAccounts(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	finished, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil || finished.Status != "failed" {
		t.Fatal("stale cleanup did not fail", err)
	}
	profiles, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(profiles) != 1 || profiles[0].Revision != updated.Revision {
		t.Fatal("stale cleanup deleted updated profile", err)
	}
}

func TestAccountCleanupActiveTaskBlocksPreviewAndTaskCreatedAfterPreviewBlocksExecution(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	preview, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile))
	if err != nil {
		t.Fatal(err)
	}
	cleanupHistoryTask(t, f, "active-workbench", "waiting_input", []accountworkbench.OAuthBatchRow{cleanupProfileRow(profile)})
	task, err := f.service.CleanupAccounts(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	profiles, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(profiles) != 1 {
		t.Fatal("cleanup deleted profile while task active", err)
	}
	blocked, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile))
	if err != nil || !blocked.Blocked || len(blocked.Blockers) != 1 || blocked.Blockers[0].TaskID != "active-workbench" {
		t.Fatalf("active cleanup scope = %+v %v", blocked, err)
	}
	if _, err := f.service.CleanupAccounts(context.Background(), "owner", blocked.ID, true); err == nil {
		t.Fatal("blocked preview started cleanup")
	}
}

func TestAccountCleanupArtifactChangedAfterPreviewPreservesAllSelectedData(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	directory := filepath.Join(t.TempDir(), "exports")
	if err := f.service.UseExportDirectory(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	artifact := createCleanupExport(t, f, []string{"101"})
	preview, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, artifact.ID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	task, err := f.service.CleanupAccounts(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	finished, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil || finished.Status != "failed" {
		t.Fatal("changed artifact cleanup did not fail", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("changed artifact deleted", err)
	}
	profiles, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(profiles) != 1 {
		t.Fatal("artifact change deleted profile", err)
	}
}

func TestAccountCleanupRejectsMissingDuplicateAndStaleProfileSelections(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	for _, input := range []accountworkbench.CleanupPreviewInput{
		{},
		{Items: []accountworkbench.ProfileExportSelection{{ID: "missing", Revision: 1}}},
		{Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: 2}}},
		{Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: 1}, {ID: profile.ID, Revision: 1}}},
	} {
		if _, err := f.service.PreviewAccountCleanup(context.Background(), "owner", input); err == nil {
			t.Fatal("invalid cleanup selection accepted")
		}
	}
	if _, err := f.service.PreviewAccountCleanup(context.Background(), "owner", accountworkbench.CleanupPreviewInput{Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: 2}}}); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatal("stale selection lost typed error")
	}
}
