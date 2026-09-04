package upstreamauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

const maximumResponseBytes = 1 << 20

type Client struct {
	http *http.Client
}

type HTTPError struct {
	StatusCode int
	Detail     string
}

type InteractionError struct {
	Code   string
	Detail string
}

type LoginOptions struct {
	AcceptLoginAgreement bool
}

func (e *InteractionError) Error() string { return e.Detail }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("上游鉴权请求失败（HTTP %d）：%s", e.StatusCode, e.Detail)
}

func New(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	copy := *client
	if copy.Timeout == 0 {
		copy.Timeout = 20 * time.Second
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &copy}
}

func ValidateRecord(record configstore.AuthRecord) error {
	platform := strings.ToLower(strings.TrimSpace(record.UpstreamType))
	mode := strings.TrimSpace(record.AuthMode)
	supported := map[string]map[string]struct{}{
		"sub2api": {
			"sub2api_user_token": {}, "sub2api_user_login": {}, "sub2api_manual_login": {},
		},
		"newapi": {
			"newapi_admin_key": {}, "newapi_user_token": {}, "newapi_user_login": {}, "newapi_manual_login": {},
		},
		"oneapi": {
			"newapi_admin_key": {}, "newapi_user_token": {}, "newapi_user_login": {}, "newapi_manual_login": {},
		},
		"custom": {"bearer_token": {}, "custom_headers": {}, "cookie": {}},
	}
	modes, found := supported[platform]
	if !found {
		modes = supported["custom"]
	}
	if _, found := modes[mode]; !found {
		return fmt.Errorf("%s 不支持鉴权方式 %s", platformOrCustom(platform), mode)
	}
	switch mode {
	case "sub2api_user_token":
		if (blank(record.AccessToken) || blank(record.RefreshToken)) && !hasCustomAuthentication(record.Headers) {
			return errors.New("Sub2API Token 鉴权必须配置 Token 和 Refresh Token，或提供包含鉴权信息的自定义 Header")
		}
	case "newapi_admin_key":
		if (blank(record.AdminKey) || blank(record.UserID)) && !hasCustomAuthentication(record.Headers) {
			return errors.New("New API Admin Key 鉴权必须配置 Admin Key 和 User ID，或提供包含鉴权信息的自定义 Header")
		}
	case "newapi_user_token", "bearer_token":
		if blank(record.AccessToken) && !hasCustomAuthentication(record.Headers) {
			return errors.New("令牌鉴权必须配置 Token，或提供包含鉴权信息的自定义 Header")
		}
	case "custom_headers":
		if !hasCustomAuthentication(record.Headers) {
			return errors.New("自定义 Header 鉴权必须至少配置一个 Header")
		}
	case "cookie":
		if len(record.Cookies) == 0 {
			return errors.New("浏览器 Cookie 鉴权必须至少配置一个 Cookie")
		}
	}
	return nil
}

