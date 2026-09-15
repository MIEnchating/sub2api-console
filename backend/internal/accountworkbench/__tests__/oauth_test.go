package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type oauthTestBrowser struct {
	mu           sync.Mutex
	options      browserlogin.OAuthOptions
	pending      bool
	wrongState   bool
	closed       chan struct{}
	closeRelease <-chan struct{}
	once         sync.Once
}

func (b *oauthTestBrowser) OpenOAuth(_ context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.options = options
	b.mu.Unlock()
	return b, nil
}
func (b *oauthTestBrowser) Screenshot(context.Context) ([]byte, error) {
	return []byte("test-frame"), nil
}
func (b *oauthTestBrowser) Input(context.Context, browserlogin.Input) error { return nil }
func (b *oauthTestBrowser) Close() {
	b.once.Do(func() {
		close(b.closed)
		if b.closeRelease != nil {
			<-b.closeRelease
		}
	})
}
func (b *oauthTestBrowser) AuthorizationCode(context.Context) (browserlogin.OAuthResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending {
		b.pending = false
		return browserlogin.OAuthResult{}, browserlogin.ErrOAuthPending
	}
	state := b.options.State
	if b.wrongState {
		state = "different-private-state"
	}
	return browserlogin.OAuthResult{Code: "private-auth-code", State: state}, nil
}

type oauthTestTasks struct {
	*taskstore.Store
	events chan taskstore.Task
}

func (s *oauthTestTasks) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.Store.Save(ctx, task); err != nil {
		return err
	}
	s.events <- task
	return nil
}

type oauthTransportFunc func(*http.Request) (*http.Response, error)

func (f oauthTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type oauthFixture struct {
	*importFixture
	browser *oauthTestBrowser
	events  chan taskstore.Task
	calls   atomic.Int32
	view    accountworkbench.OAuthView
}

func newOAuthFixture(t *testing.T) *oauthFixture {
	t.Helper()
	f := &oauthFixture{importFixture: newImportFixture(t, nil), browser: &oauthTestBrowser{closed: make(chan struct{})}, events: make(chan taskstore.Task, 30)}
	f.service = accountworkbench.New(f.private, &oauthTestTasks{Store: f.tasks.Store, events: f.events}, f.business, nil, f.runner)
	f.service.UseOAuthBrowser(f.browser)
	f.service.UseOAuthTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		f.calls.Add(1)
		if request.URL.String() != "https://auth.openai.com/oauth/token" || request.Method != http.MethodPost {
			t.Error("authorization exchange used unexpected destination")
		}
		if request.Header.Get("x-api-key") != "" || request.Header.Get("Authorization") != "" {
			t.Error("management credentials reached official OAuth endpoint")
		}
		raw, _ := io.ReadAll(request.Body)
		form, err := url.ParseQuery(string(raw))
		if err != nil || form.Get("code") != "private-auth-code" || form.Get("grant_type") != "authorization_code" {
			t.Error("invalid authorization exchange")
		}
		f.browser.mu.Lock()
		options := f.browser.options
		f.browser.mu.Unlock()
		authorization, _ := url.Parse(options.AuthorizationURL)
		digest := sha256.Sum256([]byte(form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(digest[:]) != authorization.Query().Get("code_challenge") {
			t.Error("PKCE verifier does not match challenge")
		}
		if form.Get("redirect_uri") != options.RedirectURI || form.Get("client_id") != authorization.Query().Get("client_id") {
			t.Error("authorization transaction changed identity")
		}
		return oauthResponse(`{"access_token":"private-access","refresh_token":"rt_private_refresh","id_token":"","token_type":"Bearer","expires_in":3600}`), nil
	}))
	t.Cleanup(func() {
		if f.view.ID != "" {
			_ = f.service.CancelOAuth("owner", f.view.ID)
		}
	})
	return f
}
func oauthResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func (f *oauthFixture) awaitPhase(t *testing.T, phase string) taskstore.Task {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case task := <-f.events:
			if task.Result["phase"] == phase {
				return task
			}
		case <-deadline.C:
			t.Fatalf("OAuth task did not reach %s", phase)
			return taskstore.Task{}
		}
	}
}
func (f *oauthFixture) startOAuth(t *testing.T) {
	t.Helper()
	var err error
	f.view, err = f.service.StartOAuth(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "waiting")
}
func (f *oauthFixture) finishOAuth(t *testing.T) taskstore.Task {
	t.Helper()
	if err := f.service.FinishOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	return f.awaitPhase(t, "authorized")
}

func TestOAuthAuthorizationExchangesPKCEOnceAndReturnsOnlyImportPreview(t *testing.T) {
	f := newOAuthFixture(t)
	f.startOAuth(t)
	terminal := f.finishOAuth(t)
	view, err := f.service.ReadOAuth(context.Background(), "owner", f.view.ID)
	if err != nil || view.Status != "authorized" {
		t.Fatalf("view=%s err=%v", view.Status, err)
	}
	preview, err := f.service.PreviewOAuth(context.Background(), "owner", f.view.ID, accountworkbench.OAuthPreviewInput{})
	if err != nil || len(preview.Items) != 1 || preview.ID == "" {
		t.Fatalf("preview not available: %v", err)
	}
	for _, value := range []any{view, terminal, preview} {
		raw, _ := json.Marshal(value)
		for _, secret := range []string{"private-access", "rt_private_refresh", "private-auth-code", "code_verifier"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("OAuth response or task exposed a credential")
			}
		}
	}
	if err := f.service.FinishOAuth("owner", f.view.ID); err == nil {
		t.Fatal("completed exchange was accepted again")
	}
	if f.calls.Load() != 1 {
		t.Fatal("authorization code was exchanged multiple times")
	}
	if err := f.service.CancelOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Import(context.Background(), "owner", preview.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatal("cancelled authorization retained executable preview")
	}
}

