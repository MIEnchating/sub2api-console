package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type sourceProfileBrowserFactory struct {
	factory batchBrowserFactory
	options chan browserlogin.OAuthOptions
}

func (f *sourceProfileBrowserFactory) OpenOAuth(ctx context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	f.options <- options
	return f.factory.OpenOAuth(ctx, options)
}

func awaitSourceProfileTask(t *testing.T, f *securityFixture, operation, phase string) taskstore.Task {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case task := <-f.events:
			if task.Operation == operation && task.Result["phase"] == phase {
				return task
			}
		case <-deadline.C:
			t.Fatalf("source profile task did not reach %s/%s", operation, phase)
			return taskstore.Task{}
		}
	}
}

func TestSourceProfileFreshAuthorizationChecksStableProfileAndOfficialIdentity(t *testing.T) {
	for _, outcome := range []string{"success", "wrong-user", "changed-profile"} {
		t.Run(outcome, func(t *testing.T) {
			f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
			if err := f.service.UseExportDirectory(filepath.Join(t.TempDir(), "exports")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.service.CloseExports() })
			profile := saveLocalSourceProfile(t, f.service, "security-owner", accountworkbench.SourceProfileReference{SourceOAuthID: source.ID})
			factory := &sourceProfileBrowserFactory{options: make(chan browserlogin.OAuthOptions, 2)}
			f.service.UseOAuthBrowser(factory)
			verifiers := make(chan string, 1)
			f.service.UseOAuthTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
				if err := request.ParseForm(); err != nil {
					t.Error(err)
				}
				verifiers <- request.Form.Get("code_verifier")
				user := "user-101"
				if outcome == "wrong-user" {
					user = "another-user"
				}
				return localRefreshResponse(user, "workspace-101"), nil
			}))
			input := accountworkbench.SourceProfileSelectionInput{Scope: accountworkbench.ScopeLocalExport, FreshLogin: true, Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}}}
			preview, err := f.service.PreviewSourceProfileAuthorization(context.Background(), "security-owner", input)
			if err != nil || !preview.FreshLogin || preview.Target != "" || preview.Items[0].ProfileID != profile.ID || preview.Items[0].ProfileRevision != profile.Revision || preview.Items[0].AccountID != "" {
				t.Fatalf("profile auth preview = %+v, %v", preview, err)
			}
			batch, err := f.service.StartOAuthBatch(context.Background(), "security-owner", preview.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.service.CancelOAuthBatch("security-owner", batch.ID) })
			waiting := awaitSourceProfileTask(t, f, "account-workbench-oauth", "waiting")
			options := <-factory.options
			if options.State == "" || options.Recovery != nil || options.Validate() != nil {
				t.Fatal("saved profile did not start a fresh valid OAuth transaction")
			}
			if outcome == "changed-profile" {
				if _, err := f.service.SaveSourceProfile(context.Background(), "security-owner", accountworkbench.SourceProfileSaveInput{Scope: accountworkbench.ScopeLocalExport, ID: profile.ID, Revision: profile.Revision, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: profile.Email}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.service.FinishOAuth("security-owner", waiting.ID); err != nil {
				t.Fatal(err)
			}
			phase := "failed"
			if outcome == "success" {
				phase = "authorized"
			}
			awaitSourceProfileTask(t, f, "account-workbench-oauth-batch", phase)
			view, err := f.service.ReadOAuthBatch("security-owner", batch.ID)
			if err != nil {
				t.Fatal(err)
			}
			if outcome == "success" && view.Available != 1 || outcome != "success" && view.Available != 0 {
				t.Fatal("batch exposed credentials despite invalid profile or identity")
			}
			if verifier := <-verifiers; len(verifier) < 43 {
				t.Fatal("authorization did not use a fresh PKCE verifier")
			}
			if outcome == "success" {
				exportPreview, err := f.service.PreviewOAuthBatchImport(context.Background(), "security-owner", batch.ID, accountworkbench.OAuthPreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true})
				if err != nil || len(exportPreview.Items) != 1 || exportPreview.Items[0].AccountID != "" {
					t.Fatalf("fresh local credentials cannot be exported: %v", err)
				}
				if _, err := f.service.ExportInput(context.Background(), "security-owner", exportPreview.ID, true); err != nil {
					t.Fatal(err)
				}
				if task := awaitSourceProfileTask(t, f, "account-workbench-convert", "complete"); task.Status != "succeeded" {
					t.Fatal("fresh local authorization export failed")
				}
				artifacts, err := f.service.LocalExports(context.Background(), "security-owner")
				if err != nil || len(artifacts) != 1 || artifacts[0].Count != 1 {
					t.Fatal("fresh local authorization did not produce a private artifact")
				}
			}
			if outcome != "success" {
				profiles, err := f.service.SourceProfiles(context.Background(), "security-owner", accountworkbench.ScopeLocalExport)
				if err != nil {
					t.Fatal(err)
				}
				input.Items[0].Revision, input.FailedBatchID = profiles[0].Revision, batch.ID
				if _, err := f.service.PreviewSourceProfileAuthorization(context.Background(), "security-owner", input); err != nil {
					t.Fatalf("failed local profile could not be explicitly retried: %v", err)
				}
			}
		})
	}
}

