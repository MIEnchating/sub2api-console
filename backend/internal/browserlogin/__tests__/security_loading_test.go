package browserlogin_test

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestIsolatedSecurityRejectsInputWhileOfficialDocumentIsStillLoading(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	parsed := make(chan struct{})
	finish := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(finish) }) })
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/log-in":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><script>fetch('/test-document-parsed')</script>`))
			w.(http.Flusher).Flush()
			select {
			case <-finish:
			case <-r.Context().Done():
			}
			_, _ = w.Write([]byte(`</body></html>`))
		case "/test-document-parsed":
			close(parsed)
			w.WriteHeader(http.StatusNoContent)
		default:
			fixture.ServeHTTP(w, r)
		}
	})
	ctx, browser := isolatedSecurityBrowser(t, handler)
	select {
	case <-parsed:
	case <-ctx.Done():
		t.Fatal("official document did not reach the controlled loading boundary")
	}
	err := browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Enter"})
	once.Do(func() { close(finish) })
	if !errors.Is(err, browserlogin.ErrAuthPageChanged) {
		t.Fatal("loading official page accepted an input before controls were ready")
	}
}
