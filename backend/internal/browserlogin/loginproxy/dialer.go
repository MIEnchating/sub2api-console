package loginproxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

type Config struct {
	Destinations map[string]string
	UpstreamURL  string
	// Resolve replaces the public-address boundary only for isolated tests.
	Resolve Resolver
	// RootCAs optionally supplies an explicit trust store for an HTTPS proxy.
	RootCAs *x509.CertPool
}

type Dialer struct {
	destinations map[string]string
	upstream     *url.URL
	address      string
	rootCAs      *x509.CertPool
}

func NewDialer(ctx context.Context, config Config) (*Dialer, error) {
	u, err := parse(config.UpstreamURL)
	if err != nil {
		return nil, err
	}
	d := &Dialer{destinations: make(map[string]string, len(config.Destinations)), upstream: u, rootCAs: config.RootCAs}
	for host, destination := range config.Destinations {
		name, port, err := net.SplitHostPort(host)
		ip, parseErr := netip.ParseAddrPort(destination)
		if err != nil || name == "" || parseErr != nil || ip.Port() == 0 || port != strconv.Itoa(int(ip.Port())) || (config.Resolve == nil && !isPublic(ip.Addr())) {
			return nil, ErrDestination
		}
		d.destinations[host] = destination
	}
	if u == nil {
		return d, nil
	}
	if u.User != nil && strings.Contains(u.User.Username(), "{session}") {
		return nil, ErrConfig
	}
	resolver := config.Resolve
	if resolver == nil {
		resolver = PublicAddress
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	address, err := resolver(resolveCtx, u.Hostname())
	if err != nil {
		return nil, ErrConnect
	}
	if ip, err := netip.ParseAddr(address); err != nil || ip.Zone() != "" {
		return nil, ErrConfig
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		case "socks5":
			port = "1080"
		}
	}
	d.address = net.JoinHostPort(address, port)
	return d, nil
}

func (d *Dialer) DialContext(ctx context.Context, network, host string) (net.Conn, error) {
	address, allowed := d.destinations[host]
	if !allowed || network != "tcp" {
		return nil, ErrDestination
	}
	if d.upstream == nil {
		connection, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, connectError(ctx)
		}
		return connection, nil
	}
	handshake, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var connection net.Conn
	var err error
	if d.upstream.Scheme == "socks5" {
		connection, err = d.dialSOCKS(handshake, address)
	} else {
		connection, err = d.dialHTTP(handshake, address)
	}
	if err != nil {
		return nil, connectError(ctx)
	}
	return connection, nil
}

func connectError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrConnect
}

func (d *Dialer) dialSOCKS(ctx context.Context, destination string) (net.Conn, error) {
	var auth *proxy.Auth
	if d.upstream.User != nil {
		password, _ := d.upstream.User.Password()
		auth = &proxy.Auth{User: d.upstream.User.Username(), Password: password}
	}
	dialer, err := proxy.SOCKS5("tcp", d.address, auth, &net.Dialer{Timeout: 10 * time.Second})
	if err != nil {
		return nil, err
	}
	return dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", destination)
}

func (d *Dialer) dialHTTP(ctx context.Context, destination string) (connection net.Conn, err error) {
	connection, err = (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", d.address)
	if err != nil {
		return nil, err
	}
	raw := connection
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer func() {
		stop()
		if err != nil {
			_ = raw.Close()
		}
	}()
	if d.upstream.Scheme == "https" {
		tlsConnection := tls.Client(connection, &tls.Config{ServerName: d.upstream.Hostname(), MinVersion: tls.VersionTLS12, RootCAs: d.rootCAs})
		if err = tlsConnection.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		connection = tlsConnection
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Host: destination}, Host: destination, Header: make(http.Header)}
	if d.upstream.User != nil {
		password, _ := d.upstream.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(d.upstream.User.Username()+":"+password)))
	}
	if err = request.Write(connection); err != nil {
		return nil, err
	}
	// Bound CONNECT response headers without discarding bytes already buffered
	// after them, which belong to the established TLS tunnel.
	limited := &io.LimitedReader{R: connection, N: 16 << 10}
	reader := bufio.NewReader(limited)
	response, readErr := http.ReadResponse(reader, request)
	if readErr != nil || response.StatusCode != http.StatusOK || response.ProtoMajor != 1 {
		return nil, ErrConnect
	}
	limited.N = math.MaxInt64
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &bufferedConn{Conn: connection, reader: reader}, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

func (d *Dialer) Transport() *http.Transport {
	return &http.Transport{
		DialContext: d.DialContext, DisableKeepAlives: true,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 20 * time.Second,
		MaxResponseHeaderBytes: 1 << 20, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
}
