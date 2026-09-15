package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestSecurityPasswordWaitsForVerificationAndOfficialResultWithoutResubmission(t *testing.T) {
	f := newSecurityFixture(t)
	f.browser.page.Stage = "email_code"
	f.startSecurity(t, "password")
	f.continueSecurity(t, "waiting")
	f.browser.mu.Lock()
	if f.browser.begins != 1 || f.browser.submits != 0 {
		t.Fatal("password was sent before official email verification")
	}
	f.browser.page.Stage = "new_password"
	f.browser.onSubmit = func(action browserlogin.AuthAction) error {
		raw, err := os.ReadFile(filepath.Join(f.directory, f.view.ID+".json"))
		if err != nil {
			return errors.New("password was submitted before durable private storage")
		}
		var artifact struct{ Password string }
		if json.Unmarshal(raw, &artifact) != nil || artifact.Password != securityPassword || action.Value != securityPassword || action.Stage != "new_password" {
			return errors.New("password submission did not match confirmed task")
		}
		return nil
	}
	f.browser.mu.Unlock()
	task := f.continueSecurity(t, "waiting")
	if task.Status != "waiting_input" {
		t.Fatal("missing official password response was reported as success")
	}
	f.continueSecurity(t, "waiting")
	f.browser.mu.Lock()
	f.browser.passwordResult = true
	f.browser.mu.Unlock()
	task = f.continueSecurity(t, "succeeded")
	public, _ := json.Marshal(task)
	if strings.Contains(string(public), securityPassword) || task.Result["artifact_id"] != f.view.ID {
		t.Fatal("password task did not return only its private artifact reference")
	}
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.begins != 1 || f.browser.submits != 1 {
		t.Fatal("continue replayed password reauthentication or submission")
	}
}

func TestSecurityPasswordChangedPageAfterSubmissionAttemptStopsWithoutRetry(t *testing.T) {
	f := newSecurityFixture(t)
	f.browser.submitError = browserlogin.ErrAuthPageChanged
	f.startSecurity(t, "password")
	task := f.continueSecurity(t, "failed")
	if !strings.Contains(task.Message, "未确认") {
		t.Fatal("attempted password submission did not require manual outcome review")
	}
	if err := f.service.ContinueSecurity(context.Background(), "security-owner", f.view.ID); err == nil {
		t.Fatal("password write could be repeated after a changed page response")
	}
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.submits != 1 {
		t.Fatal("password was automatically resubmitted")
	}
}

func TestSecurityPasswordValidationRejectsUnconfirmedWeakAndCrossOperationInputs(t *testing.T) {
	f := newSecurityFixture(t)
	for _, input := range []accountworkbench.SecurityStartInput{
		{AccountID: "101", Operation: "password", Password: securityPassword},
		{AccountID: "101", Operation: "password", Password: "weak", Confirmed: true},
		{AccountID: "101", Operation: "password", Password: "onlylowercasepassword", Confirmed: true},
		{AccountID: "101", Operation: "totp", Password: securityPassword, Confirmed: true},
		{AccountID: "01", Operation: "password", Password: securityPassword, Confirmed: true},
	} {
		if _, err := f.service.StartSecurity(context.Background(), "security-owner", input); err == nil {
			t.Fatal("invalid or unconfirmed safety input started a task")
		}
	}
	entries, err := os.ReadDir(f.directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("rejected password input persisted a secret")
	}
}

func TestSecurityAuthCannotReplaceConfirmedPasswordOrLoginEmail(t *testing.T) {
	f := newSecurityFixture(t)
	f.startSecurity(t, "password")
	for _, action := range []browserlogin.AuthAction{
		{Stage: "new_password", Revision: strings.Repeat("a", 64), Value: "Replacement!923Password"},
		{Stage: "email", Revision: strings.Repeat("a", 64), Value: "different@example.com"},
	} {
		if err := f.service.SubmitSecurityAuth(context.Background(), "security-owner", f.view.ID, action); err == nil {
			t.Fatal("auth input replaced the confirmed security identity or password")
		}
	}
}
