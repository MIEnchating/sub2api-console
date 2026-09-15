package browserlogin_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func isolatedCheckpointFactory(t *testing.T, handler http.Handler) (context.Context, browserlogin.Chromium) {
	t.Helper()
	executable := os.Getenv("CONSOLE_TEST_CHROMIUM")
	if executable == "" || os.Getenv("CONSOLE_TEST_OAUTH_NETWORK_ISOLATED") != "1" {
		t.Skip("requires isolated network namespace and CONSOLE_TEST_CHROMIUM")
	}
	currentNS, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	rootNS, err := os.Readlink("/proc/1/ns/net")
	if err != nil || currentNS == rootNS {
		t.Fatal("checkpoint tests must not use the production network namespace")
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
	factory := browserlogin.Chromium{Executable: executable, CertificateSPKI: base64.StdEncoding.EncodeToString(spki[:]), CheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"), Resolve: func(_ context.Context, host string) (string, error) {
		switch host {
		case "auth.openai.com", "chatgpt.com", "cdn.oaistatic.com", "images.openai.com", "cdn.auth0.com", "challenges.cloudflare.com", "static.cloudflareinsights.com":
			return "127.0.0.1", nil
		}
		return "", errors.New("isolated checkpoint test rejected unexpected host")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	return ctx, factory
}

func startCheckpointWorker(t *testing.T, factory browserlogin.Chromium) (*browserlogin.Remote, func()) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "worker.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- browserlogin.RunWorker(ctx, socket, factory) }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		connection, err := net.Dial("unix", socket)
		if err == nil {
			_ = connection.Close()
			break
		}
		select {
		case err := <-done:
			t.Fatalf("worker failed to start: %v", err)
		case <-deadline.C:
			t.Fatal("worker socket was not created")
		default:
			runtime.Gosched()
		}
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("worker shutdown: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Error("worker shutdown did not finish")
			}
		})
	}
	t.Cleanup(stop)
	return browserlogin.NewRemote(socket), stop
}

func suspendCheckpoint(t *testing.T, ctx context.Context, browser browserlogin.OAuthBrowser) browserlogin.OAuthCheckpoint {
	t.Helper()
	for {
		meta, err := browser.(browserlogin.OAuthCheckpointBrowser).SuspendOAuth(ctx)
		if err == nil {
			return meta
		}
		if !errors.Is(err, browserlogin.ErrOAuthCheckpointUnsafe) || ctx.Err() != nil {
			t.Fatalf("safe page checkpoint did not finish: %v", err)
		}
		runtime.Gosched()
	}
}

func restoredCheckpointOptions(options browserlogin.OAuthOptions, meta browserlogin.OAuthCheckpoint) browserlogin.OAuthRestoreOptions {
	binding := *options.Recovery
	binding.Lease = strings.Repeat("c", 64)
	options.Recovery = &binding
	return browserlogin.OAuthRestoreOptions{Checkpoint: browserlogin.OAuthCheckpointRef{ID: meta.ID, Owner: meta.Owner, Lease: meta.Lease}, Lease: binding.Lease, Options: options}
}

