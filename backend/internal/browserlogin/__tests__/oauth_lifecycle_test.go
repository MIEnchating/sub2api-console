package browserlogin_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func startOAuthWorker(t *testing.T, factory *oauthFactoryFixture) *browserlogin.Remote {
	t.Helper()
	return browserlogin.NewRemote(startOAuthWorkerSocket(t, factory))
}

func startOAuthWorkerSocket(t *testing.T, factory *oauthFactoryFixture) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "oauth.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- browserlogin.RunWorker(ctx, socket, factory) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("worker did not shut down")
		}
	})
	waitSocket(t, socket)
	return socket
}

func TestOAuthWorkerPreservesTypedCallbackErrors(t *testing.T) {
	for _, expected := range []error{browserlogin.ErrOAuthPending, browserlogin.ErrOAuthState, browserlogin.ErrOAuthRejected} {
		t.Run(expected.Error(), func(t *testing.T) {
			factory := &oauthFactoryFixture{browser: &oauthBrowserFixture{closed: make(chan struct{}), err: expected}, options: make(chan browserlogin.OAuthOptions, 1)}
			remote := startOAuthWorker(t, factory)
			browser, err := remote.OpenOAuth(context.Background(), validOAuthOptions("state-1"))
			if err != nil {
				t.Fatal(err)
			}
			defer browser.Close()
			result, err := browser.AuthorizationCode(context.Background())
			if !errors.Is(err, expected) || result != (browserlogin.OAuthResult{}) {
				t.Fatalf("callback error identity or privacy lost: %v", err)
			}
			if _, err := browser.Screenshot(context.Background()); err != nil {
				t.Fatalf("callback error prevented screenshot: %v", err)
			}
		})
	}
}

func TestOAuthWorkerRejectsConcurrentAuthorizationAndReleasesClosedSession(t *testing.T) {
	factory := &oauthFactoryFixture{options: make(chan browserlogin.OAuthOptions, 2), open: func(context.Context) (browserlogin.OAuthBrowser, error) {
		return &oauthBrowserFixture{closed: make(chan struct{})}, nil
	}}
	remote := startOAuthWorker(t, factory)
	browser, err := remote.OpenOAuth(context.Background(), validOAuthOptions("state-1"))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	if concurrent, err := remote.OpenOAuth(context.Background(), validOAuthOptions("state-2")); err == nil {
		concurrent.Close()
		t.Fatal("concurrent authorization replaced an active OAuth session")
	}
	browser.Close()
	next, err := remote.OpenOAuth(context.Background(), validOAuthOptions("state-2"))
	if err != nil {
		t.Fatalf("closed OAuth session did not release browser: %v", err)
	}
	next.Close()
}

func TestOAuthWorkerCloseCancelsInFlightScreenshot(t *testing.T) {
	started := make(chan struct{})
	closed := make(chan struct{})
	fixture := &oauthBrowserFixture{closed: closed, frame: func(ctx context.Context) ([]byte, error) {
		close(started)
		select {
		case <-closed:
			return nil, browserlogin.ErrSession
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	remote := startOAuthWorker(t, &oauthFactoryFixture{browser: fixture, options: make(chan browserlogin.OAuthOptions, 1)})
	browser, err := remote.OpenOAuth(context.Background(), validOAuthOptions("state-1"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := browser.Screenshot(ctx); result <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("screenshot did not start")
	}
	go browser.Close()
	select {
	case err := <-result:
		if !errors.Is(err, browserlogin.ErrSession) {
			t.Fatalf("closing OAuth session did not cancel pending screenshot: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("browser close was blocked behind screenshot")
	}
}

func TestOAuthWorkerAbandonedStartupClosesReturnedBrowser(t *testing.T) {
	started := make(chan struct{})
	fixture := &oauthBrowserFixture{closed: make(chan struct{})}
	factory := &oauthFactoryFixture{options: make(chan browserlogin.OAuthOptions, 1), open: func(ctx context.Context) (browserlogin.OAuthBrowser, error) {
		close(started)
		<-ctx.Done()
		return fixture, nil
	}}
	remote := startOAuthWorker(t, factory)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := remote.OpenOAuth(ctx, validOAuthOptions("state-1")); result <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth startup did not reach factory")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled OAuth startup returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth startup ignored cancellation")
	}
	select {
	case <-fixture.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("abandoned OAuth browser was not closed")
	}
}
