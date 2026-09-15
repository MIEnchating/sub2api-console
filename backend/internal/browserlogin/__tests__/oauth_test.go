package browserlogin_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

type oauthBrowserFixture struct {
	closed chan struct{}
	once   sync.Once
	result browserlogin.OAuthResult
	err    error
	frame  func(context.Context) ([]byte, error)
}

func (b *oauthBrowserFixture) Screenshot(ctx context.Context) ([]byte, error) {
	if b.frame != nil {
		return b.frame(ctx)
	}
	return []byte("oauth-frame"), nil
}
func (b *oauthBrowserFixture) Input(context.Context, browserlogin.Input) error { return nil }
func (b *oauthBrowserFixture) AuthorizationCode(context.Context) (browserlogin.OAuthResult, error) {
	return b.result, b.err
}
func (b *oauthBrowserFixture) Close() { b.once.Do(func() { close(b.closed) }) }

type oauthFactoryFixture struct {
	browser *oauthBrowserFixture
	options chan browserlogin.OAuthOptions
	open    func(context.Context) (browserlogin.OAuthBrowser, error)
}

func (f *oauthFactoryFixture) OpenOAuth(ctx context.Context, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	f.options <- options
	if f.open != nil {
		return f.open(ctx)
	}
	return f.browser, nil
}

func validOAuthOptions(state string) browserlogin.OAuthOptions {
	return browserlogin.OAuthOptions{
		AuthorizationURL: "https://auth.openai.com/oauth/authorize?client_id=app_EMoamEEZ73f0CkXaXp7hrann&code_challenge=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&code_challenge_method=S256&redirect_uri=http%3A%2F%2Flocalhost%3A1455%2Fauth%2Fcallback&response_type=code&scope=openid+profile+email+offline_access&state=" + state,
		State:            state, RedirectURI: "http://localhost:1455/auth/callback",
	}
}

func TestOAuthOptionsRejectExpandedPermissionsAndMalformedPKCE(t *testing.T) {
	tests := []struct {
		name, parameter, value string
	}{
		{name: "unapproved_scope", parameter: "scope", value: "openid profile email offline_access admin"},
		{name: "missing_scope", parameter: "scope", value: "openid profile email"},
		{name: "duplicate_scope", parameter: "scope", value: "openid profile email offline_access email"},
		{name: "short_challenge", parameter: "code_challenge", value: "challenge"},
		{name: "padded_challenge", parameter: "code_challenge", value: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},
		{name: "invalid_challenge_encoding", parameter: "code_challenge", value: strings.Repeat("!", 43)},
		{name: "unapproved_resource", parameter: "resource", value: "https://example.test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := validOAuthOptions("state-1")
			parsed, err := url.Parse(options.AuthorizationURL)
			if err != nil {
				t.Fatal(err)
			}
			query := parsed.Query()
			query.Set(test.parameter, test.value)
			parsed.RawQuery = query.Encode()
			options.AuthorizationURL = parsed.String()
			if err := options.Validate(); err == nil {
				t.Fatal("unsafe OAuth parameters accepted")
			}
		})
	}
}

func TestOAuthOptionsRejectAlternatePortFragmentAndAmbiguousParameters(t *testing.T) {
	valid := validOAuthOptions("state-1")
	tests := []struct{ name, address string }{
		{name: "alternate_port", address: strings.Replace(valid.AuthorizationURL, "auth.openai.com", "auth.openai.com:8443", 1)},
		{name: "fragment", address: valid.AuthorizationURL + "#authorization"},
		{name: "duplicate_state", address: valid.AuthorizationURL + "&state=other"},
		{name: "duplicate_redirect", address: valid.AuthorizationURL + "&redirect_uri=https%3A%2F%2Fevil.example"},
		{name: "malformed_query", address: valid.AuthorizationURL + "&bad=%xx"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := valid
			options.AuthorizationURL = test.address
			if err := options.Validate(); err == nil {
				t.Fatal("ambiguous OAuth authorization accepted")
			}
		})
	}
}

func waitSocket(t *testing.T, socket string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("browser worker socket did not start")
}

func TestOAuthOptionsRejectUntrustedOrMismatchedAuthorization(t *testing.T) {
	valid := validOAuthOptions("state-1")
	cases := []browserlogin.OAuthOptions{
		{AuthorizationURL: strings.Replace(valid.AuthorizationURL, "auth.openai.com", "evil.example", 1), State: valid.State, RedirectURI: valid.RedirectURI},
		{AuthorizationURL: strings.Replace(valid.AuthorizationURL, "state=state-1", "state=other", 1), State: valid.State, RedirectURI: valid.RedirectURI},
		{AuthorizationURL: strings.Replace(valid.AuthorizationURL, "client_id=app_EMoamEEZ73f0CkXaXp7hrann", "client_id=other", 1), State: valid.State, RedirectURI: valid.RedirectURI},
		{AuthorizationURL: valid.AuthorizationURL, State: "other", RedirectURI: valid.RedirectURI},
		{AuthorizationURL: valid.AuthorizationURL, State: valid.State, RedirectURI: "http://127.0.0.1:1455/auth/callback"},
	}
	for index, current := range cases {
		if err := current.Validate(); err == nil {
			t.Fatalf("case %d accepted invalid OAuth options", index)
		}
	}
}

func TestRemoteOAuthWorkerKeepsCallbackOnPrivateSocket(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "oauth-browser.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &oauthFactoryFixture{browser: &oauthBrowserFixture{closed: make(chan struct{}), result: browserlogin.OAuthResult{Code: "isolated-code", State: "state-1"}}, options: make(chan browserlogin.OAuthOptions, 1)}
	done := make(chan error, 1)
	go func() { done <- browserlogin.RunWorker(ctx, socket, f) }()
	waitSocket(t, socket)
	remote := browserlogin.NewRemote(socket)
	browser, err := remote.OpenOAuth(ctx, validOAuthOptions("state-1"))
	if err != nil {
		t.Fatal(err)
	}
	if image, err := browser.Screenshot(ctx); err != nil || string(image) != "oauth-frame" {
		t.Fatalf("OAuth screenshot failed: %v", err)
	}
	if err := browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	result, err := browser.AuthorizationCode(ctx)
	if err != nil || result.Code != "isolated-code" || result.State != "state-1" {
		t.Fatalf("OAuth callback result invalid: %#v %v", result, err)
	}
	select {
	case options := <-f.options:
		if options.State != "state-1" || options.RedirectURI != "http://localhost:1455/auth/callback" {
			t.Fatal("worker changed OAuth transaction")
		}
	case <-time.After(time.Second):
		t.Fatal("OAuth factory was not called")
	}
	browser.Close()
	select {
	case <-f.browser.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("OAuth browser was not closed")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop")
	}
}
