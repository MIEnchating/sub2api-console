package accountworkbench

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

const oauthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const oauthRedirect = "http://localhost:1455/auth/callback"

func oauthOptions() (browserlogin.OAuthOptions, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return browserlogin.OAuthOptions{}, "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(verifier))
	state, err := randomID()
	if err != nil {
		return browserlogin.OAuthOptions{}, "", err
	}
	query := url.Values{
		"client_id": {oauthClientID}, "response_type": {"code"},
		"redirect_uri": {oauthRedirect}, "state": {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(digest[:])},
		"code_challenge_method": {"S256"}, "scope": {"openid profile email offline_access"},
		"codex_cli_simplified_flow": {"true"}, "id_token_add_organizations": {"true"},
	}
	return browserlogin.OAuthOptions{AuthorizationURL: "https://auth.openai.com/oauth/authorize?" + query.Encode(), State: state, RedirectURI: oauthRedirect}, verifier, nil
}

func (s *Service) exchangeOAuth(ctx context.Context, code, verifier, proxyURL string) (InputItem, error) {
	if code == "" || len(code) > 8192 || strings.ContainsAny(code, "\r\n\x00") {
		return InputItem{}, errors.New("授权码无效，请重新授权")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {oauthClientID}, "redirect_uri": {oauthRedirect}, "code": {code}, "code_verifier": {verifier}}
	return s.requestOAuthTokens(ctx, form, proxyURL)
}

func (s *Service) refreshOfficialOAuth(ctx context.Context, token string) (InputItem, error) {
	if token == "" || len(token) > 65536 || strings.ContainsAny(token, "\r\n\x00") {
		return InputItem{}, errors.New("刷新令牌格式无效，请重新授权")
	}
	item, err := s.requestOAuthTokens(ctx, url.Values{"grant_type": {"refresh_token"}, "client_id": {oauthClientID}, "refresh_token": {token}}, "")
	if err != nil {
		return InputItem{}, errors.New("官方 RT 刷新未取得完整凭据，请核对原授权；本次请求不会自动重发")
	}
	return item, nil
}

func (s *Service) requestOAuthTokens(ctx context.Context, form url.Values, proxyURL string) (InputItem, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://auth.openai.com/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return InputItem{}, errors.New("授权交换请求创建失败")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.GetBody = nil
	request.Close = true
	transport := s.oauthTransport
	if transport == nil && proxyURL != "" {
		isolated, proxyErr := browserlogin.NewProxyTransport(ctx, proxyURL)
		if proxyErr != nil {
			return InputItem{}, errors.New("登录代理连接未就绪，请检查代理后重新授权")
		}
		transport = isolated
		defer isolated.CloseIdleConnections()
	}
	if transport == nil {
		isolated := http.DefaultTransport.(*http.Transport).Clone()
		isolated.Proxy = nil
		isolated.ForceAttemptHTTP2 = false
		transport = isolated
		defer isolated.CloseIdleConnections()
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// Authorization codes are one-use credentials; an uncertain exchange is not replayed.
	response, err := client.Do(request)
	if err != nil {
		return InputItem{}, errors.New("授权交换未完成，请检查网络后重新授权")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return InputItem{}, errors.New("官方授权接口拒绝交换，请重新登录授权")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return InputItem{}, errors.New("官方授权响应无效，请重新授权")
	}
	var tokens struct {
		Access    string          `json:"access_token"`
		Refresh   string          `json:"refresh_token"`
		ID        string          `json:"id_token"`
		TokenType string          `json:"token_type"`
		Expires   json.Number     `json:"expires_in"`
		Error     json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &tokens) != nil || len(tokens.Error) != 0 || tokens.Access == "" || tokens.Refresh == "" || !strings.EqualFold(tokens.TokenType, "bearer") {
		return InputItem{}, errors.New("官方授权响应缺少完整令牌，请重新授权")
	}
	credentials := map[string]any{"access_token": tokens.Access, "refresh_token": tokens.Refresh, "id_token": tokens.ID, "token_type": "Bearer", "client_id": oauthClientID}
	if tokens.Expires != "" {
		seconds, err := tokens.Expires.Int64()
		if err != nil || seconds <= 0 || seconds > 366*24*60*60 {
			return InputItem{}, errors.New("官方授权有效期无效，请重新授权")
		}
		credentials["expires_at"] = time.Now().Add(time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339)
	}
	encoded, _ := json.Marshal(credentials)
	items, failures := Parse(string(encoded))
	if len(items) != 1 || len(failures) != 0 {
		return InputItem{}, errors.New("官方授权凭据校验失败，请重新授权")
	}
	return items[0], nil
}
