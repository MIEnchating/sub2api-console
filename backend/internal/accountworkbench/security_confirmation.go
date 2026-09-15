package accountworkbench

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

type SecurityIdentityInput struct {
	Email     string `json:"email"`
	UserID    string `json:"user_id"`
	Confirmed bool   `json:"confirmed"`
}

func (s *Service) ConfirmSecurityIdentity(ctx context.Context, owner, id string, input SecurityIdentityInput) error {
	identity := browserlogin.SecurityIdentity{Email: input.Email, UserID: input.UserID}
	if !input.Confirmed || identity.Validate() != nil {
		return browserlogin.ErrSecurityConfirmation
	}
	value, err := s.securitySession(owner, id)
	if err != nil {
		return err
	}
	if err := s.validateSecurityOrigin(ctx, value); err != nil {
		return err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.checkpoint == nil || value.view.Status != "awaiting_confirmation" || value.browser == nil || value.view.IdentityConfirmed {
		return browserlogin.ErrSecurityConfirmation
	}
	if !sameSecurityIdentity(value.expected, identity) {
		return browserlogin.ErrSecurityIdentity
	}
	select {
	case value.commands <- securityCommand{confirmation: &identity}:
		value.view.Status, value.view.Message = "running", "正在复核并确认官方账号身份"
		return nil
	default:
		return errors.New("安全任务已在处理，请勿重复提交")
	}
}

func (s *Service) discoverSecurityIdentity(ctx context.Context, value *securitySession, browser browserlogin.SecurityBrowser) error {
	if err := s.validateSecurityOrigin(ctx, value); err != nil {
		return err
	}
	identity, err := browser.Identity(ctx)
	if err != nil {
		return err
	}
	if identity.Validate() != nil || value.checkpoint.expectedEmail != "" && !strings.EqualFold(identity.Email, value.checkpoint.expectedEmail) {
		return browserlogin.ErrSecurityIdentity
	}
	if templateTextHasSecret(identity.Email, []string{value.password, value.target.AdminKey, value.checkpoint.options.Options.ProxyURL}) || templateTextHasSecret(identity.UserID, []string{value.password, value.target.AdminKey, value.checkpoint.options.Options.ProxyURL}) {
		return browserlogin.ErrSecurityIdentity
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	value.expected = identity
	value.view.Email, value.view.UserID = identity.Email, identity.UserID
	return nil
}

func (s *Service) confirmSecurityIdentityStep(ctx context.Context, value *securitySession, browser browserlogin.SecurityBrowser, identity browserlogin.SecurityIdentity) error {
	if value.checkpoint == nil || !sameSecurityIdentity(value.expected, identity) {
		return browserlogin.ErrSecurityIdentity
	}
	confirmation, ok := browser.(browserlogin.SecurityIdentityConfirmation)
	if !ok {
		return browserlogin.ErrSecurityConfirmation
	}
	if err := s.reserveSecurityBatchIdentity(value, identity); err != nil {
		return err
	}
	if err := confirmation.ConfirmSecurityIdentity(ctx, identity); err != nil {
		return err
	}
	value.mu.Lock()
	value.view.IdentityConfirmed = true
	value.mu.Unlock()
	return nil
}