func hasCustomAuthentication(headers map[string]string) bool {
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func (c *Client) Verify(ctx context.Context, record configstore.AuthRecord) error {
	if err := ValidateRecord(record); err != nil {
		return err
	}
	path := "/api/v1/user/profile"
	if isNewAPI(record.UpstreamType) {
		path = "/api/user/self"
	}
	_, _, err := c.request(ctx, record, http.MethodGet, path, nil)
	if err == nil || !isSub2API(record.UpstreamType) || !adminComplianceRequired(err) {
		return err
	}
	if err := c.acceptSub2APIAdminCompliance(ctx, record); err != nil {
		return fmt.Errorf("Sub2API 协议自动同意失败：%w", err)
	}
	_, _, err = c.request(ctx, record, http.MethodGet, path, nil)
	return err
}

func (c *Client) acceptSub2APIAdminCompliance(ctx context.Context, record configstore.AuthRecord) error {
	payload, _, err := c.request(ctx, record, http.MethodGet, "/api/v1/admin/compliance", nil)
	if err != nil {
		return err
	}
	status := nestedObject(payload["data"])
	required, ok := status["required"].(bool)
	if !ok {
		return errors.New("上游协议状态缺少 required")
	}
	if !required {
		return nil
	}
	phrase := nonemptyText(status["ack_phrase_zh"])
	language := "zh"
	if phrase == nil {
		phrase = nonemptyText(status["ack_phrase_en"])
		language = "en"
	}
	if phrase == nil {
		return errors.New("上游协议状态缺少确认短语")
	}
	_, _, err = c.request(ctx, record, http.MethodPost, "/api/v1/admin/compliance/accept", map[string]string{
		"language": language,
		"phrase":   *phrase,
	})
	return err
}

func adminComplianceRequired(err error) bool {
	var httpError *HTTPError
	if !errors.As(err, &httpError) || (httpError.StatusCode != http.StatusForbidden && httpError.StatusCode != http.StatusLocked) {
		return false
	}
	detail := strings.ToLower(strings.TrimSpace(httpError.Detail))
	return strings.Contains(detail, "admin_compliance_ack_required") ||
		detail == "please read" ||
		strings.Contains(detail, "please read and accept") ||
		strings.Contains(detail, "compliance acknowledgement is required")
}

func (c *Client) Login(ctx context.Context, record configstore.AuthRecord, credential configstore.VaultEntry) (configstore.AuthRecord, error) {
	return c.LoginWithOptions(ctx, record, credential, LoginOptions{})
}

func (c *Client) LoginWithOptions(ctx context.Context, record configstore.AuthRecord, credential configstore.VaultEntry, options LoginOptions) (configstore.AuthRecord, error) {
	if blank(credential.Username) || blank(credential.Password) {
		return configstore.AuthRecord{}, errors.New("所选密码箱项缺少完整用户名或密码")
	}
	path, identityField := "/api/v1/auth/login", "email"
	if isNewAPI(record.UpstreamType) {
		path, identityField = "/api/user/login", "username"
	}
	loginRecord := cloneRecord(record)
	loginRecord.AccessToken, loginRecord.AdminKey, loginRecord.UserID = nil, nil, nil
	if loginRecord.Headers == nil {
		loginRecord.Headers = map[string]string{}
	}
	for key, value := range loginRecord.Headers {
		if strings.EqualFold(key, "authorization") && strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "bearer ") {
			delete(loginRecord.Headers, key)
		}
	}
	for key, value := range credential.Headers {
		loginRecord.Headers[key] = value
	}
	loginPayload := map[string]string{
		identityField: *credential.Username, "password": *credential.Password,
	}
	payload, response, err := c.request(ctx, loginRecord, http.MethodPost, path, loginPayload)
	if err != nil && isSub2API(record.UpstreamType) && loginAgreementRequired(err) {
		if !options.AcceptLoginAgreement {
			return configstore.AuthRecord{}, &InteractionError{
				Code:   "login_agreement_required",
				Detail: "上游要求阅读并同意最新登录协议后才能登录",
			}
		}
		revision, revisionErr := c.latestLoginAgreementRevision(ctx, loginRecord)
		if revisionErr != nil {
			return configstore.AuthRecord{}, fmt.Errorf("上游最新登录协议读取失败：%w", revisionErr)
		}
		loginPayload["login_agreement_revision"] = revision
		payload, response, err = c.request(ctx, loginRecord, http.MethodPost, path, loginPayload)
	}
	if err != nil {
		return configstore.AuthRecord{}, classifyLoginError(err)
	}
	data := payload
	if raw, present := payload["data"]; present {
		var ok bool
		data, ok = raw.(map[string]any)
		if !ok {
			return configstore.AuthRecord{}, errors.New("登录响应 data 不可读")
		}
	}
	token := nonemptyText(data["access_token"])
	if token == nil {
		token = nonemptyText(data["token"])
	}
	result := cloneRecord(loginRecord)
	result.AccessToken = token
	result.RefreshToken = nil
	result.AdminKey = nil
	if refresh := nonemptyText(data["refresh_token"]); refresh != nil {
		result.RefreshToken = refresh
	}
	if isNewAPI(record.UpstreamType) {
		if id := nonemptyText(data["id"]); id != nil {
			result.UserID = id
		} else if user, ok := data["user"].(map[string]any); ok {
			if id := nonemptyText(user["id"]); id != nil {
				result.UserID = id
			}
		}
	}
	if result.Cookies == nil {
		result.Cookies = map[string]string{}
	}
	for _, cookie := range response.Cookies() {
		result.Cookies[cookie.Name] = cookie.Value
		if cookie.Name == "new_api_refresh" || cookie.Name == "sub2api_refresh_token" {
			refresh := cookie.Value
			result.RefreshToken = &refresh
		}
	}
	if token == nil {
		if !isNewAPI(record.UpstreamType) || blank(result.UserID) || blankCookie(result.Cookies, "session") {
			return configstore.AuthRecord{}, errors.New("登录响应缺少新的 access token，且未返回可用的旧版 New API Session Cookie")
		}
	}
	if err := c.Verify(ctx, result); err != nil {
		return configstore.AuthRecord{}, fmt.Errorf("登录成功但鉴权复核失败：%w", err)
	}
	return result, nil
}

