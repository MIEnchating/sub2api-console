package browserlogin_test

import (
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func isolatedLoginProxy(t *testing.T) (string, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var connections, invalid atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("operator:private-proxy-password"))
		if r.Method != http.MethodConnect || r.Host != "127.0.0.1:443" || r.Header.Get("Proxy-Authorization") != wantedAuth {
			invalid.Add(1)
			http.Error(w, "isolated proxy rejected tunnel", http.StatusForbidden)
			return
		}
		connections.Add(1)
		upstream, err := net.Dial("tcp", "127.0.0.1:443")
		if err != nil {
			http.Error(w, "isolated upstream unavailable", 502)
			return
		}
		defer upstream.Close()
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buffer); upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		client.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	endpoint, _ := url.Parse(proxy.URL)
	return "http://operator:private-proxy-password@proxy.example:" + endpoint.Port(), &connections, &invalid
}

func TestIsolatedOAuthProxyKeepsOfficialNavigationAndCallbackInsideTheRestrictedTunnel(t *testing.T) {
	proxyURL, connections, invalid := isolatedLoginProxy(t)
	ctx, browser, callbackRequests := isolatedOAuthBrowserViaProxy(t, proxyURL, "state=state-1&code=isolated-proxy-code", true)
	if image, err := browser.Screenshot(ctx); err != nil || len(image) == 0 {
		t.Fatalf("proxied official page did not render: %v", err)
	}
	if err := browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	result, err := awaitOAuthCallback(t, ctx, browser)
	if err != nil || result.Code != "isolated-proxy-code" {
		t.Fatalf("proxied OAuth callback = %+v, %v", result, err)
	}
	if connections.Load() == 0 || invalid.Load() != 0 || callbackRequests.Load() != 0 {
		t.Fatalf("restricted proxy connections=%d invalid=%d callback=%d", connections.Load(), invalid.Load(), callbackRequests.Load())
	}
}

func TestIsolatedSecurityProxyPreservesIdentityAndSingleTOTPActivationOverThePrivateSocket(t *testing.T) {
	proxyURL, connections, invalid := isolatedLoginProxy(t)
	fixture := &securityHTTPFixture{t: t}
	ctx, browser := isolatedSecurityBrowserViaProxy(t, proxyURL, fixture, true)
	identity := completeSecurityLogin(t, ctx, browser)
	enrollment, err := browser.EnrollTOTP(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := browser.ActivateTOTP(ctx, identity, enrollment, "012345"); err != nil {
		t.Fatal(err)
	}
	if image, err := browser.Screenshot(ctx); err != nil || len(image) != 0 {
		t.Fatal("proxied security operation exposed private web content")
	}
	if fixture.enrolls.Load() != 1 || fixture.activates.Load() != 1 || !fixture.enabled.Load() || connections.Load() == 0 || invalid.Load() != 0 {
		t.Fatalf("proxied TOTP enroll=%d activate=%d proxy=%d invalid=%d", fixture.enrolls.Load(), fixture.activates.Load(), connections.Load(), invalid.Load())
	}
}
