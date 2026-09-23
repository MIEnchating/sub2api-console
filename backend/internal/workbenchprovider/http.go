// Package workbenchprovider implements private account authorization providers.
package workbenchprovider

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const MaxResponseBytes = 2 << 20

type HTTP struct {
	client *http.Client
	single *http.Client
}

type Request struct {
	Method        string
	URL           string
	Header        http.Header
	Body          []byte
	NonReplayable bool
}

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
}

// PublicTransport pins each connection to validated public DNS results. Callers
// must also disable redirects and validate the original HTTPS URL.
func PublicTransport() *http.Transport {
	return &http.Transport{DialContext: publicDial, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 30 * time.Second,
		MaxIdleConns: 8, MaxConnsPerHost: 4, ForceAttemptHTTP2: true}
}

func (e *Error) Error() string { return e.Message }

// NewHTTP keeps credentials on public HTTPS endpoints without proxy inheritance
// or redirects. An explicit transport is the isolated-test network boundary.
func NewHTTP(transport http.RoundTripper) *HTTP {
	if transport == nil {
		transport = PublicTransport()
	}
	singleTransport := transport
	if reusable, ok := transport.(*http.Transport); ok {
		oneShot := reusable.Clone()
		oneShot.DisableKeepAlives = true
		oneShot.ForceAttemptHTTP2 = false
		oneShot.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		oneShot.Protocols = new(http.Protocols)
		oneShot.Protocols.SetHTTP1(true)
		if oneShot.TLSClientConfig == nil {
			oneShot.TLSClientConfig = &tls.Config{}
		}
		oneShot.TLSClientConfig.NextProtos = []string{"http/1.1"}
		singleTransport = oneShot
	}
	client := func(boundary http.RoundTripper) *http.Client {
		return &http.Client{Transport: boundary, Timeout: 20 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &HTTP{client: client(transport), single: client(singleTransport)}
}

func (h *HTTP) Close() {
	h.client.CloseIdleConnections()
	h.single.CloseIdleConnections()
}

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return &Error{Code: "provider_url_invalid", Message: "收码接口需要使用公开 HTTPS 地址的默认端口"}
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || !strings.Contains(host, ".") {
		return &Error{Code: "provider_url_invalid", Message: "收码接口不能使用本地或内网地址"}
	}
	if ip, err := netip.ParseAddr(host); err == nil && !PublicAddress(ip) {
		return &Error{Code: "provider_url_invalid", Message: "收码接口不能使用本地或内网地址"}
	}
	return nil
}

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("2001::/32"),
}

func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errors.New("provider endpoint invalid")
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("provider DNS lookup failed")
	}
	for _, ip := range ips {
		if !PublicAddress(ip) {
			return nil, errors.New("provider endpoint is not public")
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	var last error
	for _, ip := range ips {
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return connection, nil
		}
		last = err
	}
	return nil, last
}

func (h *HTTP) Do(ctx context.Context, input Request) ([]byte, error) {
	if err := ValidateURL(input.URL); err != nil {
		return nil, err
	}
	if input.Method != http.MethodGet && input.Method != http.MethodPost {
		return nil, &Error{Code: "provider_method_invalid", Message: "收码接口仅支持 GET 或 POST"}
	}
	if len(input.Body) > MaxResponseBytes {
		return nil, &Error{Code: "provider_request_too_large", Message: "收码接口请求超过 2 MiB 限制"}
	}
	req, err := http.NewRequestWithContext(ctx, input.Method, input.URL, bytes.NewReader(input.Body))
	if err != nil {
		return nil, &Error{Code: "provider_request_invalid", Message: "收码接口请求无效"}
	}
	req.Header = input.Header.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Accept", "application/json, text/plain, text/html")
	client := h.client
	if input.NonReplayable || input.Method == http.MethodPost {
		// Purchases use GET in several provider APIs. A fresh HTTP/1 connection
		// prevents net/http's transparent replay on reused connections/streams.
		client = h.single
	}
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Code: "provider_connection_failed", Message: "收码服务连接失败或超时，请检查服务配置后重试"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &Error{Code: "provider_http_failed", Message: fmt.Sprintf("收码服务返回 HTTP %d，请检查权限或服务状态", response.StatusCode), HTTPStatus: response.StatusCode}
	}
	if response.ContentLength > MaxResponseBytes {
		return nil, &Error{Code: "provider_response_too_large", Message: "收码服务响应超过 2 MiB 限制"}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, &Error{Code: "provider_response_invalid", Message: "收码服务响应读取失败，请重试"}
	}
	if len(body) > MaxResponseBytes {
		return nil, &Error{Code: "provider_response_too_large", Message: "收码服务响应超过 2 MiB 限制"}
	}
	return body, nil
}
