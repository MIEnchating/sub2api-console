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
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type challengeBrowser struct {
	browserFixture
	code string
}

func (b *challengeBrowser) ChallengeCode() string { return b.code }

func TestChallengeCodeCrossesPrivateWorkerWithoutPageConsoleContent(t *testing.T) {
	for _, code := range []string{"600010", "private-page-console-content"} {
		t.Run(code, func(t *testing.T) {
			browser := &challengeBrowser{browserFixture: browserFixture{closed: make(chan struct{})}, code: code}
			manager, id := interactionManager(t, browser)
			view, err := manager.Read(context.Background(), "owner", id)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if code == "600010" && result["challenge_code"] != code {
				t.Fatal("Cloudflare error code was lost before reaching the console")
			}
			if code != "600010" && result["challenge_code"] != nil {
				t.Fatal("arbitrary page content crossed the public screenshot boundary")
			}
		})
	}
}

func TestIsolatedLoginReportsChallengeCodeAndReloadsWithoutReplayingLogin(t *testing.T) {
	executable := os.Getenv("CONSOLE_TEST_CHROMIUM")
	if executable == "" {
		t.Skip("set CONSOLE_TEST_CHROMIUM for isolated real-browser integration")
	}
	var logins atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			t.Error("reload replayed a login write")
			http.Error(w, "method", 405)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		if logins.Add(1) == 1 {
			_, _ = w.Write([]byte(`<html><body><script>console.error('[Cloudflare Turnstile] Error: 600010.');console.error('private-page-console-content')</script>Verification failed</body></html>`))
		} else {
			_, _ = w.Write([]byte(`<html><body>New verification page</body></html>`))
		}
	}))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	_, port, _ := net.SplitHostPort(endpoint.Host)
	spki := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	factory := browserlogin.Chromium{Executable: executable, CertificateSPKI: base64.StdEncoding.EncodeToString(spki[:]), Resolve: func(_ context.Context, host string) (string, error) {
		if host != "login.example.test" && host != "challenges.cloudflare.com" {
			return "", errors.New("isolated test rejected external hostname")
		}
		return "127.0.0.1", nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	browser, err := factory.Open(ctx, configstore.AuthRecord{Host: "login.example.test", BaseURL: "https://login.example.test:" + port})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	reporter, ok := browser.(interface{ ChallengeCode() string })
	if !ok || reporter.ChallengeCode() != "600010" {
		t.Fatal("browser did not report the specific challenge failure")
	}
	if err := browser.Input(ctx, browserlogin.Input{Kind: "reload"}); err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 2 || reporter.ChallengeCode() != "" {
		t.Fatal("reload did not clear the old challenge and load a fresh login page")
	}
}
