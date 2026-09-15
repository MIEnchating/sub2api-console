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
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func TestProfileExportRejectsReplacedDiscardedAndExpiredPreviews(t *testing.T) {
	f, directory := exportFixture(t)
	profile := saveExportProfile(t, f, "101")
	first := profileExportPreview(t, f, profile)
	second := profileExportPreview(t, f, profile)
	if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", first.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("replaced preview = %v", err)
	}
	f.service.DeleteExportPreview("another-owner", second.ID)
	f.service.DeleteExportPreview("profile-owner", second.ID)
	if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", second.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("discarded preview = %v", err)
	}
	synctest.Test(t, func(t *testing.T) {
		view := profileExportPreview(t, f, profile)
		time.Sleep(10*time.Minute + time.Second)
		if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
			t.Fatalf("expired profile preview = %v", err)
		}
	})
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid preview artifacts = %d, %v", len(entries), err)
	}
}

func TestProfileExportRejectsChangedTargetBeforeEnqueue(t *testing.T) {
	f, directory := exportFixture(t)
	view := profileExportPreview(t, f, saveExportProfile(t, f, "101"))
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-target-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", view.ID, true); !errors.Is(err, targetguard.ErrChanged) {
		t.Fatalf("changed target export = %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("changed target artifacts = %d, %v", len(entries), err)
	}
}

func TestProfileExportWhenQueuedScopeChangesFailsWithoutPartialFile(t *testing.T) {
	for _, change := range []string{"profile-update", "profile-delete", "target", "closed-directory"} {
		t.Run(change, func(t *testing.T) {
			f, directory := exportFixture(t)
			first, second := saveExportProfile(t, f, "101"), saveExportProfile(t, f, "102")
			if err := f.service.CloseExports(); err != nil {
				t.Fatal(err)
			}
			gate := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(gate) }) }
			t.Cleanup(release)
			f.service = accountworkbench.New(f.private, f.tasks, f.business, nil, &batchRunner{group: f.runner, gate: gate, done: make(chan string, 1)})
			if err := f.service.UseExportDirectory(directory); err != nil {
				t.Fatal(err)
			}
			view := profileExportPreview(t, f, first, second)
			if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", view.ID, true); err != nil {
				t.Fatal(err)
			}
			var err error
			switch change {
			case "profile-update":
				_, err = f.service.SaveLoginProfile(context.Background(), "profile-owner", accountworkbench.LoginProfileSaveInput{ID: second.ID, Revision: second.Revision, AccountID: second.AccountID, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: second.Email, Password: "replacement-private-password"}})
			case "profile-delete":
				err = f.service.DeleteLoginProfile(context.Background(), "profile-owner", second.ID, second.Revision, true)
			case "target":
				err = f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-target-key", 3)
			case "closed-directory":
				err = f.service.CloseExports()
			}
			if err != nil {
				t.Fatal(err)
			}
			release()
			task := f.await(t)
			rows, ok := task.Result["items"].([]accountworkbench.ResultItem)
			if task.Status != "failed" || !ok || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Report != nil {
				t.Fatalf("changed scope result = %+v", task)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("partial profile artifacts = %d, %v", len(entries), err)
			}
		})
	}
}

func TestProfileExportPreservesOriginalStructuredDataAndExactNumbers(t *testing.T) {
	f, directory := exportFixture(t)
	profile := saveExportProfile(t, f, "101")
	target, err := f.private.TargetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := json.Marshal([]string{target.BaseURL, target.AdminKey})
	digest := sha256.Sum256(identity)
	stored, err := f.private.WorkbenchLoginProfile(context.Background(), hex.EncodeToString(digest[:]), profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Login = json.RawMessage(`{"email":"owner@example.com","password":"private-profile-password","extension":{"price":0.123456789123456789,"id":9007199254740993,"items":[null,true,"original"]}}`)
	if _, err := f.private.SaveWorkbenchLoginProfile(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	profiles, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(profiles) != 1 {
		t.Fatalf("saved profile = %+v, %v", profiles, err)
	}
	metadata := completeProfileExport(t, f, profileExportPreview(t, f, profiles[0]))
	data, err := os.ReadFile(filepath.Join(directory, metadata.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), string(stored.Login)) {
		t.Fatal("private export changed or dropped the original structured login data")
	}
}

func TestProfileExportAfterRestartPreservesKindAndOwnerTargetIsolation(t *testing.T) {
	f, directory := exportFixture(t)
	metadata := completeProfileExport(t, f, profileExportPreview(t, f, saveExportProfile(t, f, "101")))
	if err := reloadExportService(t, f, directory); err != nil {
		t.Fatal(err)
	}
	list, err := f.service.Exports(context.Background(), "profile-owner")
	if err != nil || len(list) != 1 || list[0] != metadata || list[0].Kind != accountworkbench.ExportLoginProfiles {
		t.Fatalf("restored profile metadata = %+v, %v", list, err)
	}
	list, err = f.service.Exports(context.Background(), "another-owner")
	if err != nil || len(list) != 0 {
		t.Fatal("another owner can list the restored private profile file")
	}
	if err := f.service.DeleteExport(context.Background(), "another-owner", metadata.ID); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("another owner deletion = %v", err)
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-target-key", 3); err != nil {
		t.Fatal(err)
	}
	list, err = f.service.Exports(context.Background(), "profile-owner")
	if err != nil || len(list) != 0 {
		t.Fatal("changed target can list the restored private profile file")
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "test-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteExport(context.Background(), "profile-owner", metadata.ID); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("deleted profile artifacts = %d, %v", len(entries), err)
	}
}
