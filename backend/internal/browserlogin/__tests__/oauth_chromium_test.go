package browserlogin_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func isolatedOAuthBrowser(t *testing.T, callbackQuery string, subframe bool, fixture ...string) (context.Context, browserlogin.OAuthBrowser, *atomic.Int64) {
	t.Helper()
	return isolatedOAuthBrowserViaProxy(t, "", callbackQuery, subframe, fixture...)
}

func isolatedOAuthBrowserViaProxy(t *testing.T, proxyURL, callbackQuery string, subframe bool, fixture ...string) (context.Context, browserlogin.OAuthBrowser, *atomic.Int64) {
	t.Helper()
	executable := os.Getenv("CONSOLE_TEST_CHROMIUM")
	if executable == "" || os.Getenv("CONSOLE_TEST_OAUTH_NETWORK_ISOLATED") != "1" {
		t.Skip("run with CONSOLE_TEST_CHROMIUM and CONSOLE_TEST_OAUTH_NETWORK_ISOLATED=1 inside unshare --net")
	}
	currentNS, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	rootNS, err := os.Readlink("/proc/1/ns/net")
	if err != nil || currentNS == rootNS {
		t.Fatal("OAuth browser test requires a separate network namespace")
	}
	if err := exec.Command("ip", "link", "set", "dev", "lo", "up").Run(); err != nil {
		t.Fatal(err)
	}
	callbackRequests := &atomic.Int64{}
	callbackListener, err := net.Listen("tcp", "127.0.0.1:1455")
	if err != nil {
		t.Fatal(err)
	}
	callbackServer := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callbackRequests.Add(1)
		http.Error(w, "callback must be intercepted", http.StatusInternalServerError)
	})}
	go func() { _ = callbackServer.Serve(callbackListener) }()
	t.Cleanup(func() { _ = callbackServer.Close() })
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(fixture) == 1 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(fixture[0]))
			return
		}
		if r.URL.Path != "/oauth/authorize" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		target, _ := json.Marshal("http://localhost:1455/auth/callback?" + callbackQuery)
		authorize := "location.assign(" + string(target) + ")"
		_, _ = fmt.Fprintf(w, `<html><body onkeydown="%s"><button autofocus style="display:block" onclick="%s">Authorize</button>`, html.EscapeString("if(event.key==='Enter'){"+authorize+"}"), html.EscapeString(authorize))
		if subframe {
			_, _ = w.Write([]byte(`<iframe src="http://localhost:1455/auth/callback?state=state-1&amp;code=subframe-code"></iframe>`))
		}
		_, _ = w.Write([]byte(`</body></html>`))
	}))
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
		default:
			return "", errors.New("isolated test rejected unexpected DNS name")
		}
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	options := validOAuthOptions("state-1")
	options.ProxyURL = proxyURL
	browser, err := factory.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(browser.Close)
	return ctx, browser, callbackRequests
}

func awaitOAuthCallback(t *testing.T, ctx context.Context, browser browserlogin.OAuthBrowser) (browserlogin.OAuthResult, error) {
	t.Helper()
	for {
		result, err := browser.AuthorizationCode(ctx)
		if !errors.Is(err, browserlogin.ErrOAuthPending) {
			return result, err
		}
		if ctx.Err() != nil {
			t.Fatal("OAuth callback did not finish")
		}
		runtime.Gosched()
	}
}

func TestIsolatedOAuthBrowserPendingLoginAllowsInputAndCapturesCallbackWithoutNetworking(t *testing.T) {
	ctx, browser, callbackRequests := isolatedOAuthBrowser(t, "state=state-1&code=isolated-code", true)
	if _, err := browser.AuthorizationCode(ctx); !errors.Is(err, browserlogin.ErrOAuthPending) {
		t.Fatalf("unfinished login or subframe produced an authorization code: %v", err)
	}
	if image, err := browser.Screenshot(ctx); err != nil || len(image) == 0 {
		t.Fatalf("pending OAuth login cannot render screenshot: %v", err)
	}
	if err := browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	result, err := awaitOAuthCallback(t, ctx, browser)
	if err != nil || result.Code != "isolated-code" || result.State != "state-1" {
		t.Fatalf("main-frame OAuth callback was not captured correctly: %v", err)
	}
	if callbackRequests.Load() != 0 {
		t.Fatal("OAuth callback escaped interception and reached localhost")
	}
	browser.Close()
	if _, err := browser.AuthorizationCode(context.Background()); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatalf("closed browser still exposes authorization code: %v", err)
	}
}

func TestIsolatedOAuthBrowserRejectsInvalidCallbackWithoutExposingCode(t *testing.T) {
	tests := []struct {
		name, query string
		expected    error
	}{
		{name: "state_mismatch", query: "state=wrong&code=private-code", expected: browserlogin.ErrOAuthState},
		{name: "duplicate_state", query: "state=state-1&state=wrong&code=private-code", expected: browserlogin.ErrOAuthState},
		{name: "duplicate_code", query: "state=state-1&code=private-code&code=other", expected: browserlogin.ErrOAuthRejected},
		{name: "upstream_rejection", query: "state=state-1&error=access_denied&error_description=private-detail", expected: browserlogin.ErrOAuthRejected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, browser, callbackRequests := isolatedOAuthBrowser(t, test.query, false)
			if _, err := browser.Screenshot(ctx); err != nil {
				t.Fatal(err)
			}
			if err := browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
				t.Fatal(err)
			}
			result, err := awaitOAuthCallback(t, ctx, browser)
			if !errors.Is(err, test.expected) || result != (browserlogin.OAuthResult{}) {
				t.Fatalf("invalid callback exposed code or lost typed error: %v", err)
			}
			if callbackRequests.Load() != 0 {
				t.Fatal("invalid OAuth callback reached localhost")
			}
		})
	}
}
