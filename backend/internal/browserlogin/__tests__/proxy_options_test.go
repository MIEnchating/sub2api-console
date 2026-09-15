package browserlogin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestOAuthWorkerPassesPrivateProxyConfigurationWithoutIncludingItInScreenshots(t *testing.T) {
	factory := &oauthFactoryFixture{browser: &oauthBrowserFixture{closed: make(chan struct{})}, options: make(chan browserlogin.OAuthOptions, 1)}
	remote := startOAuthWorker(t, factory)
	options := validOAuthOptions("state-proxy")
	var err error
	options.ProxyURL, err = browserlogin.NewProxySessionURL("http://operator-{session}:private-proxy-password@proxy.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	browser, err := remote.OpenOAuth(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(browser.Close)
	select {
	case observed := <-factory.options:
		if observed.ProxyURL != options.ProxyURL || strings.Contains(observed.ProxyURL, "{session}") {
			t.Fatal("private worker changed or re-expanded the selected proxy")
		}
	case <-time.After(time.Second):
		t.Fatal("private worker did not receive OAuth configuration")
	}
	image, err := browser.Screenshot(context.Background())
	if err != nil || string(image) != "oauth-frame" || strings.Contains(string(image), "private-proxy-password") {
		t.Fatal("screenshot response contained proxy configuration")
	}
}

func TestOAuthOptionsRejectPrivateProxyTargetsBeforeWorkerStartup(t *testing.T) {
	options := validOAuthOptions("state-proxy")
	options.ProxyURL = "http://operator:private-proxy-password@127.0.0.1:8080"
	if err := options.Validate(); err == nil || strings.Contains(err.Error(), "private-proxy-password") {
		t.Fatalf("private proxy option = %v", err)
	}
	if _, err := browserlogin.NewProxyTransport(context.Background(), options.ProxyURL); err == nil || strings.Contains(err.Error(), "private-proxy-password") {
		t.Fatalf("private proxy exchange transport = %v", err)
	}
}

func TestSecurityOptionsRejectPrivateProxyTargetsBeforeWorkerStartup(t *testing.T) {
	options := browserlogin.SecurityOptions{Email: "owner@example.com", ProxyURL: "socks5://operator:private-proxy-password@127.0.0.1:1080"}
	if err := options.Validate(); err == nil || strings.Contains(err.Error(), "private-proxy-password") {
		t.Fatalf("private security proxy option = %v", err)
	}
	options.ProxyURL = "socks5://operator:private-proxy-password@proxy.example:1080"
	if err := options.Validate(); err != nil {
		t.Fatalf("public security proxy option = %v", err)
	}
}
