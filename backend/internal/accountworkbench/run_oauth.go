package accountworkbench

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func equalEmail(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
func authorizationOptions(proxy string) (browserlogin.OAuthOptions, string) {
	bytes := make([]byte, 32)
	_, _ = rand.Read(bytes)
	verifier := base64.RawURLEncoding.EncodeToString(bytes)
	challenge := sha256.Sum256([]byte(verifier))
	state := newID()
	query := url.Values{"client_id": {officialClientID}, "response_type": {"code"}, "redirect_uri": {officialRedirect}, "state": {state}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "scope": {"openid profile email offline_access"}, "codex_cli_simplified_flow": {"true"}, "id_token_add_organizations": {"true"}}
	return browserlogin.OAuthOptions{AuthorizationURL: "https://auth.openai.com/oauth/authorize?" + query.Encode(), State: state, RedirectURI: officialRedirect, ProxyURL: proxy}, verifier
}
func (s *Service) authorizeItem(ctx context.Context, value *privateRun, index int, active *activeRun, proxy string) (map[string]any, error) {
	row := &value.Public.Items[index]
	row.Status = "authorizing"
	row.Message = "正在启动授权登录"
	if err := s.persistRun(value); err != nil {
		return nil, errors.New("授权阶段保存失败")
	}
	assistance, err := s.prepareLoginAssistance(ctx, value, index)
	if err != nil {
		return nil, err
	}
	defer assistance.close()
	options, verifier := authorizationOptions(proxy)
	browserCtx, cancel := context.WithTimeout(ctx, browserlogin.Lifetime)
	defer cancel()
	browser, err := s.browser.OpenOAuth(browserCtx, options)
	if err != nil {
		return nil, errors.New("授权浏览器启动失败，请确认 browser 服务可用后重试")
	}
	active.mu.Lock()
	active.browser = browser
	active.browserItem = row.ID
	active.mu.Unlock()
	defer func() {
		active.mu.Lock()
		active.browser = nil
		active.browserItem = ""
		browser.Close()
		active.mu.Unlock()
		row.BrowserReady = false
	}()
	row.BrowserReady = true
	row.Status = "waiting_input"
	row.Message = "请完成官方登录页面中的验证"
	if err = s.persistRun(value); err != nil {
		return nil, errors.New("授权会话保存失败")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err = s.executionAllowed(browserCtx, value); err != nil {
			return nil, errors.New("授权会话已停止或到期")
		}
		active.mu.Lock()
		result, resultErr := browser.AuthorizationCode(browserCtx)
		active.mu.Unlock()
		if errors.Is(resultErr, browserlogin.ErrOAuthPending) {
			if err = assistance.apply(browserCtx, active); err != nil {
				return nil, err
			}
		}
		if resultErr == nil {
			if result.State != options.State {
				return nil, ErrIdentity
			}
			value.Phases[row.ID] = "exchanging"
			if err = s.persistRun(value); err != nil {
				return nil, errors.New("授权交换前保存失败")
			}
			exchange, stop := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
			credentials, err := s.exchangeCode(exchange, result.Code, verifier, proxy)
			stop()
			return credentials, err
		}
		if !errors.Is(resultErr, browserlogin.ErrOAuthPending) {
			return nil, errors.New("官方授权尚未完成，请重新登录")
		}
		select {
		case <-browserCtx.Done():
			return nil, errors.New("授权登录已到期或取消")
		case <-ticker.C:
		}
	}
}
func (s *Service) activeBrowser(ctx context.Context, owner, id, itemID string) (*activeRun, error) {
	record, err := s.readRun(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if !record.Public.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrRun
	}
	s.activeMu.Lock()
	active := s.active[id]
	s.activeMu.Unlock()
	if active == nil || active.owner != owner {
		return nil, browserlogin.ErrSession
	}
	active.mu.Lock()
	if active.browser == nil || active.browserItem != itemID {
		active.mu.Unlock()
		return nil, browserlogin.ErrSession
	}
	return active, nil
}
func (s *Service) BrowserImage(ctx context.Context, owner, id, itemID string) ([]byte, error) {
	active, err := s.activeBrowser(ctx, owner, id, itemID)
	if err != nil {
		return nil, err
	}
	defer active.mu.Unlock()
	return active.browser.Screenshot(ctx)
}
func (s *Service) BrowserInput(ctx context.Context, owner, id, itemID string, input browserlogin.Input) error {
	if err := input.Validate(); err != nil {
		return err
	}
	active, err := s.activeBrowser(ctx, owner, id, itemID)
	if err != nil {
		return err
	}
	defer active.mu.Unlock()
	return active.browser.Input(ctx, input)
}
func publicCheck(result map[string]any, credentials map[string]any) map[string]any {
	raw, err := json.Marshal(result)
	if err != nil {
		return map[string]any{"verdict": "INCONCLUSIVE"}
	}
	encoded := string(raw)
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if secret := text(credentials[key]); secret != "" {
			quoted, _ := json.Marshal(secret)
			escaped := string(quoted[1 : len(quoted)-1])
			encoded = strings.ReplaceAll(encoded, escaped, "[已隐藏]")
		}
	}
	output := map[string]any{}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.UseNumber()
	if decoder.Decode(&output) != nil {
		return map[string]any{"verdict": "INCONCLUSIVE"}
	}
	return output
}
