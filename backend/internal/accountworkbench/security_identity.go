package accountworkbench

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func securityAccountIdentity(account map[string]any) (browserlogin.SecurityIdentity, error) {
	if stringValue(account["platform"]) != "openai" || stringValue(account["type"]) != "oauth" {
		return browserlogin.SecurityIdentity{}, errors.New("安全设置仅支持 OpenAI OAuth 账号")
	}
	credentials := inputObject(account["credentials"])
	extra := inputObject(account["extra"])
	identity := browserlogin.SecurityIdentity{Email: stringValue(credentials["email"]), UserID: stringValue(credentials["chatgpt_user_id"])}
	if alternate := stringValue(extra["chatgpt_user_id"]); alternate != "" && identity.UserID != "" && alternate != identity.UserID {
		return browserlogin.SecurityIdentity{}, errors.New("账号中的官方用户 ID 不一致，请先核对授权身份")
	}
	if alternate := stringValue(extra["email"]); alternate != "" && identity.Email != "" && !strings.EqualFold(alternate, identity.Email) {
		return browserlogin.SecurityIdentity{}, errors.New("账号中的官方邮箱不一致，请先核对授权身份")
	}
	if identity.Email == "" {
		identity.Email = stringValue(extra["email"])
	}
	if identity.UserID == "" {
		identity.UserID = stringValue(extra["chatgpt_user_id"])
	}
	if identity.Validate() != nil || templateTextHasSecret(identity.Email, templateSourceSecrets(account, "")) {
		return browserlogin.SecurityIdentity{}, errors.New("账号缺少有效官方邮箱或用户 ID，请先更新授权身份")
	}
	return identity, nil
}

func sameSecurityIdentity(left, right browserlogin.SecurityIdentity) bool {
	return left.Validate() == nil && right.Validate() == nil && left.UserID == right.UserID && strings.EqualFold(left.Email, right.Email)
}

// A lease is held only for a concrete remote step, never while waiting for the
// operator. The live stable account and management target are checked again.
func (s *Service) securityGuard(ctx context.Context, value *securitySession) (context.Context, func() error, error) {
	if value.checkpoint != nil {
		return s.securityCheckpointGuard(ctx, value)
	}
	if value.source != nil || value.batchSource != nil {
		return s.securitySourceGuard(ctx, value)
	}
	ctx = targetguard.Expect(ctx, value.target)
	guarded, release, err := targetguard.Acquire(ctx, s.repository, mutationguard.Account(value.view.AccountID))
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (context.Context, func() error, error) { _ = release(); return nil, nil, err }
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return fail(err)
	}
	client, err := s.clientFor(value.target)
	if err != nil {
		return fail(err)
	}
	account, err := client.Account(guarded, value.view.AccountID)
	if err != nil {
		return fail(errors.New("无法复核所选账号，请同步账号后重试"))
	}
	identity, err := securityAccountIdentity(account)
	if err != nil || !sameSecurityIdentity(identity, value.expected) {
		return fail(browserlogin.ErrSecurityIdentity)
	}
	if value.batchExpected != nil {
		current, err := securityBatchSnapshot(account, value.target)
		if err != nil || !sameSecurityBatchItem(current, *value.batchExpected) {
			return fail(browserlogin.ErrSecurityIdentity)
		}
	}
	return guarded, release, nil
}
