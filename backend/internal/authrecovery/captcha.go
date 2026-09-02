package authrecovery

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

const (
	captchaAlgorithm        = "RSA-OAEP-256+A256GCM"
	captchaBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	maximumCaptchaResponse  = 12 << 20
	maximumImageBytes       = 8 << 20
)

type CaptchaVerifier interface {
	Verify(context.Context, configstore.AuthRecord) error
}

type CaptchaCatalogReader interface {
	ReadCatalog(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error)
}

type CaptchaPrivateStore interface {
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
	SaveAuthRecord(context.Context, configstore.AuthRecord, map[string]bool) error
	VaultEntry(context.Context, string) (*configstore.VaultEntry, error)
	SaveVaultEntry(context.Context, configstore.VaultEntry, map[string]bool) error
}

type captchaUpstreamStore interface {
	UpstreamExists(context.Context, string) (bool, error)
}

type CaptchaChallenge struct {
	ChallengeID     string            `json:"challenge_id"`
	Host            string            `json:"host"`
	ImageData       string            `json:"image_data"`
	ExpiresAt       string            `json:"expires_at"`
	Credential      map[string]string `json:"credential"`
	InteractionKind string            `json:"interaction_kind"`
}

type CaptchaResult struct {
	Success         bool    `json:"success"`
	Host            string  `json:"host"`
	ProfileStatus   string  `json:"profile_status"`
	Balance         any     `json:"balance"`
	Concurrency     any     `json:"concurrency"`
	Keys            int     `json:"keys"`
	Groups          int     `json:"groups"`
	Stored          bool    `json:"stored"`
	InteractionKind string  `json:"interaction_kind"`
	ParentTaskID    *string `json:"-"`
}

type storedChallenge struct {
	public       CaptchaChallenge
	record       configstore.AuthRecord
	baseURL      string
	entry        string
	captchaID    string
	keyID        string
	publicKey    string
	serverOffset int64
	cookies      map[string]string
	headers      map[string]string
	expiresAt    time.Time
	parentTaskID *string
	credential   configstore.VaultEntry
	saveToVault  bool
}

type CaptchaManager struct {
	private            CaptchaPrivateStore
	verifier           CaptchaVerifier
	catalog            CaptchaCatalogReader
	http               *http.Client
	now                func() time.Time
	mutationRepository any
	mu                 sync.Mutex
	items              map[string]storedChallenge
	inflight           map[string]struct{}
}

func (m *CaptchaManager) UseMutationRepository(repository any) {
	m.mutationRepository = repository
}

