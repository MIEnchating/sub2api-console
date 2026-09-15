package browserlogin

import (
	"context"
	"errors"
	"net/mail"
	"strings"
)

var ErrSecurityPending = errors.New("账号安全操作尚未完成，请继续官方页面验证")
var ErrSecurityIdentity = errors.New("官方登录身份与本次安全任务不一致，请重新登录指定账号")
var ErrSecurityUncertain = errors.New("安全操作结果尚未确认，请先核对官方账号状态，不能自动重试")

type SecurityOptions struct {
	Email    string `json:"email"`
	ProxyURL string `json:"proxy_url,omitempty"`
}

func (o SecurityOptions) Validate() error {
	if err := ValidateProxyURL(o.ProxyURL); err != nil {
		return err
	}
	address, err := mail.ParseAddress(o.Email)
	if err != nil || address.Address != o.Email || len(o.Email) > 320 || strings.TrimSpace(o.Email) != o.Email {
		return errors.New("账号安全任务需要有效邮箱")
	}
	return nil
}

// These contracts cross only the private browser socket. Tokens and cookies
// remain in the browser; enrollment secrets go directly to the Go service.
type SecurityIdentity struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

func (v SecurityIdentity) Validate() error {
	if err := (SecurityOptions{Email: v.Email}).Validate(); err != nil || v.UserID == "" || len(v.UserID) > 256 || strings.ContainsAny(v.UserID, "\r\n\x00 ") {
		return ErrSecurityIdentity
	}
	return nil
}

type SecurityEnrollment struct {
	Secret    string `json:"secret"`
	SessionID string `json:"session_id"`
}

type SecurityBrowser interface {
	OAuthAutomation
	Screenshot(context.Context) ([]byte, error)
	Input(context.Context, Input) error
	Identity(context.Context) (SecurityIdentity, error)
	BeginPassword(context.Context, SecurityIdentity) error
	SubmitPassword(context.Context, AuthAction) error
	PasswordResult(context.Context) (bool, error)
	TOTPEnabled(context.Context, SecurityIdentity) (bool, error)
	EnrollTOTP(context.Context, SecurityIdentity) (SecurityEnrollment, error)
	ActivateTOTP(context.Context, SecurityIdentity, SecurityEnrollment, string) error
	Close()
}

type SecurityFactory interface {
	OpenSecurity(context.Context, SecurityOptions) (SecurityBrowser, error)
}
