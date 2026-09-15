package accountworkbench_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestOAuthProxyIsExpandedOnceAndNeverIncludedInViewsOrTasks(t *testing.T) {
	f := newOAuthFixture(t)
	var err error
	f.view, err = f.service.StartOAuthWithInput(context.Background(), "owner", accountworkbench.OAuthStartInput{ProxyURL: "https://test-session-{session}:private-proxy-password@proxy.example:443"})
	if err != nil {
		t.Fatal(err)
	}
	waiting := f.awaitPhase(t, "waiting")
	f.browser.mu.Lock()
	proxyURL := f.browser.options.ProxyURL
	f.browser.mu.Unlock()
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(proxy.User.Username(), "test-session-") || strings.Contains(proxy.User.Username(), "{session}") {
		t.Fatal("proxy session was not expanded")
	}
	password, _ := proxy.User.Password()
	if password != "private-proxy-password" {
		t.Fatal("private proxy authentication changed")
	}
	terminal := f.finishOAuth(t)
	f.browser.mu.Lock()
	after := f.browser.options.ProxyURL
	f.browser.mu.Unlock()
	if after != proxyURL || f.calls.Load() != 1 {
		t.Fatal("authorization changed proxy or replayed exchange")
	}
	for _, value := range []any{f.view, waiting, terminal} {
		raw, _ := json.Marshal(value)
		for _, secret := range []string{"proxy.example", "private-proxy-password", proxy.User.Username()} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("proxy credentials reached public metadata")
			}
		}
	}
}

func TestOAuthProxyInvalidOrConflictingInputDoesNotStartBrowser(t *testing.T) {
	for _, input := range []accountworkbench.OAuthStartInput{
		{ProxyURL: "http://127.0.0.1:8080"},
		{ProxyURL: "http://proxy.example:8080", Login: &accountworkbench.OAuthLoginInput{Email: "owner@example.com", ProxyURL: "http://other.example:8080"}},
	} {
		f := newOAuthFixture(t)
		if _, err := f.service.StartOAuthWithInput(context.Background(), "owner", input); err == nil {
			t.Fatal("invalid proxy started authorization")
		}
		f.browser.mu.Lock()
		opened := f.browser.options.State != ""
		f.browser.mu.Unlock()
		if opened || f.calls.Load() != 0 {
			t.Fatal("invalid proxy performed an external step")
		}
	}
}

func TestOAuthBatchProxyValidationRejectsInputWithoutEchoingAuthentication(t *testing.T) {
	f := newOAuthFixture(t)
	_, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Content: "owner@example.com", ProxyURL: "http://private-user:private-password@127.0.0.1:8080"})
	if err == nil {
		t.Fatal("private proxy endpoint accepted")
	}
	if strings.Contains(err.Error(), "private-user") || strings.Contains(err.Error(), "private-password") {
		t.Fatal("proxy error exposed authentication")
	}
	items, failures := accountworkbench.ParseOAuthLogins(`[{"email":"owner@example.com","proxy_url":"https://proxy.example:443"}]`)
	if len(failures) != 0 || len(items) != 1 || items[0].ProxyURL != "https://proxy.example:443" {
		t.Fatal("structured login input lost proxy setting")
	}
}
