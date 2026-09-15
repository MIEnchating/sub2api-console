package workbenchprovider_test

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderURLRejectsPrivateAndCredentialOrigins(t *testing.T) {
	for _, raw := range []string{"http://mail.example", "https://localhost", "https://a.local", "https://127.0.0.1", "https://169.254.169.254", "https://100.64.1.1", "https://[::1]", "https://user:secret@mail.example", "https://mail.example:8443", "https://mail.example/#secret"} {
		t.Run(raw, func(t *testing.T) {
			if err := workbenchprovider.ValidateURL(raw); err == nil {
				t.Fatal("unsafe origin accepted")
			}
		})
	}
}

func TestSMSPurchaseConnectionLossAfterWarmRequestDoesNotReplayGET(t *testing.T) {
	var purchases atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "getNumber" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if purchases.Add(1) == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		_, _ = w.Write([]byte("ACCESS_NUMBER:duplicate-purchase:17005550123"))
	}))
	t.Cleanup(server.Close)
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "smsbower.page:443" {
			return nil, errors.New("isolated test refuses other destination")
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	client := workbenchprovider.NewHTTP(transport)
	t.Cleanup(client.Close)
	if _, err := client.Do(context.Background(), workbenchprovider.Request{Method: "GET", URL: "https://smsbower.page/stubs/handler_api.php?action=getPrices"}); err != nil {
		t.Fatal(err)
	}
	provider, err := workbenchprovider.NewSMS(workbenchprovider.SMSConfig{Provider: "smsbower", APIKey: "isolated-private-key", Country: "1001"}, client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Acquire(context.Background(), "one-purchase")
	var failure *workbenchprovider.SMSError
	if !errors.As(err, &failure) || !failure.Uncertain || purchases.Load() != 1 {
		t.Fatalf("uncertain purchase was replayed: requests=%d, error=%v", purchases.Load(), err)
	}
	_, _ = provider.Acquire(context.Background(), "one-purchase")
	if purchases.Load() != 1 {
		t.Fatal("same operation ID created another purchase")
	}
}

func TestProviderAddressRejectsMappedAndTranslationNetworks(t *testing.T) {
	for _, address := range []string{"::ffff:127.0.0.1", "64:ff9b::a00:1", "2002:7f00:1::", "2001:db8::1", "198.18.0.1"} {
		if workbenchprovider.PublicAddress(netip.MustParseAddr(address)) {
			t.Fatalf("unsafe address accepted: %s", address)
		}
	}
	if !workbenchprovider.PublicAddress(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("public address rejected")
	}
}

func TestProviderRedirectDoesNotForwardCredentialsOrReplay(t *testing.T) {
	requests := 0
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": {"https://other.example/capture"}}, Body: io.NopCloser(strings.NewReader("secret response")), Request: r}, nil
	}))
	_, err := client.Do(context.Background(), workbenchprovider.Request{Method: "POST", URL: "https://mail.example/token?key=private-key", Header: http.Header{"Authorization": {"Bearer private-token"}}})
	if err == nil || requests != 1 || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("redirect or privacy contract violated: %v, calls=%d", err, requests)
	}
}

func TestProviderTransportFailureHidesCredentialURL(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New(r.URL.String())
	}))
	_, err := client.Do(context.Background(), workbenchprovider.Request{Method: "GET", URL: "https://mail.example/?key=private-key"})
	if err == nil || strings.Contains(err.Error(), "private-key") {
		t.Fatalf("credential leaked in error: %v", err)
	}
}

func TestProviderResponseSizeBoundAppliesWithoutContentLength(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", workbenchprovider.MaxResponseBytes+1))), Request: r}, nil
	}))
	_, err := client.Do(context.Background(), workbenchprovider.Request{Method: "GET", URL: "https://mail.example/"})
	var providerError *workbenchprovider.Error
	if !errors.As(err, &providerError) || providerError.Code != "provider_response_too_large" {
		t.Fatalf("oversized response accepted: %v", err)
	}
}

func TestProviderSuccessfulRequestPreservesBodyAndAuthorization(t *testing.T) {
	client := workbenchprovider.NewHTTP(transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer test-only" || string(body) != "credential=test-only" {
			t.Fatal("provider request changed")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":"123456"}`)), Request: r}, nil
	}))
	body, err := client.Do(context.Background(), workbenchprovider.Request{Method: "POST", URL: "https://mail.example/", Header: http.Header{"Authorization": {"Bearer test-only"}}, Body: []byte("credential=test-only")})
	if err != nil || string(body) != `{"code":"123456"}` {
		t.Fatalf("valid response rejected: %s %v", body, err)
	}
}
