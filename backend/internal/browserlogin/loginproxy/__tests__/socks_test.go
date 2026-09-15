package loginproxy_test

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
)

type socksRequest struct {
	username, password, destination string
	err                             error
}

func TestSOCKSProxyAuthenticatesOnceAndUsesPinnedIPWithoutRemoteDNS(t *testing.T) {
	for _, success := range []bool{true, false} {
		name := "success"
		if !success {
			name = "authentication-failure"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			result := make(chan socksRequest, 1)
			go serveSOCKS(listener, success, result)
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			dialer, err := loginproxy.NewDialer(context.Background(), loginproxy.Config{UpstreamURL: "socks5://operator:private-password@proxy.example:" + port, Destinations: map[string]string{"auth.openai.com:443": "8.8.8.8:443"}, Resolve: func(context.Context, string) (string, error) { return "127.0.0.1", nil }})
			if err != nil {
				t.Fatal(err)
			}
			connection, err := dialer.DialContext(context.Background(), "tcp", "auth.openai.com:443")
			if success {
				if err != nil {
					t.Fatal(err)
				}
				_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
				_, err = connection.Write([]byte("official-tls"))
				if err != nil {
					t.Fatal(err)
				}
				got := make([]byte, len("official-tls"))
				if _, err := io.ReadFull(connection, got); err != nil || string(got) != "official-tls" {
					t.Fatalf("SOCKS tunnel payload = %q, %v", got, err)
				}
				_ = connection.Close()
			} else if connection != nil || !errors.Is(err, loginproxy.ErrConnect) {
				if connection != nil {
					connection.Close()
				}
				t.Fatalf("failed SOCKS authentication = %v", err)
			}
			select {
			case observed := <-result:
				if observed.err != nil {
					t.Fatal(observed.err)
				}
				if observed.username != "operator" || observed.password != "private-password" {
					t.Fatal("SOCKS authentication changed configured credentials")
				}
				if success && observed.destination != "8.8.8.8:443" {
					t.Fatalf("SOCKS destination = %s", observed.destination)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("isolated SOCKS transaction did not finish")
			}
		})
	}
}

func serveSOCKS(listener net.Listener, authenticate bool, result chan<- socksRequest) {
	var observed socksRequest
	defer func() { result <- observed }()
	connection, err := listener.Accept()
	if err != nil {
		observed.err = err
		return
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
	read := func(size int) []byte {
		data := make([]byte, size)
		if observed.err == nil {
			_, observed.err = io.ReadFull(connection, data)
		}
		return data
	}
	greeting := read(2)
	if greeting[0] != 5 {
		observed.err = errors.New("expected SOCKS5 greeting")
		return
	}
	methods := read(int(greeting[1]))
	hasPassword := false
	for _, method := range methods {
		if method == 2 {
			hasPassword = true
		}
	}
	if !hasPassword {
		observed.err = errors.New("client omitted password authentication")
		return
	}
	_, _ = connection.Write([]byte{5, 2})
	usernameHeader := read(2)
	if usernameHeader[0] != 1 {
		observed.err = errors.New("expected username/password authentication")
		return
	}
	observed.username = string(read(int(usernameHeader[1])))
	passwordSize := read(1)
	observed.password = string(read(int(passwordSize[0])))
	if !authenticate {
		_, _ = connection.Write([]byte{1, 1})
		return
	}
	_, _ = connection.Write([]byte{1, 0})
	header := read(4)
	if header[0] != 5 || header[1] != 1 || header[2] != 0 || header[3] != 1 {
		observed.err = errors.New("SOCKS target must be a fixed IPv4 CONNECT, never remote DNS")
		return
	}
	ip := net.IP(read(4))
	port := binary.BigEndian.Uint16(read(2))
	observed.destination = (&net.TCPAddr{IP: ip, Port: int(port)}).String()
	_, _ = connection.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	_, _ = io.Copy(connection, connection)
}