func TestOAuthEarlyFinishKeepsBrowserInteractiveWithoutExchangingCode(t *testing.T) {
	f := newOAuthFixture(t)
	f.browser.pending = true
	f.startOAuth(t)
	if err := f.service.FinishOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "waiting")
	if f.calls.Load() != 0 {
		t.Fatal("premature finish exchanged a code")
	}
	if err := f.service.InputOAuth(context.Background(), "owner", f.view.ID, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	f.finishOAuth(t)
}

func TestOAuthNextAuthorizationWaitsUntilPreviousBrowserCloses(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "authorized", false: "cancelled"}[complete], func(t *testing.T) {
			f := newOAuthFixture(t)
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			f.browser.closeRelease = release
			f.startOAuth(t)
			if complete {
				f.finishOAuth(t)
			} else if err := f.service.CancelOAuth("owner", f.view.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-f.browser.closed:
			case <-time.After(5 * time.Second):
				t.Fatal("browser cleanup did not start")
			}
			if _, err := f.service.StartOAuth(context.Background(), "next-owner"); err == nil {
				t.Fatal("next authorization was enqueued before the previous browser closed")
			}
		})
	}
}

func TestOAuthForeignSessionCannotReadInputFinishPreviewOrCancel(t *testing.T) {
	f := newOAuthFixture(t)
	f.startOAuth(t)
	_, readErr := f.service.ReadOAuth(context.Background(), "other", f.view.ID)
	_, previewErr := f.service.PreviewOAuth(context.Background(), "other", f.view.ID, accountworkbench.OAuthPreviewInput{})
	for _, err := range []error{readErr, previewErr, f.service.InputOAuth(context.Background(), "other", f.view.ID, browserlogin.Input{Kind: "key", Key: "Enter"}), f.service.FinishOAuth("other", f.view.ID), f.service.CancelOAuth("other", f.view.ID)} {
		if !errors.Is(err, browserlogin.ErrSession) {
			t.Fatalf("foreign session operation returned %v", err)
		}
	}
}

func TestOAuthInvalidStateFailsBeforeTokenExchange(t *testing.T) {
	f := newOAuthFixture(t)
	f.browser.wrongState = true
	f.startOAuth(t)
	if err := f.service.FinishOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitPhase(t, "failed")
	if f.calls.Load() != 0 {
		t.Fatal("mismatched OAuth state reached token endpoint")
	}
	if _, err := f.service.PreviewOAuth(context.Background(), "owner", f.view.ID, accountworkbench.OAuthPreviewInput{}); err == nil {
		t.Fatal("failed authorization produced a preview")
	}
}

func TestOAuthTargetChangeFailsBeforeExchangeAndBlocksAuthorizedPreview(t *testing.T) {
	for _, authorized := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting", true: "authorized"}[authorized], func(t *testing.T) {
			f := newOAuthFixture(t)
			f.startOAuth(t)
			if authorized {
				f.finishOAuth(t)
			}
			if err := f.private.ConfigureTarget(context.Background(), "http://127.0.0.1:1", "different-target-key", 3); err != nil {
				t.Fatal(err)
			}
			if !authorized {
				if err := f.service.FinishOAuth("owner", f.view.ID); err != nil {
					t.Fatal(err)
				}
				f.awaitPhase(t, "failed")
				if f.calls.Load() != 0 {
					t.Fatal("changed target exchanged credentials")
				}
			}
			if _, err := f.service.PreviewOAuth(context.Background(), "owner", f.view.ID, accountworkbench.OAuthPreviewInput{}); err == nil {
				t.Fatal("changed target accepted authorization preview")
			}
		})
	}
}

func TestOAuthCancellationDuringExchangeDiscardsCredentialsAndClosesBrowser(t *testing.T) {
	f := newOAuthFixture(t)
	started := make(chan struct{})
	f.service.UseOAuthTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	}))
	f.startOAuth(t)
	if err := f.service.FinishOAuth("owner", f.view.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("exchange did not start")
	}
	if !f.runner.CancelTask(f.view.TaskID) {
		t.Fatal("OAuth task was not cancellable")
	}
	f.awaitPhase(t, "cancelled")
	select {
	case <-f.browser.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("browser was not closed")
	}
	if _, err := f.service.PreviewOAuth(context.Background(), "owner", f.view.ID, accountworkbench.OAuthPreviewInput{}); err == nil {
		t.Fatal("cancelled task retained credentials")
	}
}

func TestOAuthMalformedSuccessAndErrorBodiesNeverExposeSecrets(t *testing.T) {
	for _, body := range []string{`{"access_token":"private-access","token_type":"Bearer"}`, `{"error":"private-access"}`, `{"access_token":"private-access","refresh_token":"rt_private","token_type":"Bearer","expires_in":-1}`, `{"access_token":"private-access","refresh_token":"rt_private","token_type":"Bearer","error":"invalid_grant"}`} {
		t.Run(body, func(t *testing.T) {
			f := newOAuthFixture(t)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) { return oauthResponse(body), nil }))
			f.startOAuth(t)
			if err := f.service.FinishOAuth("owner", f.view.ID); err != nil {
				t.Fatal(err)
			}
			task := f.awaitPhase(t, "failed")
			raw, _ := json.Marshal(task)
			if strings.Contains(string(raw), "private-access") {
				t.Fatal("invalid OAuth response leaked into task")
			}
		})
	}
}
