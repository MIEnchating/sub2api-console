package accountworkbench

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
	"github.com/pquerna/otp/totp"
)

type OAuthStartInput struct {
	callbacks       oauthCallbacks
	RecoveryEnabled bool             `json:"recovery_enabled,omitempty"`
	Scope           ExportScope      `json:"scope,omitempty"`
	Login           *OAuthLoginInput `json:"login,omitempty"`
	ProxyURL        string           `json:"proxy_url,omitempty"`
}

type oauthCallbacks struct {
	beforeLaunch func(context.Context, OAuthView) error
	onAuthorized func(context.Context, map[string]any) error
}

type OAuthLoginInput struct {
	ProxyURL    string             `json:"proxy_url,omitempty"`
	Email       string             `json:"email"`
	Password    string             `json:"password,omitempty"`
	TOTPSecret  string             `json:"totp_secret,omitempty"`
	WorkspaceID string             `json:"workspace_id,omitempty"`
	Mailbox     *OAuthMailboxInput `json:"mailbox,omitempty"`
	SMS         *OAuthSMSInput     `json:"sms,omitempty"`
}

type OAuthMailboxInput struct {
	Kind         string            `json:"kind"`
	URL          string            `json:"url,omitempty"`
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body,omitempty"`
	Email        string            `json:"email,omitempty"`
	ClientID     string            `json:"client_id,omitempty"`
	RefreshToken string            `json:"refresh_token,omitempty"`
}

type oauthAssist struct {
	smsOnly           bool
	smsAttachRevision string
	login             OAuthLoginInput
	http              *workbenchprovider.HTTP
	mailbox           *workbenchprovider.Mailbox
	baseline          workbenchprovider.MailBaseline
	requestedAt       time.Time
	attempted         map[string]bool
	lastPoll          time.Time
	sms               *workbenchprovider.SMS
	smsNumber         workbenchprovider.SMSNumber
	smsOperation      string
	smsProvider       string
	smsConfigHash     string
	lastSMSPoll       time.Time
	smsCodeSubmitted  bool
	smsVerified       bool
}

func (s *Service) prepareOAuthAssist(input *OAuthLoginInput) (*oauthAssist, error) {
	if input == nil {
		return nil, nil
	}
	login := *input
	login.Email = strings.TrimSpace(login.Email)
	validate := func(stage, value string) error {
		return (browserlogin.AuthAction{Stage: stage, Revision: strings.Repeat("0", 64), Value: value}).Validate()
	}
	if err := validate("email", login.Email); err != nil {
		return nil, errors.New("请填写有效的登录邮箱")
	}
	if login.Password != "" {
		if err := validate("password", login.Password); err != nil {
			return nil, err
		}
	}
	if login.WorkspaceID != "" {
		if err := validate("workspace", login.WorkspaceID); err != nil {
			return nil, err
		}
	}
	login.TOTPSecret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(login.TOTPSecret), " ", ""))
	if login.TOTPSecret != "" {
		if len(login.TOTPSecret) < 16 || len(login.TOTPSecret) > 128 {
			return nil, errors.New("2FA 密钥格式无效，请检查 Base32 密钥")
		}
		if _, err := totp.GenerateCode(login.TOTPSecret, time.Now()); err != nil {
			return nil, errors.New("2FA 密钥格式无效，请检查 Base32 密钥")
		}
	}
	assist := &oauthAssist{login: login, http: workbenchprovider.NewHTTP(s.providerTransport), attempted: make(map[string]bool)}
	if input.Mailbox != nil {
		mail := input.Mailbox
		if mail.Kind == "microsoft" && !strings.EqualFold(mail.Email, login.Email) {
			assist.close()
			return nil, errors.New("Microsoft 收码邮箱必须与授权登录邮箱一致")
		}
		box, err := workbenchprovider.NewMailbox(workbenchprovider.MailConfig{Kind: mail.Kind, URL: mail.URL, Method: mail.Method, Headers: mail.Headers, Body: mail.Body, Email: mail.Email, ClientID: mail.ClientID, RefreshToken: mail.RefreshToken}, assist.http, nil)
		if err != nil {
			assist.close()
			return nil, err
		}
		assist.mailbox = box
	}
	assist.login.Mailbox = nil
	assist.login.SMS = nil
	if err := assist.prepareSMS(input.SMS, s.smsPool); err != nil {
		assist.close()
		return nil, err
	}
	return assist, nil
}

func (a *oauthAssist) close() {
	if a == nil {
		return
	}
	if a.mailbox != nil {
		a.mailbox.Close()
		a.mailbox = nil
	}
	if a.http != nil {
		a.http.Close()
	}
	if a.sms != nil {
		a.sms.Close()
		a.sms = nil
	}
	a.smsNumber = workbenchprovider.SMSNumber{}
	a.login = OAuthLoginInput{}
	a.baseline = workbenchprovider.MailBaseline{}
}