func NewCaptchaManager(private CaptchaPrivateStore, verifier CaptchaVerifier, catalog CaptchaCatalogReader, client *http.Client) *CaptchaManager {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	copy := *client
	if copy.Timeout == 0 {
		copy.Timeout = 20 * time.Second
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &CaptchaManager{
		private: private, verifier: verifier, catalog: catalog, http: &copy, now: time.Now,
		items: map[string]storedChallenge{}, inflight: map[string]struct{}{},
	}
}

func (m *CaptchaManager) Prepare(ctx context.Context, record configstore.AuthRecord, entry string, parentTaskID *string) (CaptchaChallenge, error) {
	host, entry := configstore.CanonicalHost(record.Host), strings.TrimSpace(entry)
	record.Host = host
	credential, err := m.private.VaultEntry(ctx, entry)
	if err != nil || credential == nil || credential.Username == nil || credential.Password == nil || strings.TrimSpace(*credential.Username) == "" || *credential.Password == "" {
		return CaptchaChallenge{}, errors.New("所选密码箱项没有完整用户名和密码")
	}
	return m.prepareCredential(ctx, record, *credential, false, parentTaskID)
}

func (m *CaptchaManager) PrepareCredential(ctx context.Context, record configstore.AuthRecord, credential configstore.VaultEntry, saveToVault bool, parentTaskID *string) (CaptchaChallenge, error) {
	record.Host = configstore.CanonicalHost(record.Host)
	credential.Entry = strings.TrimSpace(credential.Entry)
	if credential.Entry == "" {
		credential.Entry = record.Host
	}
	if credential.Username == nil || credential.Password == nil || strings.TrimSpace(*credential.Username) == "" || *credential.Password == "" {
		return CaptchaChallenge{}, errors.New("图片验证码登录缺少完整用户名和密码")
	}
	return m.prepareCredential(ctx, record, credential, saveToVault, parentTaskID)
}

func (m *CaptchaManager) prepareCredential(ctx context.Context, record configstore.AuthRecord, credential configstore.VaultEntry, saveToVault bool, parentTaskID *string) (CaptchaChallenge, error) {
	host, entry := configstore.CanonicalHost(record.Host), strings.TrimSpace(credential.Entry)
	record.Host = host
	if !strings.EqualFold(record.UpstreamType, "sub2api") {
		return CaptchaChallenge{}, errors.New("普通图片验证码恢复当前只支持 Sub2API 上游")
	}
	if !configstore.IsEmailUsername(credential.Username) {
		return CaptchaChallenge{}, errors.New("所选密码箱项的用户名不是有效邮箱，请选择与当前 Host 匹配的密码项")
	}
	headers := captchaBrowserIdentityHeaders(record.BaseURL, record.Headers, credential.Headers)
	cookies := cloneMap(record.Cookies)
	settings, err := m.request(ctx, record.BaseURL, http.MethodGet, "/api/v1/settings/public", headers, cookies, nil)
	if err != nil {
		return CaptchaChallenge{}, fmt.Errorf("公开设置读取失败：%w", err)
	}
	settingsData, err := captchaPayloadData(settings, "公开设置")
	if err != nil {
		return CaptchaChallenge{}, err
	}
	if enabled, valid := captchaBool(settingsData["turnstile_enabled"]); !valid && settingsData["turnstile_enabled"] != nil {
		return CaptchaChallenge{}, errors.New("上游 Turnstile 配置格式不可读")
	} else if enabled {
		return CaptchaChallenge{}, errors.New("上游启用了 Turnstile，API 无法绕过，需要浏览器验证")
	}
	keyRequestTime := m.now().UTC().Unix()
	keyPayload, err := m.request(ctx, record.BaseURL, http.MethodGet, "/api/v1/auth/credential-key", headers, cookies, nil)
	if err != nil {
		return CaptchaChallenge{}, fmt.Errorf("凭据公钥读取失败：%w", err)
	}
	keyData, err := captchaPayloadData(keyPayload, "凭据公钥")
	if err != nil {
		return CaptchaChallenge{}, err
	}
	if textValue(keyData["algorithm"]) != captchaAlgorithm {
		return CaptchaChallenge{}, errors.New("上游凭据加密算法不受支持")
	}
	keyID, publicKey := textValue(keyData["key_id"]), textValue(keyData["public_key"])
	serverTime, err := strictInt64(keyData["server_time"])
	if err != nil || keyID == "" || publicKey == "" {
		return CaptchaChallenge{}, errors.New("上游凭据公钥缺少 key_id、public_key 或 server_time")
	}
	captchaPayload, err := m.request(ctx, record.BaseURL, http.MethodGet, "/api/v1/auth/captcha", headers, cookies, nil)
	if err != nil {
		return CaptchaChallenge{}, fmt.Errorf("图片验证码读取失败：%w", err)
	}
	captchaData, err := captchaPayloadData(captchaPayload, "图片验证码")
	if err != nil {
		return CaptchaChallenge{}, err
	}
	captchaID := textValue(captchaData["captcha_id"])
	image, err := validateImageData(textValue(captchaData["image_data"]))
	if err != nil || captchaID == "" {
		if err != nil {
			return CaptchaChallenge{}, err
		}
		return CaptchaChallenge{}, errors.New("上游验证码缺少 captcha_id")
	}
	expiresSeconds := int64(300)
	if value, present := captchaData["expires_in"]; present {
		expiresSeconds, err = strictInt64(value)
		if err != nil || expiresSeconds < 1 || expiresSeconds > 3600 {
			return CaptchaChallenge{}, errors.New("上游验证码有效期无效")
		}
	}
	id, err := randomURLID(24)
	if err != nil {
		return CaptchaChallenge{}, err
	}
	expires := m.now().UTC().Add(time.Duration(expiresSeconds) * time.Second)
	public := CaptchaChallenge{
		ChallengeID: id, Host: host, ImageData: image, ExpiresAt: expires.Format(time.RFC3339Nano),
		Credential: map[string]string{"entry": entry}, InteractionKind: "image_captcha_ocr",
	}
	stored := storedChallenge{
		public: public, record: record, baseURL: strings.TrimRight(record.BaseURL, "/"), entry: entry, captchaID: captchaID,
		keyID: keyID, publicKey: publicKey, serverOffset: serverTime - keyRequestTime, cookies: cookies, headers: headers,
		expiresAt: expires, parentTaskID: parentTaskID, credential: cloneVaultEntry(credential), saveToVault: saveToVault,
	}
	m.mu.Lock()
	m.discardExpiredLocked()
	m.items[id] = stored
	m.mu.Unlock()
	return public, nil
}

func (m *CaptchaManager) Submit(ctx context.Context, challengeID, code string) (CaptchaResult, error) {
	challengeID, code = strings.TrimSpace(challengeID), strings.ToUpper(strings.TrimSpace(code))
	if challengeID == "" || code == "" || len(code) > 32 {
		return CaptchaResult{}, errors.New("验证码挑战和验证码不能为空，验证码不能超过 32 个字符")
	}
	m.mu.Lock()
	m.discardExpiredLocked()
	challenge, found := m.items[challengeID]
	if _, busy := m.inflight[challengeID]; busy {
		m.mu.Unlock()
		return CaptchaResult{}, errors.New("验证码正在提交，请等待当前复核完成")
	}
	if !found {
		m.mu.Unlock()
		return CaptchaResult{}, errors.New("验证码挑战不存在或已过期，请重新准备")
	}
	m.inflight[challengeID] = struct{}{}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.inflight, challengeID)
		m.mu.Unlock()
	}()
	credential := cloneVaultEntry(challenge.credential)
	if credential.Username == nil || credential.Password == nil {
		return CaptchaResult{}, errors.New("验证码挑战中的登录凭据已不可用")
	}
	envelope, err := credentialEnvelope(challenge, *credential.Username, *credential.Password, m.now().UTC())
	if err != nil {
		return CaptchaResult{}, err
	}
	payload, err := m.request(ctx, challenge.baseURL, http.MethodPost, "/api/v1/auth/login", challenge.headers, challenge.cookies, map[string]any{
		"captcha_id": challenge.captchaID, "captcha_code": code, "credential_envelope": envelope,
	})
	if err != nil {
		return CaptchaResult{}, fmt.Errorf("验证码登录失败：%w", err)
	}
	if !explicitBusinessSuccess(payload) {
		return CaptchaResult{}, errors.New("验证码登录缺少明确业务成功标识")
	}
	data, err := captchaPayloadData(payload, "验证码登录")
	if err != nil {
		return CaptchaResult{}, err
	}
	token := textValue(firstValue(data, "access_token", "token"))
	if token == "" {
		return CaptchaResult{}, errors.New("验证码登录缺少新的 access token")
	}
	refresh := optionalString(firstValue(data, "refresh_token"))
	if refresh == nil {
		refresh = refreshTokenFromCookies(challenge.cookies)
	}
	verified := challenge.record
	verified.AuthMode, verified.AccessToken, verified.RefreshToken = "sub2api_user_token", &token, refresh
	verified.AdminKey, verified.UserID = nil, nil
	verified.Headers, verified.Cookies = cloneMap(challenge.headers), cloneMap(challenge.cookies)
	if err := m.verifier.Verify(ctx, verified); err != nil {
		return CaptchaResult{}, fmt.Errorf("验证码登录成功但鉴权复核失败：%w", err)
	}
	catalog, err := m.catalog.ReadCatalog(ctx, verified)
	if err != nil {
		return CaptchaResult{}, fmt.Errorf("验证码登录成功但分组目录复核失败：%w", err)
	}
	if err := m.commitVerified(ctx, challenge, verified, credential); err != nil {
		return CaptchaResult{}, err
	}
	m.mu.Lock()
	delete(m.items, challengeID)
	m.mu.Unlock()
	return CaptchaResult{
		Success: true, Host: challenge.public.Host, ProfileStatus: "verified", Balance: nil,
		Concurrency: nil, Keys: len(catalog.Keys), Groups: len(catalog.Groups), Stored: true,
		InteractionKind: "image_captcha_ocr", ParentTaskID: challenge.parentTaskID,
	}, nil
}

