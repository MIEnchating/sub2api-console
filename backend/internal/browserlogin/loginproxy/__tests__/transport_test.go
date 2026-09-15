package loginproxy_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
)

func TestProxyTransportKeepsProxyAuthenticationOutsideTheOfficialTLSRequest(t *testing.T) {
	originRequests := make(chan http.Header, 1)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originRequests <- r.Header.Clone()
		_, _ = w.Write([]byte(`{"access_token":"isolated-result"}`))
	}))
	t.Cleanup(origin.Close)
	var connections atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		upstream, err := net.Dial("tcp", origin.Listener.Addr().String())
		if err != nil {
			http.Error(w, "isolated origin unavailable", 502)
			return
		}
		defer upstream.Close()
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buffer); upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		client.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	proxyURL, _ := url.Parse(proxy.URL)
	dialer, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{UpstreamURL: "http://operator:proxy-private@proxy.example:" + proxyURL.Port(), Destinations: map[string]string{"example.com:443": "8.8.8.8:443"}, Resolve: func(context.Context, string) (string, error) { return "127.0.0.1", nil }})
	if err != nil {
		t.Fatal(err)
	}
	transport := dialer.Transport()
	transport.TLSClientConfig.RootCAs = origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	request, err := http.NewRequest(http.MethodPost, "https://example.com/oauth/token", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer official-only")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("official response = %d", response.StatusCode)
	}
	observed := <-originRequests
	if observed.Get("Proxy-Authorization") != "" || observed.Get("Authorization") != "Bearer official-only" || connections.Load() != 1 {
		t.Fatal("proxy credentials crossed into the official request or exchange was retried")
	}
	if _, err := client.Get("https://unapproved.example/oauth/token"); err == nil {
		t.Fatal("transport contacted an unapproved endpoint")
	}
}

func TestProxyTransportDoesNotReplayPOSTAfterTheOriginAcceptsItsBodyThenDisconnects(t *testing.T) {
	var requests atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			connection.Close()
		}
	}))
	t.Cleanup(origin.Close)
	endpoint, _ := url.Parse(origin.URL)
	dialer, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{Destinations: map[string]string{"example.com:" + endpoint.Port(): origin.Listener.Addr().String()}, Resolve: func(context.Context, string) (string, error) { return "127.0.0.1", nil }})
	if err != nil {
		t.Fatal(err)
	}
	transport := dialer.Transport()
	transport.TLSClientConfig.RootCAs = origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	t.Cleanup(transport.CloseIdleConnections)
	request, err := http.NewRequest(http.MethodPost, "https://example.com:"+endpoint.Port()+"/oauth/token", strings.NewReader("code=one-time-isolated-code"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if response != nil {
		response.Body.Close()
	}
	if err == nil || requests.Load() != 1 {
		t.Fatalf("one-time exchange error = %v, origin requests = %d", err, requests.Load())
	}
}

func TestProxyTransportRejectsTLSProxyWithAnUntrustedCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("untrusted proxy received a CONNECT request") }))
	t.Cleanup(server.Close)
	endpoint, _ := url.Parse(server.URL)
	dialer, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{UpstreamURL: "https://example.com:" + endpoint.Port(), Destinations: map[string]string{"auth.openai.com:443": "8.8.8.8:443"}, Resolve: func(context.Context, string) (string, error) { return "127.0.0.1", nil }})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := dialer.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	if connection != nil {
		connection.Close()
	}
	if !errors.Is(err, loginproxy.ErrConnect) {
		t.Fatalf("untrusted TLS proxy = %v", err)
	}
}
