package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestTrustedProxyHandlerUsesForwardedClientWithoutTrustingUnixRemoteAddress(t *testing.T) {
	throttle := newLoginThrottle(nil)
	request := httptest.NewRequest("GET", "http://console.test/", nil)
	request.RemoteAddr = "@"
	request.Header.Set("X-Forwarded-For", "198.51.100.24")
	var key string
	handler := TrustedProxyHandler(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		key = throttle.key(request)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if key != "198.51.100.24" {
		t.Fatalf("trusted Unix proxy key = %q", key)
	}
}

func TestLoginThrottleBlocksRepeatedFailuresAndExpires(t *testing.T) {
	throttle := newLoginThrottle(nil)
	request := httptest.NewRequest("POST", "/api/auth/login", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for range loginFailureLimit {
		if retry := throttle.retryAfter(request, now); retry != 0 {
			t.Fatalf("attempt was blocked early: %s", retry)
		}
		throttle.recordFailure(request, now)
	}
	if retry := throttle.retryAfter(request, now); retry != loginBlockedFor {
		t.Fatalf("retry after = %s, want %s", retry, loginBlockedFor)
	}
	if retry := throttle.retryAfter(request, now.Add(loginBlockedFor)); retry != 0 {
		t.Fatalf("expired block still active: %s", retry)
	}
}

func TestLoginThrottleScopesFailuresAndClearsSuccess(t *testing.T) {
	throttle := newLoginThrottle(nil)
	first := httptest.NewRequest("POST", "/api/auth/login", nil)
	first.RemoteAddr = "192.0.2.10:1234"
	second := httptest.NewRequest("POST", "/api/auth/login", nil)
	second.RemoteAddr = "192.0.2.11:1234"
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for range loginFailureLimit {
		throttle.recordFailure(first, now)
	}
	if retry := throttle.retryAfter(second, now); retry != 0 {
		t.Fatalf("different client was blocked: %s", retry)
	}
	throttle.recordSuccess(first)
	if retry := throttle.retryAfter(first, now); retry != 0 {
		t.Fatalf("successful authentication did not clear failures: %s", retry)
	}
}

func TestLoginThrottleCannotBeBypassedByChangingUsername(t *testing.T) {
	throttle := newLoginThrottle(nil)
	request := httptest.NewRequest("POST", "/api/auth/login", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for range loginFailureLimit {
		throttle.recordFailure(request, now)
	}
	if retry := throttle.retryAfter(request, now); retry == 0 {
		t.Fatal("source address was not blocked after rotating login identities")
	}
}

func TestLoginThrottleUsesForwardedClientFromTrustedProxy(t *testing.T) {
	throttle := newLoginThrottle([]netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")})
	first := httptest.NewRequest("POST", "/api/auth/login", nil)
	first.RemoteAddr = "172.18.0.3:43210"
	first.Header.Set("X-Forwarded-For", "198.51.100.10")
	second := httptest.NewRequest("POST", "/api/auth/login", nil)
	second.RemoteAddr = "172.18.0.3:43211"
	second.Header.Set("X-Forwarded-For", "198.51.100.11")
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for range loginFailureLimit {
		throttle.recordFailure(first, now)
	}
	if retry := throttle.retryAfter(second, now); retry != 0 {
		t.Fatalf("different forwarded client was blocked: %s", retry)
	}
}

func TestLoginThrottleIgnoresForwardedClientFromUntrustedPeer(t *testing.T) {
	throttle := newLoginThrottle([]netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")})
	first := httptest.NewRequest("POST", "/api/auth/login", nil)
	first.RemoteAddr = "192.0.2.20:43210"
	first.Header.Set("X-Forwarded-For", "198.51.100.10")
	second := httptest.NewRequest("POST", "/api/auth/login", nil)
	second.RemoteAddr = "192.0.2.20:43211"
	second.Header.Set("X-Forwarded-For", "198.51.100.11")
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for range loginFailureLimit {
		throttle.recordFailure(first, now)
	}
	if retry := throttle.retryAfter(second, now); retry == 0 {
		t.Fatal("untrusted peer bypassed throttling with a forged forwarded address")
	}
}
