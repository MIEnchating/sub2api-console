package api

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	loginFailureLimit    = 5
	loginFailureWindow   = 5 * time.Minute
	loginBlockedFor      = time.Minute
	loginThrottleMaxKeys = 1024
)

type loginAttempt struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
}

type loginThrottle struct {
	mu                sync.Mutex
	attempts          map[string]loginAttempt
	trustedProxyCIDRs []netip.Prefix
}

type trustedProxyRequestContextKey struct{}

func TrustedProxyHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := context.WithValue(request.Context(), trustedProxyRequestContextKey{}, true)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func newLoginThrottle(trustedProxyCIDRs []netip.Prefix) *loginThrottle {
	return &loginThrottle{
		attempts:          make(map[string]loginAttempt),
		trustedProxyCIDRs: append([]netip.Prefix(nil), trustedProxyCIDRs...),
	}
}

func (t *loginThrottle) retryAfter(request *http.Request, now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := t.key(request)
	attempt, found := t.attempts[key]
	if !found {
		return 0
	}
	if !attempt.blockedUntil.IsZero() {
		if now.Before(attempt.blockedUntil) {
			return attempt.blockedUntil.Sub(now)
		}
		delete(t.attempts, key)
		return 0
	}
	if now.Sub(attempt.windowStart) >= loginFailureWindow {
		delete(t.attempts, key)
	}
	return 0
}

func (t *loginThrottle) recordFailure(request *http.Request, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.prune(now)
	key := t.key(request)
	attempt := t.attempts[key]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= loginFailureWindow {
		attempt = loginAttempt{windowStart: now}
	}
	attempt.failures++
	if attempt.failures >= loginFailureLimit {
		attempt.blockedUntil = now.Add(loginBlockedFor)
	}
	t.attempts[key] = attempt
}

func (t *loginThrottle) recordSuccess(request *http.Request) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, t.key(request))
}

func (t *loginThrottle) prune(now time.Time) {
	for key, attempt := range t.attempts {
		if (!attempt.blockedUntil.IsZero() && !now.Before(attempt.blockedUntil)) ||
			(attempt.blockedUntil.IsZero() && now.Sub(attempt.windowStart) >= loginFailureWindow) {
			delete(t.attempts, key)
		}
	}
	if len(t.attempts) < loginThrottleMaxKeys {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, attempt := range t.attempts {
		if oldestKey == "" || attempt.windowStart.Before(oldest) {
			oldestKey, oldest = key, attempt.windowStart
		}
	}
	delete(t.attempts, oldestKey)
}

func (t *loginThrottle) key(request *http.Request) string {
	if address, ok := t.clientAddress(request); ok {
		return address.String()
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	return strings.TrimSpace(host)
}

func (t *loginThrottle) clientAddress(request *http.Request) (netip.Addr, bool) {
	peer, ok := requestPeerAddress(request)
	if !t.fromTrustedProxy(request) {
		return peer, ok
	}
	forwarded := strings.Split(request.Header.Get("X-Forwarded-For"), ",")
	var leftmost netip.Addr
	for index := len(forwarded) - 1; index >= 0; index-- {
		address, parseErr := netip.ParseAddr(strings.TrimSpace(forwarded[index]))
		if parseErr != nil {
			continue
		}
		address = address.Unmap()
		leftmost = address
		if !prefixContains(t.trustedProxyCIDRs, address) {
			return address, true
		}
	}
	if leftmost.IsValid() {
		return leftmost, true
	}
	if realIP, parseErr := netip.ParseAddr(strings.TrimSpace(request.Header.Get("X-Real-IP"))); parseErr == nil {
		return realIP.Unmap(), true
	}
	return peer, ok
}

func (t *loginThrottle) fromTrustedProxy(request *http.Request) bool {
	if trusted, _ := request.Context().Value(trustedProxyRequestContextKey{}).(bool); trusted {
		return true
	}
	peer, ok := requestPeerAddress(request)
	return ok && prefixContains(t.trustedProxyCIDRs, peer)
}

func requestPeerAddress(request *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	peer, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}, false
	}
	return peer.Unmap(), true
}

func prefixContains(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
