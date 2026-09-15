package browserlogin

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
)

func ValidateProxyURL(raw string) error { return loginproxy.Validate(raw) }

func NewProxySessionURL(raw string) (string, error) { return loginproxy.SessionURL(raw) }

// NewProxyTransport confines the single authorization-code exchange to its
// official endpoint and the same expanded proxy URL as the login browser.
func NewProxyTransport(ctx context.Context, raw string) (*http.Transport, error) {
	if err := ValidateProxyURL(raw); err != nil {
		return nil, err
	}
	resolve, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	address, err := loginproxy.PublicAddress(resolve, "auth.openai.com")
	if err != nil {
		return nil, err
	}
	dialer, err := loginproxy.NewDialer(resolve, loginproxy.Config{UpstreamURL: raw, Destinations: map[string]string{"auth.openai.com:443": net.JoinHostPort(address, "443")}})
	if err != nil {
		return nil, err
	}
	return dialer.Transport(), nil
}
