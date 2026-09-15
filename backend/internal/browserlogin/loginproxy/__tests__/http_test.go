package loginproxy_test

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
)

type proxyRequest struct{ method, target, auth, originAuth string }

func TestHTTPProxyPinsTheDestinationAndKeepsCredentialsInsideCONNECT(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "http"
		if secure {
			name = "https"
		}
		t.Run(name, func(t *testing.T) {
			requests := make(chan proxyRequest, 2)
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- proxyRequest{r.Method, r.Host, r.Header.Get("Proxy-Authorization"), r.Header.Get("Authorization")}
				connection, buffer, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer connection.Close()
				_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\nready")
				_ = buffer.Flush()
				_, _ = io.Copy(connection, buffer)
			})
			server := httptest.NewUnstartedServer(handler)
			if secure {
				server.StartTLS()
			} else {
				server.Start()
			}
			t.Cleanup(server.Close)
			remote, _ := url.Parse(server.URL)
			var resolutions atomic.Int32
			config := loginproxy.Config{
				UpstreamURL:  name + "://operator:private%40password@example.com:" + remote.Port(),
				Destinations: map[string]string{"auth.openai.com:443": "8.8.8.8:443"},
				Resolve: func(_ context.Context, host string) (string, error) {
					resolutions.Add(1)
					if host != "example.com" {
						return "", errors.New("unexpected resolver request")
					}
					return "127.0.0.1", nil
				},
			}
			if secure {
				config.RootCAs = x509.NewCertPool()
				config.RootCAs.AddCert(server.Certificate())
			}
			dialer, err := loginproxy.NewDialer(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			// Caller mutations cannot replace the target after policy capture.
			config.Destinations["auth.openai.com:443"] = "127.0.0.1:443"
			for range 2 {
				connection, err := dialer.DialContext(context.Background(), "tcp", "auth.openai.com:443")
				if err != nil {
					t.Fatal(err)
				}
				_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
				ready := make([]byte, 5)
				if _, err := io.ReadFull(connection, ready); err != nil || string(ready) != "ready" {
					t.Fatalf("buffered tunnel data = %q, %v", ready, err)
				}
				_, err = connection.Write([]byte("official-tls-payload"))
				if err != nil {
					t.Fatal(err)
				}
				echo := make([]byte, len("official-tls-payload"))
				if _, err := io.ReadFull(connection, echo); err != nil || string(echo) != "official-tls-payload" {
					t.Fatalf("tunnel payload = %q, %v", echo, err)
				}
				_ = connection.Close()
				request := <-requests
				wanted := "Basic " + base64.StdEncoding.EncodeToString([]byte("operator:private@password"))
				if request.method != "CONNECT" || request.target != "8.8.8.8:443" || request.auth != wanted || request.originAuth != "" {
					t.Fatal("CONNECT did not confine proxy credentials and destination")
				}
			}
			if resolutions.Load() != 1 {
				t.Fatalf("proxy DNS was resolved %d times", resolutions.Load())
			}
			if _, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:443"); !errors.Is(err, loginproxy.ErrDestination) {
				t.Fatalf("unapproved destination = %v", err)
			}
		})
	}
}

func TestHTTPProxyRejectsFailedOrOversizedCONNECTWithoutRetryOrPrivateErrorText(t *testing.T) {
	for _, status := range []string{"407", "redirect", "oversized"} {
		t.Run(status, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch status {
				case "407":
					http.Error(w, "private-upstream-response", http.StatusProxyAuthRequired)
				case "redirect":
					http.Redirect(w, r, "http://127.0.0.1/private", http.StatusFound)
				case "oversized":
					w.Header().Set("X-Proxy-Debug", strings.Repeat("p", 20<<10))
					w.WriteHeader(http.StatusOK)
				}
			}))
			t.Cleanup(server.Close)
			endpoint, _ := url.Parse(server.URL)
			dialer, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{UpstreamURL: "http://operator:private-password@proxy.example:" + endpoint.Port(), Destinations: map[string]string{"auth.openai.com:443": "8.8.8.8:443"}, Resolve: func(context.Context, string) (string, error) { return "127.0.0.1", nil }})
			if err != nil {
				t.Fatal(err)
			}
			connection, err := dialer.DialContext(context.Background(), "tcp", "auth.openai.com:443")
			if connection != nil {
				connection.Close()
				t.Fatal("failed CONNECT returned an open tunnel")
			}
			if !errors.Is(err, loginproxy.ErrConnect) || calls.Load() != 1 || strings.Contains(err.Error(), "private") {
				t.Fatalf("CONNECT error = %v, attempts = %d", err, calls.Load())
			}
		})
	}
}

func TestHTTPProxyCancellationClosesPendingHandshake(t *testing.T) {
	started := make(chan struct{})
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(closed)
	}))
	t.Cleanup(server.Close)
	endpoint, _ := url.Parse(server.URL)
	dialer, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{UpstreamURL: "http://proxy.example:" + endpoint.Port(), Destinations: map[string]string{"auth.openai.com:443": "8.8.8.8:443"}, Resolve: func(context.Context, string) (string, error) { return "127.0.0.1", nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		connection, err := dialer.DialContext(ctx, "tcp", "auth.openai.com:443")
		if connection != nil {
			connection.Close()
		}
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy handshake did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled handshake = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled handshake remained open")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy connection did not close")
	}
}

func TestDirectDialerPreservesTheApprovedSocketAndRejectsOtherTargets(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			accepted <- connection
		}
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	host := "auth.openai.com:" + port
	dialer, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{Destinations: map[string]string{host: listener.Addr().String()}, Resolve: func(context.Context, string) (string, error) { return "127.0.0.1", nil }})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := dialer.DialContext(context.Background(), "tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	remote := <-accepted
	defer remote.Close()
	if _, err := dialer.DialContext(context.Background(), "tcp", "unexpected.example:"+port); !errors.Is(err, loginproxy.ErrDestination) {
		t.Fatalf("unapproved direct socket = %v", err)
	}
}
