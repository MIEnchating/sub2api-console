package accountworkbench_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestSecurityBatchSuccessPersistsEachPrivateArtifactWithoutExposingCredentials(t *testing.T) {
	for _, operation := range []string{"password", "totp"} {
		t.Run(operation, func(t *testing.T) {
			f := newSecurityBatchFixture(t, nil)
			preview := f.previewSecurityBatch(t, operation, "101", "102")
			view, err := f.service.StartSecurityBatch(context.Background(), "owner", preview.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.service.CancelSecurityBatch("owner", view.ID) })
			first := f.finishSecurityChild(t, operation)
			second := f.finishSecurityChild(t, operation)
			if first == second {
				t.Fatal("batch reused child security session")
			}
			terminal := f.awaitTask(t, "account-workbench-security-batch", "succeeded")
			f.awaitDone(t, view.ID)
			stored, err := f.service.ReadSecurityBatch("owner", view.ID)
			if err != nil || stored.Completed != 2 || stored.Succeeded != 2 || stored.CurrentSecurityID != "" {
				t.Fatalf("batch did not complete both accounts: %+v %v", stored, err)
			}
			for i, row := range stored.Items {
				if row.Status != "succeeded" || row.ArtifactID == "" {
					t.Fatal("successful row lacks private result reference")
				}
				raw, err := os.ReadFile(filepath.Join(f.directory, row.ArtifactID+".json"))
				if err != nil {
					t.Fatal(err)
				}
				var artifact struct {
					AccountID string `json:"account_id"`
					UserID    string `json:"user_id"`
					Password  string `json:"password"`
					Secret    string `json:"secret"`
				}
				if json.Unmarshal(raw, &artifact) != nil {
					t.Fatal("private artifact invalid")
				}
				if artifact.AccountID != []string{"101", "102"}[i] || artifact.UserID != row.UserID {
					t.Fatal("private artifact bound to another account")
				}
				if operation == "password" && artifact.Password != securityPassword {
					t.Fatal("batch password changed")
				}
				if operation == "totp" && artifact.Secret != securitySecret {
					t.Fatal("TOTP private artifact missing")
				}
			}
			for _, value := range []any{preview, view, stored, terminal} {
				raw, _ := json.Marshal(value)
				for _, secret := range []string{securityPassword, securitySecret, "private-session-", "export-access-private", "rt_export_private", f.directory} {
					if strings.Contains(string(raw), secret) {
						t.Fatal("security batch exposed credentials or private path")
					}
				}
			}
		})
	}
}

func TestSecurityBatchPartialFailureKeepsIndependentPrivateResultsAndDoesNotReplay(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	f.factory.browsers[0].activateError = browserlogin.ErrSecurityUncertain
	view := f.startSecurityBatch(t, "totp", "101", "102")
	f.finishSecurityChild(t, "totp")
	f.finishSecurityChild(t, "totp")
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Status != "partial" || stored.Succeeded != 1 || stored.Completed != 2 || stored.Items[0].Status != "failed" || stored.Items[0].ArtifactID == "" || stored.Items[1].Status != "succeeded" {
		t.Fatalf("partial result invalid: %+v %v", stored, err)
	}
	f.factory.browsers[0].mu.Lock()
	defer f.factory.browsers[0].mu.Unlock()
	if f.factory.browsers[0].activations != 1 {
		t.Fatal("uncertain TOTP activation replayed")
	}
}

