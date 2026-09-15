package browserlogin_test

import (
	"net/http"
	"sync"
	"testing"
)

func TestIsolatedSecurityNavigationAfterEligibilityCheckDoesNotReturnScreenshot(t *testing.T) {
	fixture := &securityHTTPFixture{t: t}
	ready := make(chan struct{})
	var once sync.Once
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/log-in":
			w.Header().Set("Content-Type", "text/html")
			// Queue navigation at the end of the eligibility evaluation, before the
			// separate capture command. This controls the browser navigation boundary
			// without a timing race or a sleep.
			_, _ = w.Write([]byte(`<html><body>Official login<script>Object.defineProperty(document,'readyState',{get(){queueMicrotask(()=>{history.replaceState({},'', '/private-account');document.body.textContent='PRIVATE ACCOUNT DETAILS';});return 'complete';}});fetch('/test-screenshot-ready');</script></body></html>`))
		case "/test-screenshot-ready":
			once.Do(func() { close(ready) })
			w.WriteHeader(http.StatusNoContent)
		default:
			fixture.ServeHTTP(w, r)
		}
	})
	ctx, browser := isolatedSecurityBrowser(t, handler)
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("controlled official document was not ready")
	}
	image, err := browser.Screenshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(image) != 0 {
		t.Fatal("screenshot returned private page after the eligibility check")
	}
}