func (c *Client) latestLoginAgreementRevision(ctx context.Context, record configstore.AuthRecord) (string, error) {
	payload, _, err := c.request(ctx, record, http.MethodGet, "/api/v1/settings/public", nil)
	if err != nil {
		return "", err
	}
	settings := payload
	if raw, present := payload["data"]; present {
		var ok bool
		settings, ok = raw.(map[string]any)
		if !ok {
			return "", errors.New("上游公开设置 data 不可读")
		}
	}
	enabled, ok := settings["login_agreement_enabled"].(bool)
	if !ok || !enabled {
		return "", errors.New("上游公开设置未启用登录协议")
	}
	revision := nonemptyText(settings["login_agreement_revision"])
	if revision == nil || len(*revision) > 255 || strings.ContainsAny(*revision, "\r\n") {
		return "", errors.New("上游公开设置缺少有效的登录协议版本")
	}
	return *revision, nil
}

func loginAgreementRequired(err error) bool {
	var httpError *HTTPError
	if !errors.As(err, &httpError) || (httpError.StatusCode != http.StatusForbidden && httpError.StatusCode != http.StatusLocked) {
		return false
	}
	detail := strings.ToLower(strings.TrimSpace(httpError.Detail))
	return strings.Contains(detail, "login_agreement_required") ||
		strings.Contains(detail, "latest login agreement")
}

func (c *Client) Refresh(ctx context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
	if blank(record.RefreshToken) || strings.ContainsAny(*record.RefreshToken, "\r\n;") {
		return configstore.AuthRecord{}, errors.New("refresh token 无效")
	}
	refreshCookie, path := "sub2api_refresh_token", "/api/v1/auth/refresh"
	var body any
	refreshRecord := cloneRecord(record)
	if refreshRecord.Cookies == nil {
		refreshRecord.Cookies = map[string]string{}
	}
	if isNewAPI(record.UpstreamType) {
		refreshCookie, path = "new_api_refresh", "/api/user/auth/refresh"
	}
	refreshRecord.Cookies[refreshCookie] = strings.TrimSpace(*record.RefreshToken)
	payload, response, err := c.request(ctx, refreshRecord, http.MethodPost, path, body)
	if err != nil && !isNewAPI(record.UpstreamType) && legacySub2APIRefreshBodyRequired(err) {
		payload, response, err = c.request(ctx, refreshRecord, http.MethodPost, path, map[string]string{
			"refresh_token": strings.TrimSpace(*record.RefreshToken),
		})
	}
	if err != nil {
		return configstore.AuthRecord{}, err
	}
	data := payload
	if raw, present := payload["data"]; present {
		var ok bool
		data, ok = raw.(map[string]any)
		if !ok {
			return configstore.AuthRecord{}, errors.New("refresh 响应 data 不可读")
		}
	}
	token := nonemptyText(data["access_token"])
	if token == nil {
		token = nonemptyText(data["token"])
	}
	if token == nil {
		return configstore.AuthRecord{}, errors.New("refresh 响应缺少新的 access token")
	}
	result := cloneRecord(record)
	result.AccessToken = token
	if refresh := nonemptyText(data["refresh_token"]); refresh != nil {
		result.RefreshToken = refresh
	}
	if result.Cookies == nil {
		result.Cookies = map[string]string{}
	}
	for _, cookie := range response.Cookies() {
		result.Cookies[cookie.Name] = cookie.Value
		if cookie.Name == refreshCookie && strings.TrimSpace(cookie.Value) != "" {
			refresh := cookie.Value
			result.RefreshToken = &refresh
		}
	}
	if err := c.Verify(ctx, result); err != nil {
		return configstore.AuthRecord{}, fmt.Errorf("refresh 成功但鉴权复核失败：%w", err)
	}
	return result, nil
}

