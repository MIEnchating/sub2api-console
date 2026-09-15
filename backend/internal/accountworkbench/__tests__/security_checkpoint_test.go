package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestSecurityCheckpointRequiresObservedIdentityConfirmationBeforePasswordOrTOTP(t *testing.T) {
	for _, operation := range []string{"password", "totp"} {
		t.Run(operation, func(t *testing.T) {
			f, browser, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
			if err := f.private.ConfigureTarget(context.Background(), "https://changed.example", "changed-test-key", 3); err != nil {
				t.Fatal(err)
			}
			view := startCheckpointSecurity(t, f, checkpoint, operation)
			f.phase(t, "awaiting_confirmation")
			current, err := f.service.ReadSecurity(context.Background(), "checkpoint-owner", view.ID)
			if err != nil || current.Email != "owner@example.com" || current.UserID != "user-1" || current.IdentityConfirmed || current.SourceCheckpointID != checkpoint.ID || current.AccountID != "" || current.SourceOAuthID != "" {
				t.Fatalf("unconfirmed identity view = %+v %v", current, err)
			}
			if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", view.ID); err == nil {
				t.Fatal("unconfirmed official identity allowed continuation")
			}
			if err := f.service.ConfirmSecurityIdentity(context.Background(), "checkpoint-owner", view.ID, accountworkbench.SecurityIdentityInput{Email: current.Email, UserID: "wrong-user", Confirmed: true}); !errors.Is(err, browserlogin.ErrSecurityIdentity) {
				t.Fatalf("wrong identity accepted: %v", err)
			}
			browser.mu.Lock()
			writes := browser.begins + browser.enrolls + browser.submits + browser.activations
			browser.mu.Unlock()
			if writes != 0 {
				t.Fatal("security writes ran before explicit identity confirmation")
			}
			confirmCheckpointIdentity(t, f, view.ID)
			if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", view.ID); err != nil {
				t.Fatal(err)
			}
			task := f.phase(t, "succeeded")
			current, err = f.service.ReadSecurity(context.Background(), "checkpoint-owner", view.ID)
			if err != nil || !current.IdentityConfirmed || current.ArtifactID == "" {
				t.Fatalf("confirmed security did not complete: %+v %v", current, err)
			}
			public, _ := json.Marshal([]any{task, current})
			for _, secret := range []string{securityPassword, securitySecret, "private-proxy", "private-security-enrollment"} {
				if strings.Contains(string(public), secret) {
					t.Fatal("checkpoint security public result exposed secret")
				}
			}
			if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Scope: checkpoint.Scope, Revision: checkpoint.Revision, Confirmed: true}); err == nil {
				t.Fatal("transferred old OAuth transaction remained replayable")
			}
		})
	}
}

func TestSecurityCheckpointPendingIdentityAllowsLoginButNotSecurityWrite(t *testing.T) {
	f, browser, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
	browser.identityPending = true
	view := startCheckpointSecurity(t, f, checkpoint, "totp")
	f.phase(t, "waiting")
	if err := f.service.InputSecurity(context.Background(), "checkpoint-owner", view.ID, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	browser.stateMu.Lock()
	browser.identityPending = false
	browser.stateMu.Unlock()
	if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "awaiting_confirmation")
	browser.mu.Lock()
	defer browser.mu.Unlock()
	if browser.enrolls != 0 || browser.activations != 0 {
		t.Fatal("identity discovery performed an MFA write")
	}
}

func TestSecurityCheckpointChangedOfficialIdentityFailsSecondConfirmation(t *testing.T) {
	f, browser, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
	view := startCheckpointSecurity(t, f, checkpoint, "totp")
	f.phase(t, "awaiting_confirmation")
	browser.mu.Lock()
	browser.identity.UserID = "changed-user"
	browser.mu.Unlock()
	if err := f.service.ConfirmSecurityIdentity(context.Background(), "checkpoint-owner", view.ID, accountworkbench.SecurityIdentityInput{Email: "owner@example.com", UserID: "user-1", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "failed")
	browser.mu.Lock()
	defer browser.mu.Unlock()
	if browser.enrolls != 0 {
		t.Fatal("identity changed after observation but MFA was written")
	}
}

func TestSecurityCheckpointWrongVersionOwnerAndScopeLeaveSnapshotUnconsumed(t *testing.T) {
	f, browser, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeManaged)
	for _, attempt := range []struct {
		name, owner string
		scope       accountworkbench.ExportScope
		revision    int64
	}{
		{"wrong owner", "other-owner", checkpoint.Scope, checkpoint.Revision},
		{"wrong scope", "checkpoint-owner", accountworkbench.ScopeLocalExport, checkpoint.Revision},
		{"stale revision", "checkpoint-owner", checkpoint.Scope, checkpoint.Revision - 1},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			_, err := f.service.StartCheckpointSecurity(context.Background(), attempt.owner, checkpoint.ID, accountworkbench.SecurityCheckpointStartInput{OAuthCheckpointAction: accountworkbench.OAuthCheckpointAction{Scope: attempt.scope, Revision: attempt.revision, Confirmed: true}, Operation: "totp"})
			if err == nil {
				t.Fatal("invalid checkpoint binding was accepted")
			}
		})
	}
	browser.stateMu.Lock()
	defer browser.stateMu.Unlock()
	if browser.opens != 0 {
		t.Fatal("invalid source consumed browser snapshot")
	}
}

func TestSecurityCheckpointUncertainTransferCannotBeReplayed(t *testing.T) {
	f, browser, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
	browser.openErr = errors.New("private-proxy uncertain transfer")
	view := startCheckpointSecurity(t, f, checkpoint, "totp")
	task := f.phase(t, "failed")
	if strings.Contains(task.Message, "private-proxy") {
		t.Fatal("transfer error exposed private transport")
	}
	if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", view.ID); err == nil {
		t.Fatal("failed transfer remained executable")
	}
	_, err := f.service.StartCheckpointSecurity(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.SecurityCheckpointStartInput{OAuthCheckpointAction: accountworkbench.OAuthCheckpointAction{Scope: checkpoint.Scope, Revision: checkpoint.Revision, Confirmed: true}, Operation: "totp"})
	if err == nil {
		t.Fatal("uncertain transfer snapshot was replayed")
	}
}
