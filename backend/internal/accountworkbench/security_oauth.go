package accountworkbench

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func (s *Service) OAuthAfterSecurity(ctx context.Context, owner, id string, confirmed bool) (OAuthView, error) {
	if !confirmed {
		return OAuthView{}, errors.New("请确认在安全操作完成后发起新的账号授权")
	}
	value, err := s.securitySession(owner, id)
	if err != nil {
		return OAuthView{}, err
	}
	value.mu.Lock()
	ready := value.checkpoint != nil && value.view.Status == "succeeded" && value.view.IdentityConfirmed
	value.mu.Unlock()
	if !ready {
		return OAuthView{}, errors.New("请先完成并确认检查点账号的安全操作")
	}
	// Wait until the old browser has closed and released its reservation.
	select {
	case <-value.done:
	case <-ctx.Done():
		return OAuthView{}, ctx.Err()
	}
	value.op.Lock()
	defer value.op.Unlock()
	if _, err := s.securitySession(owner, id); err != nil {
		return OAuthView{}, err
	}
	value.mu.Lock()
	ready = value.view.Status == "succeeded"
	value.mu.Unlock()
	if !ready {
		return OAuthView{}, browserlogin.ErrSession
	}
	if value.restartedOAuthID != "" {
		return s.ReadOAuth(ctx, owner, value.restartedOAuthID)
	}
	if err := s.validateSecurityOrigin(ctx, value); err != nil {
		return OAuthView{}, err
	}
	ctx, _, err = s.bindWorkbenchScope(ctx, value.view.Scope, &value.target)
	if err != nil {
		return OAuthView{}, err
	}
	identity := value.expected
	input := OAuthStartInput{Scope: value.view.Scope, ProxyURL: value.checkpoint.options.Options.ProxyURL}
	input.callbacks.beforeLaunch = func(ctx context.Context, view OAuthView) error {
		if err := s.validateSecurityOrigin(ctx, value); err != nil {
			return err
		}
		session, err := s.oauthSession(owner, view.ID)
		if err != nil {
			return err
		}
		session.mu.Lock()
		session.expectedEmail = identity.Email
		session.expectedWorkspace = value.checkpoint.expectedWorkspace
		session.smsOriginTaskID = value.checkpoint.smsOriginTaskID
		session.mu.Unlock()
		return nil
	}
	input.callbacks.onAuthorized = func(_ context.Context, credentials map[string]any) error {
		actual := browserlogin.SecurityIdentity{Email: stringValue(credentials["email"]), UserID: stringValue(credentials["chatgpt_user_id"])}
		if !sameSecurityIdentity(actual, identity) {
			return browserlogin.ErrSecurityIdentity
		}
		return nil
	}
	view, err := s.StartOAuthWithInput(ctx, owner, input)
	if err != nil {
		return OAuthView{}, err
	}
	value.restartedOAuthID = view.ID
	return view, nil
}