func (m *CaptchaManager) commitVerified(
	ctx context.Context,
	challenge storedChallenge,
	verified configstore.AuthRecord,
	credential configstore.VaultEntry,
) error {
	host := configstore.CanonicalHost(verified.Host)
	resources := []string{mutationguard.Upstream(host)}
	if challenge.saveToVault {
		resources = append(resources, mutationguard.Vault(credential.Entry))
	}
	guarded, release, err := mutationguard.Acquire(ctx, m.mutationRepository, resources...)
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("验证码鉴权租约释放失败", "host", host, "error", err)
		}
	}()
	if store, ok := m.mutationRepository.(captchaUpstreamStore); ok {
		exists, err := store.UpstreamExists(guarded, host)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("验证码登录已复核，但上游已被删除")
		}
	}
	if _, err := m.private.AuthRecord(guarded, host); err != nil {
		return err
	}
	verified.Host = host
	if err := m.private.SaveAuthRecord(guarded, verified, allAuthFields()); err != nil {
		return errors.New("验证码登录已复核但鉴权信息保存失败")
	}
	if challenge.saveToVault {
		if err := m.private.SaveVaultEntry(guarded, credential, allVaultFields()); err != nil {
			return errors.New("验证码登录已复核且鉴权已保存，但密码箱保存失败")
		}
	}
	return nil
}

