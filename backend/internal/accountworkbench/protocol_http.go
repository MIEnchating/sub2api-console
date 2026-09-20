package accountworkbench

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
	"golang.org/x/net/publicsuffix"
)

type protocolResponse struct {
	URL  string
	Body []byte
}
type protocolClient struct {
	http      *http.Client
	jar       http.CookieJar
	transport *http.Transport
	allowed   func() error
}

func newProtocolClient(ctx context.Context, proxy string, injected http.RoundTripper, allowed func() error) (*protocolClient, error) {
	if err := loginproxy.Validate(proxy); err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, errors.New("协议登录会话初始化失败")
	}
	var transport *http.Transport
	if injected == nil {
		destinations := map[string]string{}
		resolve, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		for _, host := range []string{"chatgpt.com", "auth.openai.com", "sentinel.openai.com"} {
			address, err := loginproxy.PublicAddress(resolve, host)
			if err != nil {
				return nil, err
			}
			destinations[host+":443"] = net.JoinHostPort(address, "443")
		}
		dialer, err := loginproxy.NewDialer(resolve, loginproxy.Config{UpstreamURL: proxy, Destinations: destinations})
		if err != nil {
			return nil, err
		}
		injected = &protocolTransport{dialer: dialer}
	}
	return &protocolClient{http: &http.Client{Transport: injected, Jar: jar, Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, jar: jar, transport: transport, allowed: allowed}, nil
}
func (p *protocolClient) close() {
	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
}
func protocolEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" || u.Scheme != "https" || u.Fragment != "" {
		return false
	}
	return u.Host == "auth.openai.com" || u.Host == "chatgpt.com" || u.Host == "sentinel.openai.com"
}
func (p *protocolClient) request(ctx context.Context, method, rawURL string, body []byte, headers http.Header) (protocolResponse, error) {
	if !protocolEndpoint(rawURL) {
		return protocolResponse{}, errors.New("官方登录跳转超出允许的站点，已停止授权")
	}
	if err := p.allowed(); err != nil {
		return protocolResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, strings.NewReader(string(body)))
	if err != nil {
		return protocolResponse{}, errors.New("协议登录请求创建失败")
	}
	req.GetBody = nil
	req.Close = true
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="133", "Google Chrome";v="133", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range headers {
		req.Header[key] = append([]string(nil), values...)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return protocolResponse{}, errors.New("官方协议请求结果未确认，本次请求不会自动重发")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProtocolBody+1))
	if err != nil || len(raw) > maxProtocolBody {
		return protocolResponse{}, errors.New("官方协议响应不完整，本次请求不会自动重发")
	}
	if protocolSecurityChallenge(resp.StatusCode, resp.Header, raw) {
		return protocolResponse{}, errors.New("官方登录触发了安全验证，请检查当前网络或登录代理后重新授权；本次请求不会自动重发")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return protocolResponse{}, protocolError(resp.StatusCode, raw, req.URL.Path)
	}
	if resp.StatusCode >= 300 {
		if method != http.MethodGet {
			return protocolResponse{}, errors.New("官方登录提交返回意外跳转，本次请求不会自动重发")
		}
		target, err := req.URL.Parse(resp.Header.Get("Location"))
		if err != nil || resp.Header.Get("Location") == "" {
			return protocolResponse{}, errors.New("官方登录跳转地址无效")
		}
		return protocolResponse{URL: target.String(), Body: raw}, nil
	}
	return protocolResponse{URL: rawURL, Body: raw}, nil
}
func (p *protocolClient) follow(ctx context.Context, rawURL, referer string) (protocolResponse, error) {
	current := rawURL
	for step := 0; step < 12; step++ {
		if protocolIsCallback(current) {
			return protocolResponse{URL: current}, nil
		}
		// Page navigation must negotiate HTML, as in AccountWorkbench's follow().
		// A JSON-preferred request may stop at the authorization endpoint instead
		// of receiving the redirect to the password or email verification page.
		headers := http.Header{"Accept": {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"}}
		if protocolEndpoint(referer) {
			u, _ := url.Parse(referer)
			u.RawQuery = ""
			headers.Set("Referer", u.String())
		}
		response, err := p.request(ctx, http.MethodGet, current, nil, headers)
		if err != nil {
			return protocolResponse{}, err
		}
		if response.URL == current {
			return response, nil
		}
		referer, current = current, response.URL
	}
	return protocolResponse{}, errors.New("官方登录跳转次数过多，已停止授权")
}
func (p *protocolClient) hasCookie(origin, name string) bool {
	for _, c := range p.jar.Cookies(mustURL(origin)) {
		if c.Name == name && c.Value != "" {
			return true
		}
	}
	return false
}
func (p *protocolClient) verify(ctx context.Context, flow, device, path string, payload map[string]any, referer string) (protocolResponse, error) {
	headers, err := p.securityHeaders(ctx, flow, device)
	if err != nil {
		return protocolResponse{}, err
	}
	headers.Set("Referer", referer)
	headers.Set("Origin", protocolAuth)
	return p.request(ctx, http.MethodPost, protocolAuth+path, protocolJSON(payload), headers)
}
