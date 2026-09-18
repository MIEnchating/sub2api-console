package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

const officialClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const officialRedirect = "http://localhost:1455/auth/callback"

// UseOfficialTransport changes the network boundary only; endpoints remain fixed.
func (s *Service) UseOfficialTransport(transport http.RoundTripper) { s.officialTransport = transport }

// RefreshCredential makes exactly one token request. The caller must durably
// save a successful rotation before checking cancellation or doing other work.
func (s *Service) RefreshCredential(ctx context.Context, token, proxyURL string) (map[string]any, error) {
	if token == "" || len(token) > 32768 || strings.ContainsAny(token, "\r\n\x00") {
		return nil, errors.New("刷新令牌格式无效，请重新输入")
	}
	return s.officialTokenRequest(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}, "client_id": {officialClientID}}, proxyURL)
}
func (s *Service) exchangeCode(ctx context.Context, code, verifier, proxyURL string) (map[string]any, error) {
	if code == "" || len(code) > 8192 || strings.ContainsAny(code, "\r\n\x00") || len(verifier) < 43 || len(verifier) > 128 {
		return nil, errors.New("授权交易无效，请重新登录")
	}
	return s.officialTokenRequest(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "client_id": {officialClientID}, "redirect_uri": {officialRedirect}}, proxyURL)
}
func (s *Service) officialTokenRequest(ctx context.Context, form url.Values, proxyURL string) (map[string]any, error) {
	if err := browserlogin.ValidateProxyURL(proxyURL); err != nil {
		return nil, errors.New("登录代理格式无效")
	}
	transport := s.officialTransport
	if transport == nil {
		private, err := browserlogin.NewProxyTransport(ctx, proxyURL)
		if err != nil {
			return nil, errors.New("官方授权连接未就绪，请检查网络或代理")
		}
		defer private.CloseIdleConnections()
		transport = private
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://auth.openai.com/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.New("官方授权请求创建失败")
	}
	request.GetBody = nil
	request.Close = true
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	client := http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("官方授权请求结果未确认，请重新授权；本次请求不会自动重发")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("官方授权接口拒绝请求，请核对授权资料；本次请求不会自动重发")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("官方授权响应不完整，请重新授权")
	}
	var payload map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF || payload["error"] != nil {
		return nil, errors.New("官方授权响应无效，请重新授权")
	}
	credentials, ok := pickInputCredentials(payload)
	if !ok || text(credentials["access_token"]) == "" || text(credentials["refresh_token"]) == "" || !strings.EqualFold(text(credentials["token_type"]), "bearer") {
		return returnedTokenFields(payload), errors.New("官方授权响应缺少完整凭据，已返回的凭据需核对后再使用")
	}
	if expires := text(credentials["expires_in"]); expires != "" {
		seconds, err := json.Number(expires).Int64()
		if err != nil || seconds <= 0 || seconds > 366*86400 {
			return credentials, errors.New("官方授权有效期无效，已返回的凭据需核对后再使用")
		}
		credentials["expires_at"] = time.Now().UTC().Add(time.Duration(seconds) * time.Second).Format(time.RFC3339)
	}
	credentials["client_id"] = officialClientID
	enrichInputIdentity(credentials)
	return credentials, nil
}

func returnedTokenFields(payload map[string]any) map[string]any {
	result := map[string]any{}
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		value, ok := payload[key].(string)
		if ok && value != "" && len(value) <= 32768 && !strings.ContainsAny(value, "\r\n\x00") {
			result[key] = value
		}
	}
	return result
}