func cloneVaultEntry(value configstore.VaultEntry) configstore.VaultEntry {
	result := value
	result.Hosts = append([]string{}, value.Hosts...)
	result.Headers = cloneMap(value.Headers)
	return result
}

func allVaultFields() map[string]bool {
	return map[string]bool{"username": true, "password": true, "hosts": true, "headers": true}
}

func (m *CaptchaManager) Cancel(challengeID string) (*CaptchaChallenge, *string) {
	challengeID = strings.TrimSpace(challengeID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, busy := m.inflight[challengeID]; busy {
		return nil, nil
	}
	challenge, found := m.items[challengeID]
	if !found {
		return nil, nil
	}
	delete(m.items, challengeID)
	public := challenge.public
	return &public, challenge.parentTaskID
}

func (m *CaptchaManager) request(ctx context.Context, baseURL, method, path string, headers, cookies map[string]string, body any) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, errors.New("验证码请求编码失败")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, reader)
	if err != nil {
		return nil, errors.New("验证码请求创建失败")
	}
	for key, value := range captchaBrowserIdentityHeaders(baseURL) {
		request.Header.Set(key, value)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if body != nil && request.Header.Get("Origin") == "" {
		request.Header.Set("Origin", request.URL.Scheme+"://"+request.URL.Host)
	}
	for key, value := range cookies {
		request.AddCookie(&http.Cookie{Name: key, Value: value})
	}
	response, err := m.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("验证码网络请求失败：%T", err)
	}
	defer response.Body.Close()
	for _, cookie := range response.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumCaptchaResponse+1))
	if err != nil || len(raw) > maximumCaptchaResponse {
		return nil, errors.New("验证码响应过大或读取失败")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := captchaFailureDetail(raw)
		if detail == "" {
			return nil, fmt.Errorf("HTTP %d", response.StatusCode)
		}
		return nil, fmt.Errorf("HTTP %d：%s", response.StatusCode, detail)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, errors.New("验证码响应不是有效 JSON 对象")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("验证码响应包含尾随数据")
	}
	if !captchaBusinessSuccess(payload) {
		return nil, errors.New("上游返回业务失败")
	}
	return payload, nil
}

func (m *CaptchaManager) discardExpiredLocked() {
	now := m.now().UTC()
	for id, item := range m.items {
		if !item.expiresAt.After(now) {
			delete(m.items, id)
		}
	}
}

func credentialEnvelope(challenge storedChallenge, username, password string, now time.Time) (map[string]string, error) {
	der, err := decodePublicKey(challenge.publicKey)
	if err != nil {
		return nil, errors.New("验证码挑战中的凭据公钥无效")
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, errors.New("验证码挑战中的凭据公钥无效")
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("验证码挑战中的凭据公钥不是 RSA 公钥")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	plaintext, err := json.Marshal(map[string]any{
		"email": username, "password": password, "issued_at": now.UTC().Unix() + challenge.serverOffset,
	})
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, iv, plaintext, []byte(challenge.keyID))
	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, key, nil)
	if err != nil {
		return nil, errors.New("验证码凭据加密失败")
	}
	encode := base64.RawURLEncoding.EncodeToString
	return map[string]string{
		"algorithm": captchaAlgorithm, "key_id": challenge.keyID, "encrypted_key": encode(encryptedKey),
		"iv": encode(iv), "ciphertext": encode(ciphertext),
	}, nil
}

func captchaFailureDetail(raw []byte) string {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, key := range []string{"message", "reason", "code"} {
		value := safeReason(textValue(payload[key]))
		if value == "" || value == "鉴权恢复失败" || seen[value] {
			continue
		}
		seen[value] = true
		parts = append(parts, value)
	}
	return strings.Join(parts, "；")
}

func decodePublicKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, encoding := range encodings {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("凭据公钥不是有效 Base64")
}

func validateImageData(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("上游验证码图片为空")
	}
	mime, encoded := "image/png", value
	if strings.HasPrefix(value, "data:") {
		parts := strings.SplitN(value, ",", 2)
		if len(parts) != 2 || !strings.Contains(strings.ToLower(parts[0]), ";base64") {
			return "", errors.New("上游验证码图片格式不受支持")
		}
		mime = strings.ToLower(strings.SplitN(strings.TrimPrefix(parts[0], "data:"), ";", 2)[0])
		encoded = parts[1]
	}
	allowed := map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}
	if !allowed[mime] {
		return "", errors.New("上游验证码图片格式不受支持")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) == 0 || len(raw) > maximumImageBytes {
		return "", errors.New("上游验证码图片不是有效 Base64 或大小无效")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

func captchaBrowserIdentityHeaders(baseURL string, sources ...map[string]string) map[string]string {
	result := http.Header{}
	for _, source := range sources {
		for key, value := range source {
			if strings.EqualFold(key, "cookie") || strings.ContainsAny(key+value, "\r\n") {
				continue
			}
			if strings.EqualFold(key, "authorization") && strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "bearer ") {
				continue
			}
			result.Set(key, value)
		}
	}
	defaults := map[string]string{
		"Accept":             "application/json, text/plain, */*",
		"Accept-Language":    "zh-CN,zh;q=0.9",
		"Cache-Control":      "no-cache",
		"Pragma":             "no-cache",
		"User-Agent":         captchaBrowserUserAgent,
		"X-User-UI-Request":  "1",
		"Referer":            strings.TrimRight(baseURL, "/") + "/login",
		"Sec-Fetch-Site":     "same-origin",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Dest":     "empty",
		"sec-ch-ua":          `"Not;A=Brand";v="8", "Chromium";v="150", "Google Chrome";v="150"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
	}
	for key, value := range defaults {
		if result.Get(key) == "" {
			result.Set(key, value)
		}
	}
	flattened := make(map[string]string, len(result))
	for key, values := range result {
		if len(values) > 0 {
			flattened[key] = values[0]
		}
	}
	return flattened
}

func captchaPayloadData(payload map[string]any, label string) (map[string]any, error) {
	if value, present := payload["data"]; present {
		data, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s data 不可读", label)
		}
		return data, nil
	}
	return payload, nil
}

func captchaBusinessSuccess(payload map[string]any) bool {
	if value, present := payload["code"]; present {
		text := strings.ToLower(textValue(value))
		if text != "0" && text != "200" && text != "true" && text != "success" && text != "ok" {
			return false
		}
	}
	for _, key := range []string{"success", "ok"} {
		if value, present := payload[key]; present {
			enabled, valid := captchaBool(value)
			if !valid || !enabled {
				return false
			}
		}
	}
	return true
}

func explicitBusinessSuccess(payload map[string]any) bool {
	for _, key := range []string{"code", "success", "ok"} {
		if _, present := payload[key]; present {
			return captchaBusinessSuccess(payload)
		}
	}
	return false
}

func captchaBool(value any) (bool, bool) {
	switch item := value.(type) {
	case bool:
		return item, true
	case json.Number:
		if item.String() == "0" || item.String() == "1" {
			return item.String() == "1", true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "1", "true", "yes", "on", "enabled", "success", "ok":
			return true, true
		case "0", "false", "no", "off", "disabled", "":
			return false, true
		}
	}
	return false, false
}

func strictInt64(value any) (int64, error) {
	if value == nil {
		return 0, errors.New("整数缺失")
	}
	parsed, err := strconv.ParseInt(textValue(value), 10, 64)
	return parsed, err
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstValue(value map[string]any, keys ...string) any {
	for _, key := range keys {
		if item, present := value[key]; present {
			return item
		}
	}
	return nil
}

func optionalString(value any) *string {
	text := textValue(value)
	if text == "" {
		return nil
	}
	return &text
}

func refreshTokenFromCookies(cookies map[string]string) *string {
	for _, name := range []string{"sub2api_refresh_token", "refresh_token", "new_api_refresh"} {
		if value := strings.TrimSpace(cookies[name]); value != "" {
			return &value
		}
	}
	return nil
}

func randomURLID(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