func TestIsolatedOAuthCheckpointRestoresPrivateStateAfterWorkerRestartWithoutAutomaticWrites(t *testing.T) {
	var pageLoads, postRequests atomic.Int64
	restored := make(chan bool, 1)
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			postRequests.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.URL.Path {
		case "/oauth/authorize":
			http.SetCookie(w, &http.Cookie{Name: "private-login", Value: "cookie-checkpoint-secret", Path: "/", Secure: true, HttpOnly: true})
			http.Redirect(w, r, "/phone-verification", http.StatusFound)
		case "/phone-verification":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<form method="post" action="/verify"><label>验证码<input name="code"></label><button type="submit">验证</button></form>`)
			if pageLoads.Add(1) == 1 {
				_, _ = fmt.Fprint(w, `<script>localStorage.setItem("private-local","local-checkpoint-secret");sessionStorage.setItem("private-session","session-checkpoint-secret");</script>`)
			} else {
				_, _ = fmt.Fprint(w, `<script>fetch("/automatic-write",{method:"POST"}).catch(()=>{}).finally(()=>fetch("/restore-check?ok="+(localStorage.getItem("private-local")==="local-checkpoint-secret"&&sessionStorage.getItem("private-session")==="session-checkpoint-secret")));</script>`)
			}
		case "/restore-check":
			cookie, err := r.Cookie("private-login")
			restored <- err == nil && cookie.Value == "cookie-checkpoint-secret" && r.URL.Query().Get("ok") == "true"
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	remote, stop := startCheckpointWorker(t, factory)
	options := checkpointOptions()
	browser, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	meta := suspendCheckpoint(t, ctx, browser)
	if meta.Stage != "sms_code" || !meta.ExpiresAt.Equal(options.Recovery.ExpiresAt) {
		t.Fatal("checkpoint stage or original expiration changed")
	}
	encoded, err := json.Marshal(meta)
	if err != nil || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "cookies") || strings.Contains(string(encoded), "phone-verification") {
		t.Fatal("private state crossed checkpoint metadata boundary")
	}
	info, err := os.Stat(filepath.Join(factory.CheckpointDirectory, meta.ID+".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("checkpoint was not durably saved with private permissions")
	}
	stop()
	restarted, _ := startCheckpointWorker(t, factory)
	request := restoredCheckpointOptions(options, meta)
	wrong := request
	wrong.Checkpoint.Owner = strings.Repeat("d", 64)
	if _, err := restarted.RestoreOAuth(ctx, wrong); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("different owner recovered checkpoint")
	}
	for _, changed := range []string{"expiry", "state", "challenge", "proxy", "lease"} {
		candidate := restoredCheckpointOptions(options, meta)
		parsed, _ := url.Parse(candidate.Options.AuthorizationURL)
		query := parsed.Query()
		switch changed {
		case "expiry":
			candidate.Options.Recovery.ExpiresAt = candidate.Options.Recovery.ExpiresAt.Add(time.Minute)
		case "state":
			candidate.Options.State = "another-state"
			query.Set("state", candidate.Options.State)
		case "challenge":
			digest := sha256.Sum256([]byte("different verifier"))
			query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(digest[:]))
		case "proxy":
			candidate.Options.ProxyURL = "http://proxy.example:8080"
		case "lease":
			candidate.Checkpoint.Lease = strings.Repeat("d", 64)
		}
		parsed.RawQuery = query.Encode()
		candidate.Options.AuthorizationURL = parsed.String()
		if _, err := restarted.RestoreOAuth(ctx, candidate); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
			t.Fatalf("changed %s binding was accepted: %v", changed, err)
		}
		if _, err := os.Stat(filepath.Join(factory.CheckpointDirectory, meta.ID+".json")); err != nil {
			t.Fatalf("rejected %s binding consumed another session's checkpoint", changed)
		}
	}
	recovered, err := restarted.RestoreOAuth(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	frame, err := recovered.Screenshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(frame))
	if err != nil || decoded.Bounds().Dx() != browserlogin.Width || decoded.Bounds().Dy() != browserlogin.Height {
		t.Fatal("restored browser screenshot is not a valid fixed-size frame")
	}
	if directory := os.Getenv("CONSOLE_TEST_ARTIFACT_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "oauth-checkpoint-restored.jpg"), frame, 0600); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case valid := <-restored:
		if !valid {
			t.Fatal("private cookie and page storage were not restored")
		}
	case <-ctx.Done():
		t.Fatal("restored browser did not verify private state")
	}
	if postRequests.Load() != 0 {
		t.Fatal("restoring page replayed an automatic write")
	}
	automation := recovered.(browserlogin.OAuthAutomation)
	authPage, err := automation.InspectAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := automation.ApplyAuth(ctx, browserlogin.AuthAction{Stage: authPage.Stage, Revision: authPage.Revision, Value: "123456"}); !errors.Is(err, browserlogin.ErrOAuthRecoveryPaused) {
		t.Fatal("automatic verification was allowed before manual takeover")
	}
	if err := recovered.Input(ctx, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(factory.CheckpointDirectory, meta.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("claimed checkpoint retained reusable credentials")
	}
	recovered.Close()
	if _, err := restarted.RestoreOAuth(ctx, request); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("consumed checkpoint lease was replayed")
	}
}

func TestIsolatedOAuthCheckpointAllowsOnlyOneConcurrentLeaseClaim(t *testing.T) {
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/authorize" {
			http.Redirect(w, r, "/add-phone", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<form method="post"><input type="tel"><button>继续</button></form>`)
	}))
	options := checkpointOptions()
	browser, err := factory.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	meta := suspendCheckpoint(t, ctx, browser)
	type claim struct {
		browser browserlogin.OAuthBrowser
		err     error
	}
	results := make(chan claim, 2)
	ready := make(chan struct{})
	for range 2 {
		go func() {
			<-ready
			value, err := factory.RestoreOAuth(ctx, restoredCheckpointOptions(options, meta))
			results <- claim{browser: value, err: err}
		}()
	}
	close(ready)
	succeeded := 0
	for range 2 {
		select {
		case value := <-results:
			if value.err == nil {
				succeeded++
				value.browser.Close()
			} else if !errors.Is(value.err, browserlogin.ErrOAuthCheckpoint) {
				t.Fatal(value.err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent checkpoint claim did not finish")
		}
	}
	if succeeded != 1 {
		t.Fatalf("expected one checkpoint lease winner, got %d", succeeded)
	}
}

func TestIsolatedOAuthCheckpointRejectsCallbackPostAndUnapprovedNavigation(t *testing.T) {
	for _, scenario := range []string{"callback", "post", "query", "unknown_path"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if r.URL.Path == "/oauth/authorize" {
					switch scenario {
					case "callback":
						http.Redirect(w, r, "http://localhost:1455/auth/callback?state=checkpoint-state&code=one-use-code", http.StatusFound)
					case "post":
						_, _ = fmt.Fprint(w, `<form id="p" action="/phone-verification" method="post"></form><script>document.getElementById("p").submit();</script>`)
					case "query":
						http.Redirect(w, r, "/phone-verification?continue=unsafe", http.StatusFound)
					default:
						http.Redirect(w, r, "/unapproved", http.StatusFound)
					}
					return
				}
				_, _ = fmt.Fprint(w, `<form><input name="code"><button>继续</button></form>`)
			}))
			browser, err := factory.OpenOAuth(ctx, checkpointOptions())
			if err != nil {
				t.Fatal(err)
			}
			defer browser.Close()
			if scenario == "callback" {
				for {
					_, err := browser.AuthorizationCode(ctx)
					if err == nil {
						break
					}
					if !errors.Is(err, browserlogin.ErrOAuthPending) || ctx.Err() != nil {
						t.Fatal("fixture callback was not captured")
					}
					runtime.Gosched()
				}
			}
			if scenario == "post" {
				for {
					page, err := browser.(browserlogin.OAuthAutomation).InspectAuth(ctx)
					if err == nil && page.Stage == "sms_code" {
						break
					}
					if ctx.Err() != nil {
						t.Fatal("POST response did not render the verification form")
					}
					runtime.Gosched()
				}
			}
			if _, err := browser.(browserlogin.OAuthCheckpointBrowser).SuspendOAuth(ctx); !errors.Is(err, browserlogin.ErrOAuthCheckpointUnsafe) {
				t.Fatalf("unsafe state allowed checkpoint: %v", err)
			}
		})
	}
}