func legacySub2APIRefreshBodyRequired(err error) bool {
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusBadRequest {
		return false
	}
	detail := strings.ToLower(strings.TrimSpace(httpError.Detail))
	return strings.Contains(detail, "invalid request: eof") ||
		(strings.Contains(detail, "refreshtokenrequest.refreshtoken") && strings.Contains(detail, "required"))
}

func (c *Client) request(ctx context.Context, record configstore.AuthRecord, method, path string, body any) (map[string]any, *http.Response, error) {
	baseURL, err := configstore.ValidateBaseURL(record.BaseURL)
	if err != nil {
		return nil, nil, err
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, errors.New("鉴权请求编码失败")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, reader)
	if err != nil {
		return nil, nil, errors.New("鉴权请求创建失败")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Sub2API-Console/0.1")
	for key, value := range record.Headers {
		request.Header.Set(key, value)
	}
	if isNewAPI(record.UpstreamType) && path == "/api/user/auth/refresh" && request.Header.Get("Origin") == "" {
		request.Header.Set("Origin", request.URL.Scheme+"://"+request.URL.Host)
	}
	if request.Header.Get("Authorization") == "" {
		var token *string
		switch record.AuthMode {
		case "newapi_admin_key":
			token = record.AdminKey
		case "sub2api_user_token", "newapi_user_token", "bearer_token", "sub2api_user_login", "newapi_user_login", "sub2api_manual_login", "newapi_manual_login":
			token = record.AccessToken
		}
		if !blank(token) {
			request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(*token))
		}
	}
	if record.AuthMode != "custom_headers" && record.AuthMode != "cookie" && !blank(record.UserID) {
		request.Header.Set("New-Api-User", strings.TrimSpace(*record.UserID))
	}
	for key, value := range record.Cookies {
		request.AddCookie(&http.Cookie{Name: key, Value: value})
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("上游鉴权网络请求失败：%T", err)
	}
	defer response.Body.Close()
	limited, readErr := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if readErr != nil || len(limited) > maximumResponseBytes {
		return nil, response, errors.New("上游鉴权响应过大或读取失败")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := responseFailureDetail(limited)
		if detail == "" {
			detail = "上游未返回错误详情"
		}
		return nil, response, &HTTPError{StatusCode: response.StatusCode, Detail: detail}
	}
	decoder := json.NewDecoder(bytes.NewReader(limited))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, response, errors.New("上游返回不是可读 JSON 对象")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, response, errors.New("上游鉴权响应包含尾随数据")
	}
	if !businessSuccess(payload) {
		detail := businessFailureDetail(payload)
		if detail == "" {
			return nil, response, errors.New("上游业务鉴权失败")
		}
		return nil, response, errors.New("上游业务鉴权失败：" + detail)
	}
	return payload, response, nil
}

func businessSuccess(payload map[string]any) bool {
	if value, present := payload["code"]; present && !successValue(value, true) {
		return false
	}
	for _, key := range []string{"success", "ok"} {
		if value, present := payload[key]; present && !successValue(value, false) {
			return false
		}
	}
	for _, key := range []string{"error", "errors"} {
		if value, present := payload[key]; present && !emptyValue(value) {
			return false
		}
	}
	if status, ok := payload["status"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "error", "failed", "failure":
			return false
		}
	}
	return true
}

