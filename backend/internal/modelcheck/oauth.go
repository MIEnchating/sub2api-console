package modelcheck

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	oauthResponsesEndpoint = "https://chatgpt.com/backend-api/codex/responses"
	oauthCodexUserAgent    = "codex_cli_rs/0.144.1 (Windows 10.0; x86_64)"
	oauthCheckInstructions = "Follow the output contract exactly. Do not explain, add markdown, or repeat the questions."
)

type oauthCredential struct {
	accessToken string
	workspaceID string
	userAgent   string
}

type oauthBundleSender struct {
	client     *http.Client
	credential oauthCredential
	models     []string
	slots      chan struct{}
}

// UseOAuthTransport injects a trusted transport for isolated integration tests.
// It cannot change the request endpoint and is never configured from API input.
func (s *Service) UseOAuthTransport(transport http.RoundTripper) {
	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	s.oauthTransport = transport
}

// CheckOAuth checks the current Sol profile with a caller-owned OAuth credential.
// Credentials stay in memory; the caller owns task, audit and account state updates.
func (s *Service) CheckOAuth(ctx context.Context, accountID, accountName string, credentials map[string]any, model string, timeoutSeconds int) (map[string]any, error) {
	accountID, accountName, model = strings.TrimSpace(accountID), strings.TrimSpace(accountName), strings.TrimSpace(model)
	if accountID == "" || !validOAuthHeader(accountID, 256) || !utf8.ValidString(accountName) || utf8.RuneCountInString(accountName) > 200 {
		return nil, errors.New("OAuth 检测账号标识或名称无效，请重新选择账号")
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = 25
	}
	if timeoutSeconds < 1 || timeoutSeconds > 120 {
		return nil, errors.New("OAuth 检测超时必须为 1 到 120 秒")
	}
	credential, err := parseOAuthCredential(credentials)
	if err != nil {
		return nil, err
	}
	s.profilesMu.RLock()
	profile := s.solProfile
	version, fingerprint := s.configuration.Active.ID, s.configuration.Active.Fingerprint
	transport := s.oauthTransport
	s.profilesMu.RUnlock()
	if model == "" && len(profile.Models) > 0 {
		model = profile.Models[0]
	}
	if !slices.Contains(profile.Models, model) {
		return nil, errors.New("所选模型不在当前 Sol 检测画像中，请重新选择模型")
	}
	for _, field := range []string{"access_token", "refresh_token", "id_token"} {
		if secret := stringField(credentials, field); secret != "" && (strings.Contains(accountID, secret) || strings.Contains(accountName, secret) || strings.Contains(model, secret)) {
			return nil, errors.New("OAuth 检测账号信息或模型名称不能包含凭据")
		}
	}
	if transport == nil {
		direct := &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: time.Duration(timeoutSeconds) * time.Second,
			MaxConnsPerHost: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
			ForceAttemptHTTP2: true,
		}
		transport = direct
		defer direct.CloseIdleConnections()
	}
	sender := oauthBundleSender{
		client: &http.Client{Transport: transport, Timeout: time.Duration(timeoutSeconds) * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		credential: credential, models: profile.Models, slots: make(chan struct{}, 2),
	}
	result, err := runSolCheck(ctx, sender, profile, targetRequest{
		AccountID: accountID, AccountName: accountName, Model: model, Rounds: 1, TimeoutSeconds: timeoutSeconds,
	})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, errors.New("Sol 检测画像不可用，请检查已发布的检测规则")
	}
	result["profile_version"], result["profile_fingerprint"] = version, fingerprint
	result["transport"] = "oauth-direct"
	result["production_path_equivalent"] = false
	result["scope"] = "closed-set behavioral similarity; not model identity or production-route proof"
	return result, nil
}

func parseOAuthCredential(credentials map[string]any) (oauthCredential, error) {
	credential := oauthCredential{
		accessToken: stringField(credentials, "access_token"),
		workspaceID: stringField(credentials, "chatgpt_account_id"),
		userAgent:   stringField(credentials, "user_agent"),
	}
	if credential.workspaceID == "" {
		credential.workspaceID = stringField(credentials, "account_id")
	}
	if credential.userAgent == "" {
		credential.userAgent = oauthCodexUserAgent
	}
	if credential.accessToken == "" || !validOAuthHeader(credential.accessToken, 65536) || strings.IndexFunc(credential.accessToken, unicode.IsSpace) >= 0 {
		return oauthCredential{}, errors.New("OAuth Access Token 缺失或无效，请先刷新或重新授权账号")
	}
	if !validOAuthHeader(credential.workspaceID, 256) || !validOAuthHeader(credential.userAgent, 512) {
		return oauthCredential{}, errors.New("OAuth 工作区标识或客户端信息无效，请重新授权账号")
	}
	return credential, nil
}

func validOAuthHeader(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func (sender oauthBundleSender) Send(ctx context.Context, _ string, model, prompt string, timeoutSeconds int) (string, string, error) {
	select {
	case sender.slots <- struct{}{}:
		defer func() { <-sender.slots }()
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
	requestContext, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	body, err := json.Marshal(map[string]any{
		"model": model, "input": []map[string]string{{"role": "user", "content": prompt}},
		"instructions": oauthCheckInstructions, "reasoning": map[string]string{"effort": "none"},
		"stream": true, "store": false,
	})
	if err != nil {
		return "", "", visibleRequestError{message: "OAuth 检测请求编码失败"}
	}
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, oauthResponsesEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", visibleRequestError{message: "OAuth 检测请求创建失败"}
	}
	request.Header.Set("Authorization", "Bearer "+sender.credential.accessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("OpenAI-Beta", "responses=experimental")
	request.Header.Set("Originator", "codex_cli_rs")
	request.Header.Set("User-Agent", sender.credential.userAgent)
	if sender.credential.workspaceID != "" {
		request.Header.Set("ChatGPT-Account-Id", sender.credential.workspaceID)
	}
	response, err := sender.client.Do(request)
	if err != nil {
		return "", "", visibleRequestError{message: safeTransportError(err).Error()}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", "", visibleRequestError{message: fmt.Sprintf("OAuth 检测请求失败（HTTP %d），请检查账号授权或稍后重试", response.StatusCode)}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumDirectResponseBytes+1))
	if err != nil {
		return "", "", visibleRequestError{message: safeTransportError(err).Error()}
	}
	if len(raw) > maximumDirectResponseBytes {
		return "", "", visibleRequestError{message: "OAuth 检测响应过大，请稍后重试"}
	}
	payload, err := decodeOAuthResponse(raw)
	if err != nil {
		return "", "", err
	}
	text := openAIResponseText(payload)
	if text == "" {
		return "", "", visibleRequestError{message: "OAuth 检测未返回有效文本，请稍后重试"}
	}
	responseModel := stringField(payload, "model")
	if !slices.Contains(sender.models, responseModel) {
		responseModel = ""
	}
	return text, responseModel, nil
}
