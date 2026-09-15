package accountworkbench

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/pquerna/otp/totp"
)

func (s *Service) securityStep(ctx context.Context, value *securitySession, browser browserlogin.SecurityBrowser) (bool, string, error) {
	if value.view.Operation == "password" {
		return s.passwordStep(ctx, value, browser)
	}
	identity, err := browser.Identity(ctx)
	if err != nil {
		return false, "请先在官方页面完成所选账号登录", err
	}
	if !sameSecurityIdentity(identity, value.expected) {
		return false, "", browserlogin.ErrSecurityIdentity
	}
	enabled, err := browser.TOTPEnabled(ctx, value.expected)
	if err != nil {
		return false, "", err
	}
	if enabled {
		return true, "该账号已启用 TOTP 双重验证，无需重复设置", nil
	}
	if err := s.validateSecurityOrigin(ctx, value); err != nil {
		return false, "", err
	}
	enrollment, err := browser.EnrollTOTP(ctx, value.expected)
	if err != nil {
		return false, "", browserlogin.ErrSecurityUncertain
	}
	secret := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(enrollment.Secret), " ", ""))
	if len(secret) < 16 || len(secret) > 128 || strings.Trim(secret, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567=") != "" || enrollment.SessionID == "" || len(enrollment.SessionID) > 1024 || strings.ContainsAny(enrollment.SessionID, "\r\n\x00") {
		return false, "", browserlogin.ErrSecurityUncertain
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		return false, "", browserlogin.ErrSecurityUncertain
	}
	enrollment.Secret = secret
	if err := value.saveSecret(secret, enrollment.SessionID); err != nil {
		return false, "", err
	}
	if err := s.validateSecurityOrigin(ctx, value); err != nil {
		return false, "", err
	}
	if err := browser.ActivateTOTP(ctx, value.expected, enrollment, code); err != nil {
		return false, "", browserlogin.ErrSecurityUncertain
	}
	enabled, err = browser.TOTPEnabled(ctx, value.expected)
	if err != nil || !enabled {
		return false, "", browserlogin.ErrSecurityUncertain
	}
	return true, "TOTP 双重验证已启用，密钥已保存至服务器私有目录", nil
}

func (s *Service) passwordStep(ctx context.Context, value *securitySession, browser browserlogin.SecurityBrowser) (bool, string, error) {
	if !value.passwordBegun {
		identity, err := browser.Identity(ctx)
		if err != nil {
			return false, "请先在官方页面完成所选账号登录", err
		}
		if !sameSecurityIdentity(identity, value.expected) {
			return false, "", browserlogin.ErrSecurityIdentity
		}
		// Mark before sending; a lost response must never repeat reauthentication.
		value.passwordBegun = true
		if err := browser.BeginPassword(ctx, value.expected); err != nil {
			return false, "", browserlogin.ErrSecurityUncertain
		}
	}
	if !value.passwordSubmitted {
		page, err := browser.InspectAuth(ctx)
		if err != nil {
			return false, "请完成官方密码验证页面，完成后继续", err
		}
		if page.Stage != "new_password" {
			return false, "请完成官方邮箱验证，进入新密码页面后继续", nil
		}
		action := browserlogin.AuthAction{Stage: "new_password", Revision: page.Revision, Value: value.password}
		if action.Validate() != nil {
			return false, "", browserlogin.ErrAuthPageChanged
		}
		if value.artifactID == "" {
			if err := value.saveSecret("", ""); err != nil {
				return false, "", err
			}
		}
		if err := s.validateSecurityOrigin(ctx, value); err != nil {
			return false, "", err
		}
		value.passwordSubmitted = true
		if err := browser.SubmitPassword(ctx, action); err != nil {
			return false, "", browserlogin.ErrSecurityUncertain
		}
	}
	ok, err := browser.PasswordResult(ctx)
	if errors.Is(err, browserlogin.ErrSecurityPending) || (err == nil && !ok) {
		return false, "密码已提交，等待官方结果；请刷新后继续核对", nil
	}
	if err != nil {
		return false, "", browserlogin.ErrSecurityUncertain
	}
	return true, "账号密码已设置，并保存至服务器私有目录", nil
}