func TestSecurityBatchQueuedCancellationPersistsTerminalStateAndDoesNotOpenBrowser(t *testing.T) {
	gate := make(chan struct{})
	f := newSecurityBatchFixture(t, gate)
	view := f.startSecurityBatch(t, "password", "101", "102")
	view.Items[0].AccountID = "modified"
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Items[0].AccountID != "101" {
		t.Fatal("returned batch rows modify stored identity")
	}
	if err := f.service.CancelSecurityBatch("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	close(gate)
	f.awaitDone(t, view.ID)
	task, err := f.tasks.Get(context.Background(), view.ID)
	if err != nil || task.Status != "cancelled" {
		t.Fatalf("pre-start cancellation not persisted: %s %v", task.Status, err)
	}
	f.factory.mu.Lock()
	defer f.factory.mu.Unlock()
	if f.factory.opened != 0 {
		t.Fatal("cancelled batch opened security browser")
	}
}

func TestSecurityBatchSnapshotChangeAfterQueueStopsBeforeOpeningBrowser(t *testing.T) {
	gate := make(chan struct{})
	f := newSecurityBatchFixture(t, gate)
	view := f.startSecurityBatch(t, "totp", "101", "102")
	f.remote.mu.Lock()
	f.remote.accounts["101"]["credentials"].(map[string]any)["chatgpt_account_id"] = "rebound-workspace"
	f.remote.mu.Unlock()
	close(gate)
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Status != "failed" || stored.Succeeded != 0 {
		t.Fatalf("queued identity drift accepted: %+v %v", stored, err)
	}
	f.factory.mu.Lock()
	defer f.factory.mu.Unlock()
	if f.factory.opened != 0 {
		t.Fatal("rebound account opened browser")
	}
}

func TestSecurityBatchWaitsForCleanupAndExcludesSingleSecurityAndOAuth(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.factory.browsers[0].closeRelease = release
	view := f.startSecurityBatch(t, "totp", "101", "102")
	f.finishSecurityChild(t, "totp")
	awaitSecurityClosed(t, f.factory.browsers[0])
	if _, err := f.service.StartSecurity(context.Background(), "owner", accountworkbench.SecurityStartInput{AccountID: "102", Operation: "totp", Confirmed: true}); err == nil {
		t.Fatal("single security operation bypassed batch reservation")
	}
	if _, err := f.service.StartOAuth(context.Background(), "owner"); err == nil {
		t.Fatal("OAuth bypassed security batch reservation")
	}
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Items[1].Status != "queued" {
		t.Fatal("next child started before browser cleanup")
	}
	unblock()
	f.finishSecurityChild(t, "totp")
	f.awaitDone(t, view.ID)
}

func TestSecurityBatchTaskCancellationClosesBrowserAndDoesNotStartRemainingAccount(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	view := f.startSecurityBatch(t, "totp", "101", "102")
	f.awaitTask(t, "account-workbench-security-totp", "waiting")
	if !f.runBoundary.CancelTask(view.ID) {
		t.Fatal("security batch task is not cancellable")
	}
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Status != "cancelled" || stored.CurrentSecurityID != "" || stored.Items[1].Status != "cancelled" {
		t.Fatalf("batch cancellation invalid: %+v %v", stored, err)
	}
	f.factory.mu.Lock()
	defer f.factory.mu.Unlock()
	if f.factory.opened != 1 {
		t.Fatal("cancelled batch started another browser")
	}
	awaitSecurityClosed(t, f.factory.browsers[0])
}

func TestSecurityBatchFinalTaskPersistenceFailureRetainsPrivateArtifactAndReportsFailure(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	f.taskBoundary.failSecurityFinal = true
	view := f.startSecurityBatch(t, "totp", "101")
	f.finishSecurityChild(t, "totp")
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Status != "failed" || stored.Items[0].ArtifactID == "" {
		t.Fatalf("lost task persistence was reported as success: %+v %v", stored, err)
	}
	if _, err := os.Stat(filepath.Join(f.directory, stored.Items[0].ArtifactID+".result.json")); err != nil {
		t.Fatal("task persistence failure discarded completed private result")
	}
	f.factory.browsers[0].mu.Lock()
	defer f.factory.browsers[0].mu.Unlock()
	if f.factory.browsers[0].activations != 1 {
		t.Fatal("task persistence failure replayed external security write")
	}
}

func TestSecurityBatchCancellationDuringChildLaunchFailureFinishesWithoutPanic(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	f.runBoundary.rejectChild = func() { close(entered); <-release }
	view := f.startSecurityBatch(t, "password", "101")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("child launch not reached")
	}
	if err := f.service.CancelSecurityBatch("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	f.awaitDone(t, view.ID)
	task, err := f.tasks.Get(context.Background(), view.ID)
	if err != nil || task.Status != "cancelled" {
		t.Fatalf("launch cancellation not persisted: %s %v", task.Status, err)
	}
}

func TestSecurityBatchAlreadyEnabledTOTPCompletesWithoutReplacingSecret(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	f.factory.browsers[0].enabled = true
	view := f.startSecurityBatch(t, "totp", "101")
	f.finishSecurityChild(t, "totp")
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Status != "succeeded" || stored.Items[0].ArtifactID != "" {
		t.Fatalf("existing TOTP was not preserved: %+v %v", stored, err)
	}
	f.factory.browsers[0].mu.Lock()
	defer f.factory.browsers[0].mu.Unlock()
	if f.factory.browsers[0].enrolls != 0 || f.factory.browsers[0].activations != 0 {
		t.Fatal("existing TOTP secret was replaced")
	}
}
