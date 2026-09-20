package accountworkbench

// This file contains the AccountWorkbench login protocol. It intentionally
// uses a private cookie jar and HTTP requests; no page is opened and no
// browser worker is involved in the account import path.

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

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

const (
	protocolChatGPT = "https://chatgpt.com"
	protocolAuth    = "https://auth.openai.com"
	maxProtocolBody = 2 << 20
)

func protocolJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

func protocolPayload(raw []byte) (map[string]any, error) {
	var payload map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil || payload == nil || decoder.Decode(new(any)) != io.EOF || payload["error"] != nil {
		return nil, errors.New("官方登录返回内容无效")
	}
	return payload, nil
}

func protocolURL(payload map[string]any) string {
	for _, key := range []string{"continue_url", "url"} {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}
	if page, ok := payload["page"].(map[string]any); ok {
		if nested, ok := page["payload"].(map[string]any); ok {
			if value, ok := nested["url"].(string); ok {
				return value
			}
		}
	}
	return ""
}

func (s *Service) authorizeProtocol(ctx context.Context, value *privateRun, index int, active *activeRun, proxy string) (map[string]any, error) {
	ctx, stop := context.WithTimeout(ctx, 15*time.Minute)
	defer stop()
	row := &value.Public.Items[index]
	assistance, err := s.prepareLoginAssistance(ctx, value, index)
	if err != nil {
		return nil, err
	}
	defer assistance.close()
	details := assistance.details
	client, err := newProtocolClient(ctx, proxy, s.officialTransport, func() error { return s.executionAllowed(ctx, value) })
	if err != nil {
		return nil, err
	}
	defer client.close()
	row.Status = "authorizing"
	row.LoginPrompt = nil
	row.Message = "正在通过官方协议登录"
	if err := s.persistRun(value); err != nil {
		return nil, errors.New("授权阶段保存失败")
	}

	csrfPage, err := client.follow(ctx, protocolChatGPT+"/", protocolChatGPT+"/")
	if err != nil {
		return nil, err
	}
	if csrfPage.URL == "" {
		return nil, errors.New("官方登录首页未返回")
	}
	if _, err := client.request(ctx, http.MethodGet, protocolChatGPT+"/api/auth/providers", nil, nil); err != nil {
		return nil, err
	}
	csrfResponse, err := client.request(ctx, http.MethodGet, protocolChatGPT+"/api/auth/csrf", nil, http.Header{"Referer": {protocolChatGPT + "/"}})
	if err != nil {
		return nil, err
	}
	csrf, err := protocolPayload(csrfResponse.Body)
	if err != nil || text(csrf["csrfToken"]) == "" || !client.hasCookie(protocolChatGPT, "__Host-next-auth.csrf-token") {
		return nil, errors.New("官方登录会话初始化失败")
	}
	deviceID := ""
	for _, cookie := range client.jar.Cookies(mustURL(protocolChatGPT)) {
		if cookie.Name == "oai-did" {
			deviceID = cookie.Value
		}
	}
	if deviceID == "" {
		deviceID = uuid.NewString()
	}
	assistance.requestedAt = time.Now().UTC()
	signinURL := protocolChatGPT + "/api/auth/signin/openai?" + url.Values{"prompt": {"login"}, "ext-oai-did": {deviceID}, "auth_session_logging_id": {uuid.NewString()}, "screen_hint": {"login_or_signup"}, "login_hint": {details.Email}}.Encode()
	signin, err := client.request(ctx, http.MethodPost, signinURL, []byte(url.Values{"callbackUrl": {protocolChatGPT + "/"}, "csrfToken": {text(csrf["csrfToken"])}, "json": {"true"}}.Encode()), http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Referer": {protocolChatGPT + "/"}, "Origin": {protocolChatGPT}})
	if err != nil {
		return nil, err
	}
	signinPayload, err := protocolPayload(signin.Body)
	if err != nil || text(signinPayload["url"]) == "" {
		return nil, errors.New("官方登录地址未返回")
	}
	authPage, err := client.follow(ctx, text(signinPayload["url"]), protocolChatGPT+"/")
	if err != nil {
		return nil, err
	}
	authPayload := map[string]any{}
	if protocolPath(authPage.URL) == "/log-in/password" {
		row.Message = "正在验证登录密码"
		if err := s.persistRun(value); err != nil {
			return nil, err
		}
		authPayload, err = s.verifyProtocolFactor(ctx, client, value, index, active, protocolFactor{kind: "password", flow: "password_verify", device: deviceID, path: "/api/accounts/password/verify", referer: authPage.URL, field: "password", initial: details.Password})
	} else if protocolPath(authPage.URL) == "/email-verification" {
		row.Message = "正在等待邮箱验证码"
		code, codeErr := s.protocolMailCode(ctx, assistance, value)
		if codeErr != nil {
			return nil, codeErr
		}
		authPayload, err = s.verifyProtocolFactor(ctx, client, value, index, active, protocolFactor{kind: "email_code", flow: "email_otp_validate", device: deviceID, path: "/api/accounts/email-otp/validate", referer: authPage.URL, field: "code", initial: code})
	} else if client.completedLogin(authPage.URL) {
		authPayload = map[string]any{"continue_url": authPage.URL}
	} else {
		return nil, protocolUnexpectedLoginPage(authPage.URL)
	}
	if err != nil {
		return nil, err
	}
	if isProtocolMFA(authPayload) {
		factorID := protocolFactorID(authPayload)
		if factorID == "" {
			return nil, errors.New("官方 2FA 响应缺少验证因子")
		}
		_, err = client.request(ctx, http.MethodPost, protocolAuth+"/api/accounts/mfa/issue_challenge", protocolJSON(map[string]any{"type": "totp", "id": factorID, "force_fresh_challenge": false}), protocolHeaders(protocolURL(authPayload)))
		if err != nil {
			return nil, err
		}
		var code string
		if details.TOTP != "" {
			code, err = totp.GenerateCode(details.TOTP, time.Now().UTC())
		}
		if err != nil {
			return nil, err
		}
		referer := protocolURL(authPayload)
		if referer == "" {
			referer = protocolAuth + "/mfa-challenge/" + url.PathEscape(factorID)
		}
		authPayload, err = s.verifyProtocolFactor(ctx, client, value, index, active, protocolFactor{kind: "totp_code", flow: "password_verify", device: deviceID, path: "/api/accounts/mfa/verify", referer: referer, field: "code", initial: code, fields: map[string]any{"type": "totp", "id": factorID}})
		if err != nil {
			return nil, err
		}
	}
	if protocolPage(authPayload) == "workspace" || protocolPath(protocolURL(authPayload)) == "/workspace" {
		authPayload, err = client.selectWorkspace(ctx, authPayload)
		if err != nil {
			return nil, err
		}
	}
	if protocolPage(authPayload) == "add_phone" || protocolPage(authPayload) == "about_you" {
		return nil, errors.New("官方要求补充账号资料，请在官方站点完成后重新授权")
	}
	if next := protocolURL(authPayload); next != "" && !strings.Contains(next, "/oauth/authorize") {
		authPage, err = client.follow(ctx, next, authPage.URL)
		if err != nil {
			return nil, err
		}
	}
	return s.finishProtocolOAuth(ctx, client, value, index, authPage.URL)
}

func protocolHeaders(referer string) http.Header {
	return http.Header{"Referer": {referer}, "Origin": {protocolAuth}}
}

func mustURL(raw string) *url.URL { parsed, _ := url.Parse(raw); return parsed }

func (s *Service) protocolMailCode(ctx context.Context, assistance *loginAssistance, value *privateRun) (string, error) {
	if assistance.mailbox == nil {
		return "", nil
	}
	if err := s.persistRun(value); err != nil {
		return "", err
	}
	for attempt := 0; attempt < 20; attempt++ {
		if err := s.executionAllowed(ctx, value); err != nil {
			return "", err
		}
		candidates, err := assistance.mailbox.Fetch(ctx, assistance.requestedAt, assistance.baseline)
		if assistance.fatal != nil {
			return "", assistance.fatal
		}
		if err == nil && len(candidates) == 1 {
			return candidates[0].Code, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return "", nil
}

func (s *Service) finishProtocolOAuth(ctx context.Context, client *protocolClient, value *privateRun, index int, referer string) (map[string]any, error) {
	verifierBytes := make([]byte, 48)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, errors.New("授权参数生成失败")
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	hash := sha256.Sum256([]byte(verifier))
	stateBytes := make([]byte, 24)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, errors.New("授权状态生成失败")
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	authURL := protocolAuth + "/oauth/authorize?" + url.Values{"client_id": {officialClientID}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}, "code_challenge_method": {"S256"}, "codex_cli_simplified_flow": {"true"}, "id_token_add_organizations": {"true"}, "redirect_uri": {officialRedirect}, "response_type": {"code"}, "scope": {"openid profile email offline_access"}, "state": {state}}.Encode()
	page, err := client.follow(ctx, authURL, referer)
	if err != nil {
		return nil, err
	}
	if code, returnedState := protocolCallback(page.URL); code != "" {
		if returnedState != state {
			return nil, errors.New("官方授权状态校验失败")
		}
		return s.exchangeProtocolCode(ctx, client, value, index, code, verifier)
	}
	sessionID := protocolSessionID(page.Body)
	if sessionID == "" {
		return nil, errors.New("官方授权未返回账号会话，请重新登录")
	}
	selected, err := client.request(ctx, http.MethodPost, protocolAuth+"/api/accounts/session/select", protocolJSON(map[string]any{"session_id": sessionID}), protocolHeaders(page.URL))
	if err != nil {
		return nil, err
	}
	payload, err := protocolPayload(selected.Body)
	if err != nil {
		return nil, err
	}
	payload, err = client.selectWorkspace(ctx, payload)
	if err != nil {
		return nil, err
	}
	continueURL := protocolURL(payload)
	if continueURL == "" {
		return nil, errors.New("官方授权未返回继续地址")
	}
	callback, err := client.follow(ctx, continueURL, page.URL)
	if err != nil {
		return nil, err
	}
	code, returnedState := protocolCallback(callback.URL)
	if code == "" || returnedState != state {
		return nil, errors.New("官方授权回调缺少有效授权码")
	}
	return s.exchangeProtocolCode(ctx, client, value, index, code, verifier)
}
