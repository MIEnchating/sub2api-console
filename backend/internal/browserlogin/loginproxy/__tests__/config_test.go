package loginproxy_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
)

func TestProxyConfigAcceptsSupportedSchemesAndExplicitUsernameSessionTemplate(t *testing.T) {
	for _, raw := range []string{"", "http://proxy.example", "https://operator:private@proxy.example:443", "socks5://operator-{session}:private@proxy.example:1080", "http://8.8.8.8:8080", "https://[2606:4700:4700::1111]"} {
		if err := loginproxy.Validate(raw); err != nil {
			t.Fatalf("supported proxy rejected: %v", err)
		}
	}
}

func TestProxyConfigRejectsPrivateAddressesAmbiguousURLsAndCredentialInjection(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8080", "http://localhost", "http://service.local", "http://proxy.internal", "http://10.0.0.1", "http://169.254.169.254",
		"http://100.64.0.1", "http://192.0.2.1", "http://198.18.0.1", "http://[::1]", "http://[::ffff:127.0.0.1]", "http://[64:ff9b::7f00:1]",
		"ftp://proxy.example", "socks5h://proxy.example", "https://proxy.example/path", "http://proxy.example?password=secret", "http://proxy.example#secret", "http://proxy.example?",
		"http://proxy.example:0", "http://proxy.example:65536", "http://proxy.example:", " http://proxy.example", "http://user%0d%0a:pass@proxy.example",
		"http://user:pass%00word@proxy.example", "http://user:pass-{session}@proxy.example", "http://{session}.proxy.example", "http://user-{unknown}@proxy.example",
		"http://user%3Aother:pass@proxy.example", "socks5://" + strings.Repeat("u", 256) + ":pass@proxy.example", "http://@proxy.example",
	} {
		if err := loginproxy.Validate(raw); err == nil {
			t.Fatalf("unsafe proxy configuration accepted: %q", raw)
		} else if strings.Contains(err.Error(), raw) {
			t.Fatal("proxy validation error included private input")
		}
	}
}

func TestProxySessionExpansionChangesOnlyTheExplicitUsernamePlaceholder(t *testing.T) {
	first, err := loginproxy.SessionURL("socks5://account-{session}:private%40password@proxy.example:1080")
	if err != nil {
		t.Fatal(err)
	}
	second, err := loginproxy.SessionURL("socks5://account-{session}:private%40password@proxy.example:1080")
	if err != nil {
		t.Fatal(err)
	}
	left, _ := url.Parse(first)
	right, _ := url.Parse(second)
	password, _ := left.User.Password()
	if !strings.HasPrefix(left.User.Username(), "account-") || len(left.User.Username()) != len("account-")+32 || left.User.Username() == right.User.Username() || password != "private@password" || left.Host != "proxy.example:1080" {
		t.Fatal("new proxy sessions did not preserve credentials and isolate the rotation identifier")
	}
	unchanged, err := loginproxy.SessionURL(first)
	if err != nil || unchanged != first {
		t.Fatal("an expanded proxy URL rotated again")
	}
}

func TestProxyDialerRejectsUnexpandedSessionBeforeResolvingAnEndpoint(t *testing.T) {
	resolved := false
	_, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{UpstreamURL: "http://account-{session}:private@proxy.example", Resolve: func(context.Context, string) (string, error) { resolved = true; return "127.0.0.1", nil }})
	if err == nil || resolved {
		t.Fatal("proxy dialer accepted or resolved a session template before explicit session creation")
	}
}

func TestProxyDialerRejectsPrivatePinnedDestinationsUnderProductionPolicy(t *testing.T) {
	_, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{Destinations: map[string]string{"auth.openai.com:443": "127.0.0.1:443"}})
	if err == nil {
		t.Fatal("production proxy accepted an internal pinned destination")
	}
}
