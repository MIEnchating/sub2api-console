package browserlogin_test

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestIsolatedSecurityCheckpointPasswordReauthenticationKeepsConfirmedEmail(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	var loginHint atomic.Value
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/authorize" || r.URL.Path == "/phone-verification" {
			checkpointLoginPage(w, r)
			return
		}
		if r.URL.Path == "/api/auth/signin/openai" && r.URL.Query().Get("post_login_add_password") == "true" {
			loginHint.Store(r.URL.Query().Get("login_hint"))
		}
		fixture.ServeHTTP(w, r)
	}))
	options := automaticCheckpointOptions()
	oauth, err := factory.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oauth.Close)
	meta := awaitAutomaticCheckpoint(t, ctx, factory, options, 1)
	oauth.(browserlogin.OAuthCheckpointPreserver).ClosePreservingOAuthCheckpoint()
	browser, err := factory.OpenSecurityFromCheckpoint(ctx, browserlogin.SecurityCheckpointOptions{
		Checkpoint: meta,
		SessionID:  strings.Repeat("f", 48),
		Options:    options,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(browser.Close)
	identity, err := browser.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := browser.(browserlogin.SecurityIdentityConfirmation).ConfirmSecurityIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if err := browser.BeginPassword(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if hint, _ := loginHint.Load().(string); hint != identity.Email {
		t.Fatalf("password reauthentication login hint = %q, want confirmed email %q", hint, identity.Email)
	}
}
