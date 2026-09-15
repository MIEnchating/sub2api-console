package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestSecurityCheckpointStartsFreshOAuthOnceWithOriginalProxyAndSMSOrigin(t *testing.T) {
	f, browser, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
	view := startCheckpointSecurity(t, f, checkpoint, "totp")
	f.phase(t, "awaiting_confirmation")
	if _, err := f.service.OAuthAfterSecurity(context.Background(), "checkpoint-owner", view.ID, true); err == nil {
		t.Fatal("unconfirmed security started new OAuth")
	}
	confirmCheckpointIdentity(t, f, view.ID)
	if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "succeeded")
	restarted, err := f.service.OAuthAfterSecurity(context.Background(), "checkpoint-owner", view.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, restarted.ID)
	f.phase(t, "waiting")
	repeated, err := f.service.OAuthAfterSecurity(context.Background(), "checkpoint-owner", view.ID, true)
	if err != nil || repeated.ID != restarted.ID {
		t.Fatalf("restart did not reconnect existing transaction: %+v %v", repeated, err)
	}
	f.factory.mu.Lock()
	fresh := f.factory.original
	f.factory.mu.Unlock()
	browser.stateMu.Lock()
	original := browser.options.Options
	browser.stateMu.Unlock()
	originalURL, _ := url.Parse(original.AuthorizationURL)
	freshURL, _ := url.Parse(fresh.AuthorizationURL)
	if fresh.State == original.State || freshURL.Query().Get("code_challenge") == originalURL.Query().Get("code_challenge") || fresh.ProxyURL != original.ProxyURL || restarted.Scope != checkpoint.Scope {
		t.Fatal("new authorization reused transaction state or changed scope/proxy")
	}
	if f.exchanges.Load() != 0 {
		t.Fatal("security transfer exchanged an old authorization code")
	}
	var smsOrigin string
	f.state.onSaved = func(record configstore.WorkbenchOAuthCheckpoint) {
		if record.SourceTaskID != restarted.ID {
			return
		}
		persisted, err := f.private.WorkbenchOAuthCheckpoint(context.Background(), record.OwnerHash, record.TargetFingerprint, record.ID)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			SMSOriginTaskID string `json:"sms_origin_task_id"`
		}
		if json.Unmarshal(persisted.Payload, &payload) == nil {
			smsOrigin = payload.SMSOriginTaskID
		}
	}
	if _, err := f.service.SaveOAuthCheckpoint(context.Background(), "checkpoint-owner", restarted.ID, true); err != nil {
		t.Fatal(err)
	}
	if smsOrigin != checkpoint.SourceTaskID {
		t.Fatal("fresh authorization lost original SMS order ownership")
	}
}

func TestSecurityCheckpointFreshOAuthPreservesConfirmedOfficialIdentity(t *testing.T) {
	f, _, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
	view := startCheckpointSecurity(t, f, checkpoint, "totp")
	f.phase(t, "awaiting_confirmation")
	confirmCheckpointIdentity(t, f, view.ID)
	if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "succeeded")
	restarted, err := f.service.OAuthAfterSecurity(context.Background(), "checkpoint-owner", view.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, restarted.ID)
	f.phase(t, "waiting")
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", restarted.ID, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", restarted.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "authorized")
	source, err := f.service.SecuritySource(context.Background(), "checkpoint-owner", restarted.ID)
	if err != nil || source.UserID != "user-1" {
		t.Fatalf("fresh result lost confirmed identity: %+v %v", source, err)
	}
}

func TestSecurityCheckpointFreshOAuthRejectsDifferentOfficialUser(t *testing.T) {
	f, _, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
	view := startCheckpointSecurity(t, f, checkpoint, "totp")
	f.phase(t, "awaiting_confirmation")
	confirmCheckpointIdentity(t, f, view.ID)
	if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "succeeded")
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		claims := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"owner@example.com","https://api.openai.com/auth":{"chatgpt_user_id":"another-user","chatgpt_account_id":"workspace-1"}}`))
		raw, _ := json.Marshal(map[string]any{"access_token": "e30." + claims + ".test-signature", "refresh_token": "rt_wrong_user", "token_type": "Bearer"})
		return oauthResponse(string(raw)), nil
	}))
	restarted, err := f.service.OAuthAfterSecurity(context.Background(), "checkpoint-owner", view.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, restarted.ID)
	f.phase(t, "waiting")
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", restarted.ID, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", restarted.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "failed")
	if _, err := f.service.PreviewOAuth(context.Background(), "checkpoint-owner", restarted.ID, accountworkbench.OAuthPreviewInput{Scope: checkpoint.Scope, ExportOnly: true}); err == nil {
		t.Fatal("new authorization exported a different official user")
	}
}
