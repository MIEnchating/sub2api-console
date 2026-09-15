package workbenchprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const microsoftTokenURL = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
const microsoftMessagesURL = "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages"

type MicrosoftCredential struct {
	Email        string `json:"-"`
	Password     string `json:"-"`
	ClientID     string `json:"-"`
	RefreshToken string `json:"-"`
}

var microsoftLineSeparator = regexp.MustCompile(`-{2,}`)

func ParseMicrosoftCredentialLine(source string) (MicrosoftCredential, error) {
	if len(source) > 40<<10 {
		return MicrosoftCredential{}, mailError("mail_microsoft_line_invalid", "Microsoft 邮箱凭据行过长，请检查输入")
	}
	fields := microsoftLineSeparator.Split(strings.TrimSpace(source), -1)
	if len(fields) != 4 {
		return MicrosoftCredential{}, mailError("mail_microsoft_line_invalid", "Microsoft 邮箱凭据需要邮箱、密码、客户端 ID 和刷新令牌四个字段")
	}
	item := MicrosoftCredential{Email: strings.TrimSpace(fields[0]), Password: fields[1], ClientID: strings.TrimSpace(fields[2]), RefreshToken: strings.TrimSpace(fields[3])}
	address, err := mail.ParseAddress(item.Email)
	if err != nil || address.Address != item.Email || !microsoftClientID.MatchString(item.ClientID) || !validMailToken(item.RefreshToken) {
		return MicrosoftCredential{}, mailError("mail_microsoft_line_invalid", "Microsoft 邮箱凭据格式无效，请检查邮箱、客户端 ID 和刷新令牌")
	}
	return item, nil
}

type microsoftToken struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	TokenType    string          `json:"token_type"`
	Scope        string          `json:"scope"`
	ExpiresIn    json.Number     `json:"expires_in"`
	Error        json.RawMessage `json:"error"`
}

type microsoftMessages struct {
	Value []struct {
		ID               string `json:"id"`
		Subject          string `json:"subject"`
		BodyPreview      string `json:"bodyPreview"`
		ReceivedDateTime string `json:"receivedDateTime"`
		Body             struct {
			Content string `json:"content"`
		} `json:"body"`
	} `json:"value"`
	Error json.RawMessage `json:"error"`
}

func (m *Mailbox) fetchMicrosoft(ctx context.Context) ([]MailCandidate, error) {
	token, err := m.microsoftAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	query := url.Values{"$top": {"10"}, "$orderby": {"receivedDateTime desc"}, "$select": {"id,subject,bodyPreview,body,receivedDateTime,from"}}
	body, err := m.http.Do(ctx, Request{Method: http.MethodGet, URL: microsoftMessagesURL + "?" + query.Encode(), Header: http.Header{"Authorization": {"Bearer " + token}}})
	if err != nil {
		if isMailPermissionError(err) {
			return nil, mailReadPermissionError()
		}
		var remote *Error
		if errors.As(err, &remote) && remote.HTTPStatus == http.StatusUnauthorized {
			m.accessToken = ""
			return nil, mailError("mail_microsoft_access_expired", "Microsoft 邮箱访问令牌失效，下次读取将重新验证授权")
		}
		return nil, err
	}
	var response microsoftMessages
	if err := decodeMailJSON(body, &response); err != nil {
		return nil, err
	}
	if microsoftHasError(response.Error) {
		var graphError struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(response.Error, &graphError)
		if graphError.Code == "ErrorAccessDenied" || graphError.Code == "Authorization_RequestDenied" {
			return nil, mailReadPermissionError()
		}
		return nil, mailError("mail_microsoft_service_failed", "Microsoft 邮箱接口未成功处理请求，请核对授权状态")
	}
	if response.Value == nil {
		return nil, mailError("mail_microsoft_response_invalid", "Microsoft 邮箱没有返回有效邮件列表，请检查读取权限")
	}
	messages := make([]map[string]any, 0, len(response.Value))
	for _, message := range response.Value {
		if strings.TrimSpace(message.ID) == "" || parseMailTime(message.ReceivedDateTime).IsZero() {
			return nil, mailError("mail_microsoft_response_invalid", "Microsoft 邮件缺少有效 ID 或接收时间，请稍后重试")
		}
		messages = append(messages, map[string]any{"id": message.ID, "subject": message.Subject, "text": message.BodyPreview, "html": message.Body.Content, "received_at": message.ReceivedDateTime})
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		return nil, mailError("mail_microsoft_response_invalid", "Microsoft 邮件内容无效，请稍后重试")
	}
	return ExtractMailCandidates(encoded)
}