func (a *oauthAssist) initialize(ctx context.Context) error {
	if a.mailbox == nil {
		return nil
	}
	baseline, err := a.mailbox.Baseline(ctx)
	if err != nil {
		return err
	}
	a.baseline = baseline
	return nil
}

func (a *oauthAssist) step(ctx context.Context, browser browserlogin.OAuthBrowser) (string, error) {
	automation, ok := browser.(browserlogin.OAuthAutomation)
	if !ok {
		return "", errors.New("授权浏览器尚不支持自动辅助，请更新 browser 服务或手动登录")
	}
	page, err := automation.InspectAuth(ctx)
	if err != nil {
		return "", err
	}
	if page.Stage == "manual" {
		return "当前页面需要人工操作，请完成后验证授权", nil
	}
	if a.smsCodeSubmitted && page.Stage != "sms_code" && page.Stage != "phone" {
		a.smsVerified = true
	}
	if a.smsOnly && page.Stage != "phone" && page.Stage != "sms_code" {
		return "短信步骤以外的页面需要人工操作，请完成后验证授权", nil
	}
	if page.Stage == "phone" && a.smsAttachRevision != "" && page.Revision != a.smsAttachRevision {
		return "", browserlogin.ErrAuthPageChanged
	}
	if a.attempted[page.Revision] || a.attempted["stage:"+page.Stage] {
		return "已提交当前步骤，等待页面变化；如提示失败请人工处理", nil
	}
	value := ""
	switch page.Stage {
	case "email":
		value = a.login.Email
	case "phone", "sms_code":
		value, err = a.smsValue(ctx, page.Stage)
		if err != nil {
			return "", err
		}
		if page.Stage == "sms_code" && a.sms != nil && a.smsNumber.RequestID != "" && value == "" {
			return "正在等待新的短信验证码", nil
		}
	case "password":
		value = a.login.Password
	case "workspace":
		value = a.login.WorkspaceID
		if value == "" && len(page.WorkspaceIDs) == 1 {
			value = page.WorkspaceIDs[0]
		}
	case "totp_code":
		if a.login.TOTPSecret != "" {
			value, err = totp.GenerateCode(a.login.TOTPSecret, time.Now())
			if err != nil {
				return "", errors.New("2FA 验证码生成失败，请人工输入")
			}
		}
	case "email_code":
		if a.mailbox != nil && !a.requestedAt.IsZero() {
			if time.Since(a.lastPoll) < 5*time.Second {
				return "正在等待新的邮箱验证码", nil
			}
			a.lastPoll = time.Now()
			candidates, err := a.mailbox.Fetch(ctx, a.requestedAt, a.baseline)
			if err != nil {
				return "", err
			}
			if len(candidates) == 0 {
				return "正在等待新的邮箱验证码", nil
			}
			value = candidates[0].Code
		}
	}
	if value == "" {
		return "当前步骤缺少自动填写信息，请在授权页面完成", nil
	}
	if page.Stage == "email" || page.Stage == "password" {
		a.requestedAt = time.Now()
	}
	// An uncertain submit must never resend credentials or a one-time code.
	a.attempted[page.Revision] = true
	a.attempted["stage:"+page.Stage] = true
	if err := automation.ApplyAuth(ctx, browserlogin.AuthAction{Stage: page.Stage, Revision: page.Revision, Value: value}); err != nil {
		return "", errors.New("自动填写未确认成功，请检查授权页面后人工继续")
	}
	if page.Stage == "phone" && a.sms != nil {
		if err := a.sms.MarkReady(ctx, a.smsNumber.RequestID); err != nil {
			return "", err
		}
	}
	if page.Stage == "sms_code" {
		a.smsCodeSubmitted = true
	}
	return "已提交当前登录步骤，正在等待官方页面", nil
}

func (s *Service) tickOAuthAssist(ctx context.Context, value *oauthSession, task *taskstore.Task, assist *oauthAssist) error {
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	paused, browser, status := value.assistPaused, value.browser, value.view.Status
	value.mu.Unlock()
	if paused || browser == nil || status != "waiting" || assist == nil || assist.smsOnly && assist.sms == nil {
		return nil
	}
	if err := s.validateOAuthScope(ctx, value); err != nil {
		return err
	}
	if _, err := browser.AuthorizationCode(ctx); !errors.Is(err, browserlogin.ErrOAuthPending) {
		_ = s.FinishOAuth(value.owner, task.ID)
		return nil
	}
	message, assistErr := assist.step(ctx, browser)
	value.mu.Lock()
	defer value.mu.Unlock()
	if assistErr != nil {
		value.assistPaused = true
		message = assistErr.Error()
	}
	if value.view.Status != "waiting" || message == task.Message {
		return nil
	}
	value.view.Message = message
	task.Message, task.UpdatedAt = message, time.Now().UTC().Format(time.RFC3339Nano)
	return s.tasks.Save(ctx, *task)
}
