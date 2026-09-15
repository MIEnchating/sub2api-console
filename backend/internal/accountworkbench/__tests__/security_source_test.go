package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

func sourceSecurityFixture(t *testing.T, scope accountworkbench.ExportScope, claims string) (*securityFixture, accountworkbench.OAuthView) {
	t.Helper()
	done := make(chan string, 4)
	f := securityFixtureWithConfiguration(t, func(group *taskrunner.Group) taskrunner.Runner { return &batchRunner{group: group, done: done} }, scope == accountworkbench.ScopeLocalExport)
	browser := &oauthTestBrowser{closed: make(chan struct{})}
	f.service.UseOAuthBrowser(browser)
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		jwt := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".source-private"
		body, _ := json.Marshal(map[string]any{"access_token": jwt, "refresh_token": "rt_source_private", "token_type": "Bearer"})
		return oauthResponse(string(body)), nil
	}))
	view, err := f.service.StartOAuthWithInput(context.Background(), "security-owner", accountworkbench.OAuthStartInput{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuth("security-owner", view.ID) })
	f.awaitSecurity(t, "waiting")
	if err := f.service.FinishOAuth("security-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitSecurity(t, "authorized")
	select {
	case id := <-done:
		if id != view.ID {
			t.Fatal("unexpected task completion")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth browser did not release its reservation")
	}
	return f, view
}

const sourceSecurityClaims = `{"email":"owner@example.com","https://api.openai.com/auth":{"chatgpt_user_id":"user-101","chatgpt_account_id":"workspace-101"}}`

func TestSecuritySourceLocalOperationsRetainExportAndPrivateIdentityWithoutManagement(t *testing.T) {
	for _, operation := range []string{"password", "totp"} {
		t.Run(operation, func(t *testing.T) {
			f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
			if err := f.private.ConfigureTarget(context.Background(), "https://changed.example", "changed-test-key", 3); err != nil {
				t.Fatal(err)
			}
			summary, err := f.service.SecuritySource(context.Background(), "security-owner", source.ID)
			if err != nil || summary.Email != "owner@example.com" || summary.UserID != "user-101" || summary.WorkspaceID != "workspace-101" || summary.Scope != accountworkbench.ScopeLocalExport {
				t.Fatalf("source summary = %+v, %v", summary, err)
			}
			f.browser.passwordResult = true
			input := accountworkbench.SecurityStartInput{Scope: accountworkbench.ScopeLocalExport, SourceOAuthID: source.ID, Operation: operation, Confirmed: true}
			if operation == "password" {
				input.Password = securityPassword
			}
			f.view, err = f.service.StartSecurity(context.Background(), "security-owner", input)
			if err != nil {
				t.Fatal(err)
			}
			if f.view.AccountID != "" || f.view.SourceOAuthID != source.ID || f.view.ExpiresAt != source.ExpiresAt {
				t.Fatalf("source binding = %+v", f.view)
			}
			f.awaitSecurity(t, "waiting")
			task := f.continueSecurity(t, "succeeded")
			if _, found := task.Result["account_id"]; found || task.Result["source_oauth_id"] != source.ID {
				t.Fatal("task invented an online account binding")
			}
			preview, err := f.service.PreviewOAuth(context.Background(), "security-owner", source.ID, accountworkbench.OAuthPreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true})
			if err != nil || len(preview.Items) != 1 {
				t.Fatalf("original authorization no longer exportable: %v", err)
			}
			raw, err := os.ReadFile(filepath.Join(f.directory, f.view.ID+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var artifact map[string]any
			if json.Unmarshal(raw, &artifact) != nil || artifact["source_oauth_id"] != source.ID || artifact["scope"] != "local-export" || artifact["user_id"] != "user-101" || artifact["workspace_id"] != "workspace-101" {
				t.Fatal("private artifact lost source identity")
			}
			if _, present := artifact["account_id"]; present {
				t.Fatal("private source artifact invented account ID")
			}
			public, _ := json.Marshal([]any{summary, f.view, task, preview})
			for _, secret := range []string{securityPassword, securitySecret, "source-private", "rt_source_private", "private-session-id"} {
				if strings.Contains(string(public), secret) {
					t.Fatal("public metadata exposed private source or security material")
				}
			}
			_, err = f.service.ApplySecurityToLoginProfile(context.Background(), "security-owner", accountworkbench.LoginProfileSecurityInput{SecurityID: f.view.ID, ProfileID: "missing", Revision: 1, Confirmed: true})
			if err == nil || !strings.Contains(err.Error(), "未绑定线上账号") {
				t.Fatalf("source artifact entered profile write path: %v", err)
			}
		})
	}
}

func TestSecuritySourceRejectsOwnerScopeAndAmbiguousAccountBeforeLaunch(t *testing.T) {
	f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
	for _, attempt := range []struct {
		name, owner, account string
		scope                accountworkbench.ExportScope
	}{
		{"other owner", "other-owner", "", accountworkbench.ScopeLocalExport},
		{"wrong scope", "security-owner", "", accountworkbench.ScopeManaged},
		{"ambiguous source", "security-owner", "101", accountworkbench.ScopeLocalExport},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			_, err := f.service.StartSecurity(context.Background(), attempt.owner, accountworkbench.SecurityStartInput{Scope: attempt.scope, SourceOAuthID: source.ID, AccountID: attempt.account, Operation: "totp", Confirmed: true})
			if err == nil {
				t.Fatal("invalid source started a security task")
			}
		})
	}
	if _, err := f.service.SecuritySource(context.Background(), "other-owner", source.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatalf("source identity exposed to other owner: %v", err)
	}
}

func TestSecuritySourceCancelledAuthorizationRejectsFurtherSteps(t *testing.T) {
	f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
	var err error
	f.view, err = f.service.StartSecurity(context.Background(), "security-owner", accountworkbench.SecurityStartInput{Scope: accountworkbench.ScopeLocalExport, SourceOAuthID: source.ID, Operation: "totp", Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	f.awaitSecurity(t, "waiting")
	if err := f.service.CancelOAuth("security-owner", source.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ContinueSecurity(context.Background(), "security-owner", f.view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatalf("cancelled source still writable: %v", err)
	}
	if _, err := f.service.ReadSecurity(context.Background(), "security-owner", f.view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatalf("cancelled source exposed screenshot: %v", err)
	}
	f.browser.mu.Lock()
	defer f.browser.mu.Unlock()
	if f.browser.enrolls != 0 || f.browser.activations != 0 {
		t.Fatal("cancelled source changed official security")
	}
}

func TestSecuritySourceManagedTargetChangeRejectsSummaryAndStart(t *testing.T) {
	f, source := sourceSecurityFixture(t, accountworkbench.ScopeManaged, sourceSecurityClaims)
	if err := f.private.ConfigureTarget(context.Background(), "https://changed.example", "other-test-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SecuritySource(context.Background(), "security-owner", source.ID); err == nil {
		t.Fatal("changed managed target accepted source summary")
	}
	if _, err := f.service.StartSecurity(context.Background(), "security-owner", accountworkbench.SecurityStartInput{SourceOAuthID: source.ID, Operation: "totp", Confirmed: true}); err == nil {
		t.Fatal("changed managed target accepted source security")
	}
}

func TestSecuritySourceMissingOfficialIdentityCannotStartSecurity(t *testing.T) {
	f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, `{}`)
	if _, err := f.service.SecuritySource(context.Background(), "security-owner", source.ID); !errors.Is(err, browserlogin.ErrSecurityIdentity) {
		t.Fatalf("missing official identity was accepted: %v", err)
	}
}
