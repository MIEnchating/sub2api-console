package workbenchprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type MailConfig struct {
	Kind         string            `json:"-"`
	URL          string            `json:"-"`
	Method       string            `json:"-"`
	Headers      map[string]string `json:"-"`
	Body         string            `json:"-"`
	Email        string            `json:"-"`
	ClientID     string            `json:"-"`
	RefreshToken string            `json:"-"`
}

type MailCandidate struct {
	Code       string    `json:"-"`
	Key        string    `json:"-"`
	ReceivedAt time.Time `json:"-"`
	Score      int       `json:"-"`
}

type MailBaseline struct {
	mailboxID uint64
	takenAt   time.Time
	keys      map[string]bool
	codes     map[string]bool
}

type Mailbox struct {
	mu          sync.Mutex
	id          uint64
	config      MailConfig
	http        *HTTP
	onRotate    func(context.Context, string) error
	accessToken string
	expiresAt   time.Time
	rotateDue   bool
	closed      bool
}

var mailboxID atomic.Uint64
var microsoftClientID = regexp.MustCompile(`(?i)^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

func NewMailbox(config MailConfig, client *HTTP, onRotate func(context.Context, string) error) (*Mailbox, error) {
	if client == nil {
		client = NewHTTP(nil)
	}
	config.Kind = strings.TrimSpace(config.Kind)
	switch config.Kind {
	case "http":
		if err := ValidateURL(config.URL); err != nil {
			return nil, err
		}
		config.Method = strings.ToUpper(strings.TrimSpace(config.Method))
		if config.Method == "" {
			config.Method = http.MethodGet
		}
		if config.Method != http.MethodGet && config.Method != http.MethodPost {
			return nil, mailError("mail_method_invalid", "邮箱收码接口仅支持 GET 或 POST")
		}
		if len(config.Body) > MaxResponseBytes || len(config.Headers) > 50 || (config.Method == http.MethodGet && config.Body != "") {
			return nil, mailError("mail_config_invalid", "邮箱请求配置无效，请检查请求方法、请求头和请求体")
		}
		copiedHeaders := make(map[string]string, len(config.Headers))
		for key, value := range config.Headers {
			if strings.TrimSpace(key) == "" || strings.ContainsAny(key+value, "\r\n") || len(key)+len(value) > 16<<10 {
				return nil, mailError("mail_config_invalid", "邮箱请求头无效，请检查配置")
			}
			copiedHeaders[key] = value
		}
		config.Headers = copiedHeaders
	case "microsoft":
		address, err := mail.ParseAddress(config.Email)
		if err != nil || address.Address != config.Email || !microsoftClientID.MatchString(config.ClientID) || !validMailToken(config.RefreshToken) {
			return nil, mailError("mail_microsoft_config_invalid", "Microsoft 邮箱需要完整邮箱地址、客户端 ID 和刷新令牌")
		}
	default:
		return nil, mailError("mail_kind_invalid", "请选择 HTTP 或 Microsoft 邮箱收码服务")
	}
	return &Mailbox{id: mailboxID.Add(1), config: config, http: client, onRotate: onRotate}, nil
}

// Baseline must finish before triggering the upstream verification email.
// Candidates without trustworthy timestamps are never used without it.
func (m *Mailbox) Baseline(ctx context.Context) (MailBaseline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	candidates, err := m.fetch(ctx)
	if err != nil {
		return MailBaseline{}, err
	}
	baseline := MailBaseline{mailboxID: m.id, takenAt: time.Now(), keys: make(map[string]bool), codes: make(map[string]bool)}
	for _, candidate := range candidates {
		baseline.keys[candidate.Key] = true
		baseline.codes[candidate.Code] = true
	}
	return baseline, nil
}

func (m *Mailbox) Fetch(ctx context.Context, requestedAt time.Time, baseline MailBaseline) ([]MailCandidate, error) {
	if requestedAt.IsZero() || requestedAt.After(time.Now().Add(time.Minute)) {
		return nil, mailError("mail_request_time_invalid", "验证码请求时间无效，请重新发送验证码")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	candidates, err := m.fetch(ctx)
	if err != nil {
		return nil, err
	}
	baselineReady := baseline.mailboxID == m.id && !baseline.takenAt.IsZero() && !baseline.takenAt.After(requestedAt)
	cutoff := requestedAt
	if baselineReady {
		// Graph timestamps commonly have second precision. The pre-request
		// baseline excludes old codes while allowing a new same-second email.
		cutoff = requestedAt.Truncate(time.Second)
	}
	result := make([]MailCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if baselineReady && (baseline.keys[candidate.Key] || baseline.codes[candidate.Code]) {
			continue
		}
		if candidate.ReceivedAt.IsZero() {
			if !baselineReady {
				continue
			}
		} else if candidate.ReceivedAt.Before(cutoff) || candidate.ReceivedAt.After(time.Now().Add(time.Minute)) {
			continue
		}
		result = append(result, candidate)
	}
	return result, nil
}

func (m *Mailbox) fetch(ctx context.Context) ([]MailCandidate, error) {
	if m.closed {
		return nil, mailError("mail_closed", "邮箱收码会话已结束，请重新授权")
	}
	if m.config.Kind == "microsoft" {
		return m.fetchMicrosoft(ctx)
	}
	headers := make(http.Header, len(m.config.Headers))
	for key, value := range m.config.Headers {
		headers.Set(key, value)
	}
	body, err := m.http.Do(ctx, Request{Method: m.config.Method, URL: m.config.URL, Header: headers, Body: []byte(m.config.Body)})
	if err != nil {
		return nil, err
	}
	return ExtractMailCandidates(body)
}

func (m *Mailbox) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.config = MailConfig{}
	m.accessToken = ""
	m.onRotate = nil
}

func mailError(code, message string) error { return &Error{Code: code, Message: message} }

func mailFingerprint(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validMailToken(token string) bool {
	return strings.TrimSpace(token) != "" && len(token) <= 32<<10 && !strings.ContainsAny(token, "\r\n\x00")
}

func isMailPermissionError(err error) bool {
	var providerError *Error
	return errors.As(err, &providerError) && providerError.HTTPStatus == http.StatusForbidden
}
