package accountworkbench_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestSecurityDifferentOfficialUserCannotMutateSelectedAccount(t *testing.T) {
	f := newSecurityFixture(t)
	f.browser.identity.UserID = "another-user"
	f.startSecurity(t, "totp")
	f.continueSecurity(t, "failed")
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.enrolls != 0 || f.browser.activations != 0 {
		t.Fatal("different official user modified the selected account")
	}
}

func TestSecurityAccountSummaryRejectsCredentialMaterialInPublicUserID(t *testing.T) {
	f := newSecurityFixture(t)
	f.remote.accounts["101"]["credentials"].(map[string]any)["chatgpt_user_id"] = "export-access-private"
	_, err := f.service.StartSecurity(context.Background(), "security-owner", accountworkbench.SecurityStartInput{AccountID: "101", Operation: "totp", Confirmed: true})
	if err == nil {
		t.Fatal("credential value accepted as public official user ID")
	}
}

func TestSecurityConflictingStoredIdentityCannotStartBrowserTask(t *testing.T) {
	f := newSecurityFixture(t)
	f.remote.accounts["101"]["extra"] = map[string]any{"chatgpt_user_id": "conflicting-user"}
	if _, err := f.service.StartSecurity(context.Background(), "security-owner", accountworkbench.SecurityStartInput{AccountID: "101", Operation: "totp", Confirmed: true}); err == nil {
		t.Fatal("conflicting identity metadata started a security task")
	}
}

func TestSecurityLiveAccountIdentityChangeCannotUseOldConfirmation(t *testing.T) {
	f := newSecurityFixture(t)
	f.startSecurity(t, "totp")
	f.remote.mu.Lock()
	f.remote.accounts["101"]["credentials"].(map[string]any)["chatgpt_user_id"] = "replaced-user"
	f.remote.mu.Unlock()
	f.continueSecurity(t, "failed")
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.enrolls != 0 {
		t.Fatal("changed account identity used stale safety confirmation")
	}
}

func TestSecurityManagementIdentityChangeBlocksQueuedContinuation(t *testing.T) {
	f := newSecurityFixture(t)
	f.startSecurity(t, "totp")
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-management-key", 3); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ContinueSecurity(context.Background(), "security-owner", f.view.ID); err == nil {
		t.Fatal("security task survived replacement of management identity")
	}
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.enrolls != 0 {
		t.Fatal("changed management target reached official write")
	}
}

func TestSecurityAnotherConsoleSessionCannotReadControlOrCancel(t *testing.T) {
	f := newSecurityFixture(t)
	f.startSecurity(t, "totp")
	if _, err := f.service.ReadSecurity(context.Background(), "another-owner", f.view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("another console session read a security screenshot")
	}
	if err := f.service.InputSecurity(context.Background(), "another-owner", f.view.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("another console session controlled safety browser")
	}
	if err := f.service.ContinueSecurity(context.Background(), "another-owner", f.view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("another console session continued a security write")
	}
	if err := f.service.CancelSecurity("another-owner", f.view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("another console session cancelled a security task")
	}
	f.continueSecurity(t, "succeeded")
}

func TestSecurityPrivateDirectoryRejectsSymlinkAndNonPrivatePermissions(t *testing.T) {
	for _, kind := range []string{"symlink", "permissions"} {
		t.Run(kind, func(t *testing.T) {
			f := newImportFixture(t, nil)
			directory := filepath.Join(t.TempDir(), "security")
			if kind == "symlink" {
				if err := os.Symlink(t.TempDir(), directory); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(directory, 0755); err != nil {
				t.Fatal(err)
			}
			if err := f.service.UseSecurityDirectory(directory); !errors.Is(err, accountworkbench.ErrSecurityStorage) {
				t.Fatal("unsafe security directory accepted")
			}
		})
	}
}
