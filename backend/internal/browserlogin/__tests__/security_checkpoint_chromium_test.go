package browserlogin_test

import (
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestIsolatedSecurityCheckpointTransfersPrivateStateAndRequiresOfficialIdentityConfirmation(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	var authorizationRequests, oldPageRequests, syntheticRequests atomic.Int64
	stateRead := make(chan bool, 1)
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/authorize":
			authorizationRequests.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "private-transfer", Value: "cookie-secret", Secure: true, HttpOnly: true, Path: "/"})
			http.Redirect(w, r, "/phone-verification", http.StatusFound)
		case "/phone-verification":
			oldPageRequests.Add(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<input name="code"><script>localStorage.setItem("transfer","local-secret");sessionStorage.setItem("transfer","session-secret");</script>`)
		case "/__console_security_checkpoint":
			syntheticRequests.Add(1)
			http.Error(w, "bootstrap must remain worker-private", 500)
		case "/api/auth/session":
			if _, err := r.Cookie("web-session"); err != nil {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{}`)
				return
			}
			fixture.ServeHTTP(w, r)
		case "/log-in":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<button autofocus onclick="location.assign('https://chatgpt.com/complete')">Complete login</button><script>fetch('/transfer-state?ok='+(localStorage.getItem('transfer')==='local-secret'&&sessionStorage.getItem('transfer')==='session-secret'));</script>`)
		case "/transfer-state":
			cookie, err := r.Cookie("private-transfer")
			stateRead <- err == nil && cookie.Value == "cookie-secret" && r.URL.Query().Get("ok") == "true"
			w.WriteHeader(http.StatusNoContent)
		case "/complete":
			http.SetCookie(w, &http.Cookie{Name: "web-session", Value: "private-web-session", Secure: true, HttpOnly: true, Path: "/"})
			http.Redirect(w, r, "/", http.StatusFound)
		default:
			fixture.ServeHTTP(w, r)
		}
	}))
	remote, _ := startCheckpointWorker(t, factory)
	options := automaticCheckpointOptions()
	old, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	meta := awaitAutomaticCheckpoint(t, ctx, remote, options, 1)
	request := securityCheckpointRequest(options, meta)
	wrong := request
	wrong.Checkpoint.Stage = "workspace"
	if _, err := remote.OpenSecurityFromCheckpoint(ctx, wrong); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("wrong stage was accepted")
	}
	if _, err := old.Screenshot(ctx); err != nil {
		t.Fatal("rejected transfer closed the original browser")
	}
	browser, err := remote.OpenSecurityFromCheckpoint(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	select {
	case valid := <-stateRead:
		if !valid {
			t.Fatal("transferred official page lost private cookies or origin storage")
		}
	case <-ctx.Done():
		t.Fatal("transferred login did not read its state")
	}
	if _, err := old.AuthorizationCode(ctx); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("old OAuth callback remained usable")
	}
	if _, err := remote.ReadOAuthCheckpoint(ctx, automaticCheckpointRef(options)); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("transfer retained an executable OAuth checkpoint")
	}
	identity := completeSecurityLogin(t, ctx, browser)
	if identity.UserID != "user-stable" || identity.Email != "owner@example.com" {
		t.Fatal("security identity was not obtained from official session")
	}
	if err := browser.BeginPassword(ctx, identity); !errors.Is(err, browserlogin.ErrSecurityConfirmation) {
		t.Fatal("password operation started before identity confirmation")
	}
	if _, err := browser.EnrollTOTP(ctx, identity); !errors.Is(err, browserlogin.ErrSecurityConfirmation) {
		t.Fatal("TOTP enrollment started before identity confirmation")
	}
	if err := browser.ActivateTOTP(ctx, identity, browserlogin.SecurityEnrollment{}, "123456"); !errors.Is(err, browserlogin.ErrSecurityConfirmation) {
		t.Fatal("TOTP activation started before identity confirmation")
	}
	confirmation := browser.(browserlogin.SecurityIdentityConfirmation)
	wrongIdentity := identity
	wrongIdentity.UserID = "other-user"
	if err := confirmation.ConfirmSecurityIdentity(ctx, wrongIdentity); !errors.Is(err, browserlogin.ErrSecurityIdentity) {
		t.Fatal("wrong official user was accepted")
	}
	if err := confirmation.ConfirmSecurityIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.EnrollTOTP(ctx, identity); err != nil || fixture.enrolls.Load() != 1 {
		t.Fatalf("confirmed identity could not enroll: %v", err)
	}
	if authorizationRequests.Load() != 1 || oldPageRequests.Load() != 1 || syntheticRequests.Load() != 0 {
		t.Fatal("transfer revisited the original authorization or exposed its private bootstrap")
	}
	browser.Close()
	if _, err := remote.OpenSecurityFromCheckpoint(ctx, request); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("completed transfer replayed its original checkpoint")
	}
}

func TestIsolatedSecurityCheckpointUsesExistingWebIdentityWithoutStartingNewLogin(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	var logins atomic.Int64
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/authorize" || r.URL.Path == "/phone-verification" {
			checkpointLoginPage(w, r)
			return
		}
		if r.URL.Path == "/api/auth/signin/openai" {
			logins.Add(1)
		}
		fixture.ServeHTTP(w, r)
	}))
	remote, stop := startCheckpointWorker(t, factory)
	options := checkpointOptions()
	old, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	meta := suspendCheckpoint(t, ctx, old)
	stop()
	restarted, _ := startCheckpointWorker(t, factory)
	browser, err := restarted.OpenSecurityFromCheckpoint(ctx, securityCheckpointRequest(options, meta))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	identity, err := browser.Identity(ctx)
	if err != nil || identity.UserID != "user-stable" || logins.Load() != 0 {
		t.Fatal("existing official identity was lost or a new login was automatically started")
	}
	if image, err := browser.Screenshot(ctx); err != nil || len(image) != 0 {
		t.Fatal("private logged-in page was exposed")
	}
	if err := browser.(browserlogin.SecurityIdentityConfirmation).ConfirmSecurityIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}
	fixture.identity.Store("different-user")
	if _, err := browser.EnrollTOTP(ctx, identity); !errors.Is(err, browserlogin.ErrSecurityIdentity) || fixture.enrolls.Load() != 0 {
		t.Fatal("identity change after confirmation reached a security write")
	}
}

func TestIsolatedSecurityCheckpointBlocksPageEnrollmentBeforeConfirmation(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	pageAttempt := make(chan struct{}, 1)
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/authorize", "/phone-verification":
			checkpointLoginPage(w, r)
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<script>fetch('/backend-api/accounts/mfa/enroll',{method:'POST',body:'{}'}).catch(()=>{}).finally(()=>fetch('/attempt-finished'))</script>`)
		case "/attempt-finished":
			pageAttempt <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		default:
			fixture.ServeHTTP(w, r)
		}
	}))
	remote, _ := startCheckpointWorker(t, factory)
	options := checkpointOptions()
	oauth, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	meta := suspendCheckpoint(t, ctx, oauth)
	browser, err := remote.OpenSecurityFromCheckpoint(ctx, securityCheckpointRequest(options, meta))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	select {
	case <-pageAttempt:
	case <-ctx.Done():
		t.Fatal("official page did not finish its attempted enrollment")
	}
	if fixture.enrolls.Load() != 0 {
		t.Fatal("page enrollment reached official endpoint without identity confirmation")
	}
}
