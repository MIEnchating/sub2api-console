package browserlogin_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func isolatedSecurityBrowser(t *testing.T, handler http.Handler, remote ...bool) (context.Context, browserlogin.SecurityBrowser) {
	t.Helper()
	return isolatedSecurityBrowserViaProxy(t, "", handler, remote...)
}

func isolatedSecurityBrowserViaProxy(t *testing.T, proxyURL string, handler http.Handler, remote ...bool) (context.Context, browserlogin.SecurityBrowser) {
	t.Helper()
	executable := os.Getenv("CONSOLE_TEST_CHROMIUM")
	if executable == "" || os.Getenv("CONSOLE_TEST_OAUTH_NETWORK_ISOLATED") != "1" {
		t.Skip("requires isolated Chromium network namespace")
	}
	current, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.Readlink("/proc/1/ns/net")
	if err != nil || current == root {
		t.Fatal("security browser test refuses host network namespace")
	}
	if err := exec.Command("ip", "link", "set", "dev", "lo", "up").Run(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	_ = server.Listener.Close()
	server.Listener, err = net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		t.Fatal(err)
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	spki := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	factory := browserlogin.Chromium{Executable: executable, CertificateSPKI: base64.StdEncoding.EncodeToString(spki[:]), Resolve: func(_ context.Context, host string) (string, error) {
		switch host {
		case "auth.openai.com", "chatgpt.com", "cdn.oaistatic.com", "images.openai.com", "cdn.auth0.com", "challenges.cloudflare.com", "static.cloudflareinsights.com", "proxy.example":
			return "127.0.0.1", nil
		}
		return "", errors.New("isolated test rejects DNS name")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	var securityFactory browserlogin.SecurityFactory = factory
	if len(remote) > 0 && remote[0] {
		socket := filepath.Join(t.TempDir(), "security.sock")
		workerCtx, workerCancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- browserlogin.RunWorker(workerCtx, socket, factory) }()
		t.Cleanup(func() {
			workerCancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(10 * time.Second):
				t.Error("security worker did not stop")
			}
		})
		for ctx.Err() == nil {
			conn, err := net.Dial("unix", socket)
			if err == nil {
				_ = conn.Close()
				break
			}
			runtime.Gosched()
		}
		securityFactory = browserlogin.NewRemote(socket)
	}
	browser, err := securityFactory.OpenSecurity(ctx, browserlogin.SecurityOptions{Email: "owner@example.com", ProxyURL: proxyURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(browser.Close)
	return ctx, browser
}

type securityHTTPFixture struct {
	password  atomic.Bool
	enabled   atomic.Bool
	enrolls   atomic.Int64
	activates atomic.Int64
	passwords atomic.Int64
	identity  atomic.Value
	t         *testing.T
}

func (f *securityHTTPFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/auth/csrf":
		_, _ = w.Write([]byte(`{"csrfToken":"isolated-csrf"}`))
	case "/api/auth/signin/openai":
		if r.URL.Query().Get("post_login_add_password") == "true" {
			f.password.Store(true)
			_, _ = w.Write([]byte(`{"url":"https://auth.openai.com/reset-password/new-password"}`))
			return
		}
		_, _ = w.Write([]byte(`{"url":"https://auth.openai.com/log-in"}`))
	case "/log-in":
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><button autofocus onclick="location.assign('https://chatgpt.com/')">Complete login</button></body></html>`))
	case "/reset-password/new-password":
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><form action="/api/accounts/password/add" onsubmit="event.preventDefault();fetch(this.action,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({password:this.password.value})}).then(r=>r.json()).then(()=>location.assign('https://chatgpt.com/'))"><input name="password" type="password"><input name="confirm" type="password"><button type="submit">Save password</button></form></body></html>`))
	case "/api/accounts/password/add":
		f.passwords.Add(1)
		var input struct {
			Password string `json:"password"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Password != "StrongPassword-2026!" {
			f.t.Error("unexpected password submission")
			http.Error(w, "rejected", 400)
			return
		}
		_, _ = w.Write([]byte(`{"page":{"type":"external_url"},"continue_url":"https://chatgpt.com/"}`))
	case "/api/auth/session":
		id := "user-stable"
		if v := f.identity.Load(); v != nil {
			id = v.(string)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]string{"id": id, "email": "owner@example.com"}, "accessToken": "private-web-token-should-never-leave-browser"})
	case "/backend-api/accounts/mfa_info":
		f.assertMFAHeaders(r)
		if f.enabled.Load() {
			_, _ = w.Write([]byte(`{"mfa_enabled_v2":true,"factors":{"totp":[{"id":"factor-1","factor_type":"totp"}]}}`))
		} else {
			_, _ = w.Write([]byte(`{"mfa_enabled_v2":false,"factors":{"totp":[]}}`))
		}
	case "/backend-api/accounts/mfa/enroll":
		f.assertMFAHeaders(r)
		f.enrolls.Add(1)
		_, _ = w.Write([]byte(`{"secret":"JBSWY3DPEHPK3PXP","session_id":"enrollment-1"}`))
	case "/backend-api/accounts/mfa/user/activate_enrollment":
		f.assertMFAHeaders(r)
		f.activates.Add(1)
		var input struct {
			Code      string `json:"code"`
			SessionID string `json:"session_id"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Code != "012345" || input.SessionID != "enrollment-1" {
			f.t.Error("activation did not preserve six-digit code or enrollment ID")
			http.Error(w, "bad", 400)
			return
		}
		f.enabled.Store(true)
		_, _ = w.Write([]byte(`{"success":true}`))
	case "/":
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>Private web account content must never be screenshotted</body></html>`))
	default:
		http.NotFound(w, r)
	}
}

func (f *securityHTTPFixture) assertMFAHeaders(r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer private-web-token-should-never-leave-browser" || r.Header.Get("X-Openai-Target-Path") != r.URL.Path {
		f.t.Error("MFA request missing official authentication or target headers")
	}
}

func awaitSecurityPage(t *testing.T, ctx context.Context, b browserlogin.SecurityBrowser, stage string) browserlogin.AuthPage {
	t.Helper()
	for ctx.Err() == nil {
		value, err := b.InspectAuth(ctx)
		if err == nil && value.Stage == stage {
			return value
		}
		runtime.Gosched()
	}
	t.Fatalf("security page did not reach %s", stage)
	return browserlogin.AuthPage{}
}

func completeSecurityLogin(t *testing.T, ctx context.Context, b browserlogin.SecurityBrowser) browserlogin.SecurityIdentity {
	t.Helper()
	// Wait for a rendered login page before clicking, as the console does.
	// Document readiness can precede Chromium painting the button.
	for ctx.Err() == nil {
		image, err := b.Screenshot(ctx)
		if err != nil || len(image) == 0 {
			runtime.Gosched()
			continue
		}
		if err := b.Input(ctx, browserlogin.Input{Kind: "click", X: 60, Y: 18}); err == nil {
			break
		}
		runtime.Gosched()
	}
	for ctx.Err() == nil {
		identity, err := b.Identity(ctx)
		if err == nil {
			return identity
		}
		runtime.Gosched()
	}
	t.Fatal("official login did not complete")
	return browserlogin.SecurityIdentity{}
}

func TestIsolatedSecurityTOTPChecksIdentityAndDoesNotReplayEnrollment(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	ctx, b := isolatedSecurityBrowser(t, fixture)
	identity := completeSecurityLogin(t, ctx, b)
	if image, err := b.Screenshot(ctx); err != nil || len(image) != 0 {
		t.Fatal("private ChatGPT page exposed through screenshot")
	}
	if err := b.Input(ctx, browserlogin.Input{Kind: "key", Key: "Enter"}); !errors.Is(err, browserlogin.ErrAuthPageChanged) {
		t.Fatal("private ChatGPT page accepted user input")
	}
	if enabled, err := b.TOTPEnabled(ctx, identity); err != nil || enabled {
		t.Fatalf("initial status=%t %v", enabled, err)
	}
	wrong := identity
	wrong.UserID = "other-user"
	if _, err := b.EnrollTOTP(ctx, wrong); !errors.Is(err, browserlogin.ErrSecurityIdentity) || fixture.enrolls.Load() != 0 {
		t.Fatalf("mismatched identity enrolled: %v", err)
	}
	// Identity rejection is deliberately terminal for this operation; obtain a fresh
	// browser for the successful path in the next test.
}

func TestIsolatedSecurityTOTPActivatesOnceAndPreservesLeadingZeroCode(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	ctx, b := isolatedSecurityBrowser(t, fixture)
	identity := completeSecurityLogin(t, ctx, b)
	enrollment, err := b.EnrollTOTP(ctx, identity)
	if err != nil || enrollment.Secret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("enrollment missing: %v", err)
	}
	if _, err := b.EnrollTOTP(ctx, identity); !errors.Is(err, browserlogin.ErrSecurityUncertain) || fixture.enrolls.Load() != 1 {
		t.Fatal("enrollment replayed")
	}
	if err := b.ActivateTOTP(ctx, identity, enrollment, "012345"); err != nil {
		t.Fatal(err)
	}
	if enabled, err := b.TOTPEnabled(ctx, identity); err != nil || !enabled {
		t.Fatalf("activation not verified: %v", err)
	}
	if err := b.ActivateTOTP(ctx, identity, enrollment, "012345"); !errors.Is(err, browserlogin.ErrSecurityUncertain) || fixture.activates.Load() != 1 {
		t.Fatal("activation replayed")
	}
}

func TestIsolatedSecurityPasswordUsesConfirmedValueAndVerifiesOfficialResponse(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	ctx, b := isolatedSecurityBrowser(t, fixture, true)
	identity := completeSecurityLogin(t, ctx, b)
	if err := b.BeginPassword(ctx, identity); err != nil {
		t.Fatal(err)
	}
	form := awaitSecurityPage(t, ctx, b, "new_password")
	if err := b.Input(ctx, browserlogin.Input{Kind: "text", Text: "unconfirmed-password"}); !errors.Is(err, browserlogin.ErrAuthPageChanged) {
		t.Fatal("new password could be replaced through unconfirmed manual input")
	}
	action := browserlogin.AuthAction{Stage: form.Stage, Revision: form.Revision, Value: "StrongPassword-2026!"}
	if err := b.SubmitPassword(ctx, action); err != nil {
		t.Fatal(err)
	}
	lastIdentity := ""
	for ctx.Err() == nil {
		if identity, err := b.Identity(ctx); err == nil {
			lastIdentity = identity.UserID
		}
		success, err := b.PasswordResult(ctx)
		if err == nil && success {
			break
		}
		if !errors.Is(err, browserlogin.ErrSecurityPending) {
			t.Fatalf("password result: %v (requests=%d last_identity=%s)", err, fixture.passwords.Load(), lastIdentity)
		}
		runtime.Gosched()
	}
	if ctx.Err() != nil {
		t.Fatal("password result was not verified")
	}
	if err := b.SubmitPassword(ctx, action); !errors.Is(err, browserlogin.ErrSecurityUncertain) || fixture.passwords.Load() != 1 {
		t.Fatal("password write replayed")
	}
}

func TestSecurityOptionsRejectsInvalidIdentity(t *testing.T) {
	for _, email := range []string{"", "Owner <owner@example.com>", " owner@example.com", strings.Repeat("x", 321) + "@example.com"} {
		if err := (browserlogin.SecurityOptions{Email: email}).Validate(); err == nil {
			t.Fatal("invalid email accepted")
		}
	}
}