func successValue(value any, code bool) bool {
	if value == true {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	if code && (text == "0" || text == "200") {
		return true
	}
	return text == "1" || text == "200" || text == "true" || text == "success" || text == "succeeded" || text == "ok"
}

func emptyValue(value any) bool {
	if value == nil || value == "" {
		return true
	}
	switch item := value.(type) {
	case []any:
		return len(item) == 0
	case map[string]any:
		return len(item) == 0
	}
	return false
}

func businessFailureDetail(payload map[string]any) string {
	for _, source := range []map[string]any{payload, nestedObject(payload["data"])} {
		for _, key := range []string{"message", "msg", "reason", "code", "detail", "error"} {
			value, found := source[key]
			if !found {
				continue
			}
			text, ok := value.(string)
			text = sanitizeAuthDetail(text)
			if ok && text != "" {
				return text
			}
		}
	}
	return ""
}

func responseFailureDetail(raw []byte) string {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	seen := map[string]struct{}{}
	for _, source := range []map[string]any{payload, nestedObject(payload["data"])} {
		for _, key := range []string{"message", "msg", "reason", "code", "detail", "error"} {
			value, ok := source[key].(string)
			if !ok {
				continue
			}
			value = sanitizeAuthDetail(value)
			if value == "" {
				continue
			}
			if _, duplicate := seen[value]; duplicate {
				continue
			}
			seen[value] = struct{}{}
			parts = append(parts, value)
			if len(parts) == 3 {
				return strings.Join(parts, "；")
			}
		}
	}
	return strings.Join(parts, "；")
}

func sanitizeAuthDetail(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	value = redact.Secrets(value)
	if len(value) > 300 {
		value = value[:300]
	}
	return value
}

func nestedObject(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func classifyLoginError(err error) error {
	var httpError *HTTPError
	detail := err.Error()
	if errors.As(err, &httpError) {
		detail = httpError.Detail
	}
	lower := strings.ToLower(detail)
	if loginAgreementRequired(err) {
		return &InteractionError{Code: "login_agreement_required", Detail: "上游要求阅读并同意最新登录协议后才能登录"}
	}
	if strings.Contains(lower, "credential_browser_flow_required") || strings.Contains(lower, "browser credential flow") {
		return &InteractionError{Code: "image_captcha_required", Detail: "登录要求加密凭据和图片验证码"}
	}
	if strings.Contains(lower, "turnstile") || strings.Contains(lower, "cloudflare") || strings.Contains(lower, "managed challenge") || strings.Contains(lower, "cf-chl") {
		return &InteractionError{Code: "browser_challenge_required", Detail: "登录触发浏览器人机验证"}
	}
	if strings.Contains(lower, "captcha") || strings.Contains(lower, "验证码") || strings.Contains(lower, "image_captcha") {
		return &InteractionError{Code: "image_captcha_required", Detail: "登录需要图片验证码"}
	}
	return err
}

func cloneRecord(value configstore.AuthRecord) configstore.AuthRecord {
	result := value
	result.Headers = cloneMap(value.Headers)
	result.Cookies = cloneMap(value.Cookies)
	return result
}

func cloneMap(value map[string]string) map[string]string {
	result := map[string]string{}
	for key, item := range value {
		result[key] = item
	}
	return result
}

func nonemptyText(value any) *string {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return nil
	}
	return &text
}

func blank(value *string) bool { return value == nil || strings.TrimSpace(*value) == "" }
func blankCookie(values map[string]string, name string) bool {
	for key, value := range values {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value) == ""
		}
	}
	return true
}
func isNewAPI(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "newapi" || value == "oneapi"
}
func isSub2API(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "sub2api")
}
func platformOrCustom(value string) string {
	if value == "" {
		return "custom"
	}
	return value
}
