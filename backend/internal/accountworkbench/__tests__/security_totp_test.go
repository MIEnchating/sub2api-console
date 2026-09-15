package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/pquerna/otp/totp"
)

func TestSecurityTOTPStoresPrivateKeyBeforeOneActivationAndReturnsOnlyMetadata(t *testing.T) {
	f := newSecurityFixture(t)
	f.startSecurity(t, "totp")
	f.browser.onActivate = func(enrollment browserlogin.SecurityEnrollment, code string) error {
		raw, err := os.ReadFile(filepath.Join(f.directory, f.view.ID+".json"))
		if err != nil {
			return errors.New("activation preceded durable secret storage")
		}
		var stored struct{ Secret, Email, UserID string }
		if json.Unmarshal(raw, &stored) != nil || stored.Secret != enrollment.Secret || stored.Email != "owner@example.com" {
			return errors.New("private secret artifact is incomplete")
		}
		if !totp.Validate(code, enrollment.Secret) {
			return errors.New("activation code does not match enrolled key")
		}
		return nil
	}
	task := f.continueSecurity(t, "succeeded")
	view, err := f.service.Security(context.Background(), "security-owner", f.view.ID, true)
	if err != nil || view.ArtifactID != f.view.ID || view.Image != "" {
		t.Fatalf("security metadata = %#v, %v", view, err)
	}
	public, _ := json.Marshal([]any{task, view})
	for _, secret := range []string{securitySecret, "private-session-id", "export-access-private", "rt_export_private", "test-admin-key"} {
		if strings.Contains(string(public), secret) {
			t.Fatal("security task or view leaked a credential")
		}
	}
	for _, suffix := range []string{".json", ".result.json"} {
		stat, err := os.Stat(filepath.Join(f.directory, f.view.ID+suffix))
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatal("security result is not private")
		}
	}
	f.browser.mu.Lock()
	enrolls, activations := f.browser.enrolls, f.browser.activations
	f.browser.mu.Unlock()
	if enrolls != 1 || activations != 1 {
		t.Fatal("TOTP setup did not use one enrollment and activation")
	}
	if err := f.service.ContinueSecurity(context.Background(), "security-owner", f.view.ID); err == nil {
		t.Fatal("completed TOTP write was replayable")
	}
}

func TestSecurityTOTPAlreadyEnabledCompletesWithoutEnrollmentOrArtifact(t *testing.T) {
	f := newSecurityFixture(t)
	f.browser.enabled = true
	f.startSecurity(t, "totp")
	task := f.continueSecurity(t, "succeeded")
	if task.Result["artifact_id"] != nil {
		t.Fatal("existing TOTP claimed a new key artifact")
	}
	entries, err := os.ReadDir(f.directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("existing TOTP wrote a private secret")
	}
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.enrolls != 0 || f.browser.activations != 0 {
		t.Fatal("already enabled factor was enrolled again")
	}
}

func TestSecurityTOTPUncertainActivationKeepsPrivateKeyAndCannotReplay(t *testing.T) {
	f := newSecurityFixture(t)
	f.browser.activateError = errors.New("private provider detail " + securitySecret)
	f.startSecurity(t, "totp")
	task := f.continueSecurity(t, "failed")
	if !strings.Contains(task.Message, "未确认") || strings.Contains(task.Message, securitySecret) {
		t.Fatal("uncertain activation failure was misreported")
	}
	if _, err := os.Stat(filepath.Join(f.directory, f.view.ID+".json")); err != nil {
		t.Fatal("uncertain activation discarded the recovery key")
	}
	if err := f.service.ContinueSecurity(context.Background(), "security-owner", f.view.ID); err == nil {
		t.Fatal("uncertain write was replayable")
	}
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.activations != 1 {
		t.Fatal("activation automatically retried")
	}
}

func TestSecurityTOTPInvalidEnrollmentDoesNotActivate(t *testing.T) {
	f := newSecurityFixture(t)
	f.browser.enrollment.Secret = "invalid"
	f.startSecurity(t, "totp")
	f.continueSecurity(t, "failed")
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.activations != 0 {
		t.Fatal("invalid TOTP enrollment was activated")
	}
}

func TestSecurityTOTPRefusesArtifactSymlinkBeforeActivation(t *testing.T) {
	f := newSecurityFixture(t)
	f.startSecurity(t, "totp")
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.directory, f.view.ID+".json")); err != nil {
		t.Fatal(err)
	}
	f.continueSecurity(t, "failed")
	raw, _ := os.ReadFile(outside)
	if string(raw) != "untouched" {
		t.Fatal("security storage followed an artifact symlink")
	}
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.activations != 0 {
		t.Fatal("activation continued after secret storage failed")
	}
}

func TestSecurityCancelClosesBrowserAndPreservesSharedBrowserReservationUntilCloseCompletes(t *testing.T) {
	f := newSecurityFixture(t)
	release := make(chan struct{})
	f.browser.closeRelease = release
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	f.startSecurity(t, "totp")
	if err := f.service.CancelSecurity("security-owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.browser.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not close security browser")
	}
	f.service.UseOAuthBrowser(&oauthTestBrowser{closed: make(chan struct{})})
	if _, err := f.service.StartOAuth(context.Background(), "owner"); err == nil {
		t.Fatal("another browser started before security cleanup finished")
	}
	close(release)
}
