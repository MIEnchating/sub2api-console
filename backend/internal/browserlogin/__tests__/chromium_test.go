package browserlogin_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestIsolatedBrowserReadsSub2APILoginAndPreservesUserAgent(t *testing.T) {
	executable := os.Getenv("CONSOLE_TEST_CHROMIUM")
	if executable == "" {
		t.Skip("set CONSOLE_TEST_CHROMIUM for isolated real-browser integration")
	}
	if _, err := exec.LookPath("Xvfb"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><label>Name<input autofocus id="name"></label><button id="login" onclick="localStorage.setItem('auth_token',document.getElementById('name').value);localStorage.setItem('refresh_token','isolated-refresh');document.cookie='sub2api_refresh_token=isolated-refresh; Secure; path=/';document.body.dataset.loggedin='true'">Login</button></body></html>`))
	}))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	_, port, _ := net.SplitHostPort(endpoint.Host)
	spki := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	factory := browserlogin.Chromium{Executable: executable, CertificateSPKI: base64.StdEncoding.EncodeToString(spki[:]), Resolve: func(_ context.Context, host string) (string, error) {
		if host != "login.example.test" && host != "challenges.cloudflare.com" {
			t.Errorf("unexpected host: %s", host)
		}
		return "127.0.0.1", nil
	}}
	base := "https://login.example.test:" + port
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	browser, err := factory.Open(ctx, configstore.AuthRecord{Host: "login.example.test", BaseURL: base, UpstreamType: "sub2api"})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	// A newly opened browser has no inherited credentials.
	if _, err = browser.Credentials(ctx); err == nil {
		t.Fatal("fresh browser already authenticated")
	}
	if err = browser.Input(ctx, browserlogin.Input{Kind: "text", Text: "isolated-access"}); err != nil {
		t.Fatal(err)
	}
	if err = browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	if err = browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	credentials, err := browser.Credentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.AccessToken == nil || *credentials.AccessToken != "isolated-access" || credentials.RefreshToken == nil || *credentials.RefreshToken != "isolated-refresh" {
		t.Fatalf("incorrect credentials (values withheld)")
	}
	if !strings.Contains(credentials.Headers["User-Agent"], "Chrome/") || credentials.Headers["Origin"] != base {
		t.Fatal("browser request identity missing")
	}
	if credentials.Cookies["sub2api_refresh_token"] != "isolated-refresh" {
		t.Fatal("refresh cookie missing")
	}
	image, err := browser.Screenshot(ctx)
	if err != nil || len(image) == 0 {
		t.Fatalf("screenshot unavailable: %v", err)
	}
	browser.Close()
	if _, err = browser.Screenshot(ctx); err == nil {
		t.Fatal("closed browser still accepts commands")
	}
}
func TestBrowserRejectsUntrustedDestinationBeforeLaunching(t *testing.T) {
	for _, base := range []string{"http://login.example", "file:///etc/passwd", "https://user:password@login.example", "https://login.example:8443"} {
		_, err := (browserlogin.Chromium{}).Open(context.Background(), configstore.AuthRecord{BaseURL: base})
		if err == nil {
			t.Fatalf("accepted invalid destination %s", base)
		}
	}
}
