package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

type securityProxyFactory struct {
	next   browserlogin.SecurityFactory
	opened chan browserlogin.SecurityOptions
}

func (f securityProxyFactory) OpenSecurity(ctx context.Context, options browserlogin.SecurityOptions) (browserlogin.SecurityBrowser, error) {
	f.opened <- options
	return f.next.OpenSecurity(ctx, options)
}

func TestSecurityProxyReachesOnlyPrivateBrowserAndNotTaskMetadata(t *testing.T) {
	f := newSecurityFixture(t)
	opened := make(chan browserlogin.SecurityOptions, 1)
	f.service.UseSecurityBrowser(securityProxyFactory{next: f.browser, opened: opened})
	view, err := f.service.StartSecurity(context.Background(), "security-owner", accountworkbench.SecurityStartInput{AccountID: "101", Operation: "totp", Confirmed: true, ProxyURL: "http://session-{session}:private-proxy-password@proxy.example:8080"})
	if err != nil {
		t.Fatal(err)
	}
	waiting := f.awaitSecurity(t, "waiting")
	select {
	case options := <-opened:
		if options.ProxyURL == "" || strings.Contains(options.ProxyURL, "{session}") {
			t.Fatal("private browser missed per-session proxy")
		}
	case <-time.After(time.Second):
		t.Fatal("security browser not opened")
	}
	for _, value := range []any{view, waiting} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "proxy.example") || strings.Contains(string(raw), "private-proxy-password") {
			t.Fatal("proxy leaked to public view")
		}
	}
	if err := f.service.CancelSecurity("security-owner", view.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSecurityBatchRejectsPrivateProxyBeforeReadingOrStartingAccount(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	_, err := f.service.PreviewSecurityBatch(context.Background(), "owner", accountworkbench.SecurityBatchPreviewInput{AccountIDs: []string{"101"}, Operation: "totp", ProxyURL: "http://user:private-pass@127.0.0.1:8080"})
	if err == nil {
		t.Fatal("private proxy accepted")
	}
	if strings.Contains(err.Error(), "private-pass") {
		t.Fatal("proxy password exposed by validation")
	}
	f.factory.mu.Lock()
	opened := f.factory.opened
	f.factory.mu.Unlock()
	if opened != 0 {
		t.Fatal("invalid proxy launched security browser")
	}
}
