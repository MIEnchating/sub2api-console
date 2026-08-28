package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginThrottleBlocksRepeatedFailuresAndExpires(t *testing.T) {
	throttle := newLoginThrottle()
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
	throttle := newLoginThrottle()
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
	throttle := newLoginThrottle()
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
