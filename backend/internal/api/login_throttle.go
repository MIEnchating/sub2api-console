package api

import (
	"net"
	"net/http"
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
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{attempts: make(map[string]loginAttempt)}
}

func (t *loginThrottle) retryAfter(request *http.Request, now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := loginThrottleKey(request)
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
	key := loginThrottleKey(request)
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
	delete(t.attempts, loginThrottleKey(request))
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

func loginThrottleKey(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	return host
}