func TestSourceProfileSMSRequiresCurrentBatchConfirmationBeforePurchase(t *testing.T) {
	f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
	profile := saveLocalSourceProfile(t, f.service, "security-owner", accountworkbench.SourceProfileReference{SourceOAuthID: source.ID})
	profile, err := f.service.SaveSourceProfile(context.Background(), "security-owner", accountworkbench.SourceProfileSaveInput{Scope: accountworkbench.ScopeLocalExport, ID: profile.ID, Revision: profile.Revision, Login: smsLoginInput(), Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("security-owner"))
	stored, err := f.private.WorkbenchSourceProfile(context.Background(), hex.EncodeToString(digest[:]), "local-export", profile.ID)
	var login accountworkbench.OAuthLoginInput
	if err != nil || json.Unmarshal(stored.Login, &login) != nil || login.SMS == nil || login.SMS.Confirmed {
		t.Fatal("saving source profile retained a prior SMS confirmation")
	}
	browser := &assistBrowser{oauthTestBrowser: &oauthTestBrowser{closed: make(chan struct{})}, stages: []string{"phone", "sms_code"}, actions: make(chan browserlogin.AuthAction, 2)}
	f.service.UseOAuthBrowser(browser)
	ticks := make(chan time.Time)
	f.service.UseOAuthAssistTicks(ticks)
	var purchases atomic.Int32
	finished := make(chan struct{}, 1)
	f.service.UseProviderTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Query().Get("action") {
		case "getNumber":
			purchases.Add(1)
			return oauthResponse("ACCESS_NUMBER:source-order:15555550101"), nil
		case "getStatus":
			return oauthResponse("STATUS_OK:654321"), nil
		case "setStatus":
			if request.URL.Query().Get("status") == "6" {
				finished <- struct{}{}
				return oauthResponse("ACCESS_ACTIVATION"), nil
			}
			return oauthResponse("ACCESS_READY"), nil
		default:
			t.Error("unexpected SMS provider request")
			return oauthResponse("BAD_ACTION"), nil
		}
	}))
	preview, err := f.service.PreviewSourceProfileAuthorization(context.Background(), "security-owner", accountworkbench.SourceProfileSelectionInput{Scope: accountworkbench.ScopeLocalExport, FreshLogin: true, Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}}})
	if err != nil || preview.ID == "" || preview.Items[0].SMSProvider != "smsbower" {
		t.Fatalf("stored SMS profile could not be previewed: %+v, %v", preview, err)
	}
	if _, err := f.service.StartOAuthBatch(context.Background(), "security-owner", preview.ID, false); err == nil || purchases.Load() != 0 {
		t.Fatal("unconfirmed batch contacted SMS provider")
	}
	batch, err := f.service.StartOAuthBatch(context.Background(), "security-owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuthBatch("security-owner", batch.ID) })
	waiting := awaitSourceProfileTask(t, f, "account-workbench-oauth", "waiting")
	ticks <- time.Now()
	if action := awaitAssistAction(t, browser); action.Stage != "phone" || action.Value != "+15555550101" {
		t.Fatal("confirmed source batch did not bind purchased number")
	}
	ticks <- time.Now()
	if action := awaitAssistAction(t, browser); action.Stage != "sms_code" || action.Value != "654321" {
		t.Fatal("confirmed source batch did not submit SMS code")
	}
	if err := f.service.FinishOAuth("security-owner", waiting.ID); err != nil {
		t.Fatal(err)
	}
	awaitSourceProfileTask(t, f, "account-workbench-oauth-batch", "authorized")
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("authorized source batch did not complete the SMS order")
	}
	if purchases.Load() != 1 {
		t.Fatal("source profile replayed an SMS purchase")
	}
}

func TestSourceProfileChangedAfterPreviewCannotStartNewOAuth(t *testing.T) {
	f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
	profile := saveLocalSourceProfile(t, f.service, "security-owner", accountworkbench.SourceProfileReference{SourceOAuthID: source.ID})
	preview, err := f.service.PreviewSourceProfileAuthorization(context.Background(), "security-owner", accountworkbench.SourceProfileSelectionInput{Scope: accountworkbench.ScopeLocalExport, FreshLogin: true, Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteSourceProfile(context.Background(), "security-owner", profile.ID, accountworkbench.ScopeLocalExport, profile.Revision, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartOAuthBatch(context.Background(), "security-owner", preview.ID, true); err == nil {
		t.Fatal("deleted source profile started a new browser")
	}
}