func (m *Mailbox) microsoftAccessToken(ctx context.Context) (string, error) {
	if m.rotateDue {
		if err := m.persistMicrosoftRotation(ctx); err != nil {
			return "", err
		}
	}
	if m.accessToken != "" && time.Now().Before(m.expiresAt) {
		return m.accessToken, nil
	}
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {m.config.ClientID}, "refresh_token": {m.config.RefreshToken}, "scope": {"https://graph.microsoft.com/Mail.Read offline_access"}}
	body, err := m.http.Do(ctx, Request{Method: http.MethodPost, URL: microsoftTokenURL, Header: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, Body: []byte(form.Encode())})
	if err != nil {
		if isMailPermissionError(err) {
			return "", mailReadPermissionError()
		}
		return "", err
	}
	var response microsoftToken
	if err := decodeMailJSON(body, &response); err != nil {
		return "", err
	}
	if microsoftHasError(response.Error) {
		return "", mailError("mail_microsoft_refresh_failed", "Microsoft 邮箱令牌刷新失败，请重新授权邮箱")
	}
	if response.RefreshToken != "" && !validMailToken(response.RefreshToken) {
		return "", mailError("mail_microsoft_response_invalid", "Microsoft 返回的刷新凭据无效，请重新授权邮箱")
	}
	lifetime, tokenErr := validateMicrosoftAccess(response)
	if tokenErr == nil {
		m.accessToken = response.AccessToken
		m.expiresAt = time.Now().Add(lifetime - min(time.Minute, lifetime/10))
	}
	// A successful exchange can rotate RT even when its access scope or
	// lifetime is unusable. Preserve the replacement before reporting that.
	if response.RefreshToken != "" && response.RefreshToken != m.config.RefreshToken {
		m.config.RefreshToken = response.RefreshToken
		m.rotateDue = true
		if err := m.persistMicrosoftRotation(ctx); err != nil {
			return "", err
		}
	}
	if tokenErr != nil {
		return "", tokenErr
	}
	return m.accessToken, nil
}

func validateMicrosoftAccess(response microsoftToken) (time.Duration, error) {
	if !validMailToken(response.AccessToken) || (response.TokenType != "" && !strings.EqualFold(response.TokenType, "Bearer")) {
		return 0, mailError("mail_microsoft_refresh_failed", "Microsoft 邮箱令牌刷新失败，请重新授权邮箱")
	}
	if response.Scope != "" && !hasMicrosoftMailRead(response.Scope) {
		return 0, mailReadPermissionError()
	}
	expires := int64(3600)
	if response.ExpiresIn != "" {
		var err error
		expires, err = response.ExpiresIn.Int64()
		if err != nil || expires < 1 || expires > 86400 {
			return 0, mailError("mail_microsoft_response_invalid", "Microsoft 令牌有效期无效，请重新授权邮箱")
		}
	}
	return time.Duration(expires) * time.Second, nil
}

func (m *Mailbox) persistMicrosoftRotation(ctx context.Context) error {
	if m.onRotate != nil {
		if err := m.onRotate(ctx, m.config.RefreshToken); err != nil {
			return mailError("mail_microsoft_rotation_save_failed", "邮箱刷新令牌已轮换，但私有保存失败，请重试保存后继续收码")
		}
	}
	m.rotateDue = false
	return nil
}

func hasMicrosoftMailRead(scope string) bool {
	for _, permission := range strings.Fields(scope) {
		permission = strings.TrimPrefix(strings.ToLower(permission), "https://graph.microsoft.com/")
		if permission == "mail.read" || permission == "mail.readwrite" {
			return true
		}
	}
	return false
}

func mailReadPermissionError() error {
	return mailError("mail_microsoft_consent_required", "Microsoft 邮箱缺少委托 Mail.Read 权限，请重新授权并同意读取邮件")
}

func microsoftHasError(value json.RawMessage) bool {
	return len(value) > 0 && string(value) != "null" && string(value) != `""`
}

func decodeMailJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return mailError("mail_microsoft_response_invalid", "Microsoft 邮箱响应不是有效 JSON，请稍后重试")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return mailError("mail_microsoft_response_invalid", "Microsoft 邮箱响应包含多余内容，请稍后重试")
	}
	return nil
}
