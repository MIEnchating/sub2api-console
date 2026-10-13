package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// GenerationCredentials is backend-only. It binds saved material to the current
// management target, stable account ID and verified workspace/user identity.
// Generation never rotates refresh tokens shared with the management platform.
func (s *Service) GenerationCredentials(ctx context.Context, target configstore.TargetSettings, account map[string]any) (map[string]any, string, error) {
	ctx, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private)
	if err != nil {
		return nil, "", err
	}
	s.rememberMu.Lock()
	defer s.rememberMu.Unlock()
	id := rememberedAccountID(target, text(account["id"]))
	raw, _, err := s.private.WorkbenchDocument(ctx, rememberedAccountKey(target, id))
	if err != nil {
		return nil, "", errors.New("工作台本地凭证读取失败，请稍后重试")
	}
	if len(raw) == 0 {
		return nil, "", nil
	}
	saved, err := s.readRememberedAccount(ctx, target, id)
	if err != nil {
		return nil, "", errors.New("工作台本地凭证无法读取，请重新登录账号")
	}
	if !sameAccountIdentity(account, saved.Input.Credentials) {
		return nil, "", errors.New("工作台保存的账号身份与当前账号不一致，请重新登录账号")
	}
	credential := saved.GenerationCredentials
	if text(credential["access_token"]) == "" {
		return nil, "", errors.New("工作台尚未保存此账号的生成凭证，请在账号工作台重新登录或导入后重试")
	}
	if !sameAccountIdentity(account, credential) {
		return nil, "", errors.New("工作台生成凭证的账号身份不一致，请重新登录账号")
	}
	if expires := text(credential["expires_at"]); expires != "" {
		seconds, err := accountExpirySeconds(expires)
		if seconds > 1e12 {
			seconds /= 1000
		}
		if err != nil || !time.Unix(seconds, 0).After(time.Now()) {
			return nil, "", errors.New("工作台保存的生成凭证已过期或有效期无效，请重新登录账号后重试")
		}
	}
	binding, err := s.accountProxy(ctx, target, account)
	if err != nil {
		return nil, "", err
	}
	return maps.Clone(credential), proxyBindingURL(binding), nil
}

func generationCredential(credentials map[string]any) map[string]any {
	result := map[string]any{}
	copyInputIdentity(result, credentials)
	for _, key := range []string{"access_token", "user_agent", "expires_at"} {
		if value, ok := credentials[key]; ok {
			result[key] = value
		}
	}
	return result
}

// Update only existing remembered material; maintenance must never resurrect a
// record deleted by the user. Persist before clearing a recovered authorization.
func (s *Service) saveGenerationCredential(ctx context.Context, target configstore.TargetSettings, account map[string]any, credentials map[string]any) error {
	s.rememberMu.Lock()
	defer s.rememberMu.Unlock()
	id := rememberedAccountID(target, text(account["id"]))
	raw, _, err := s.private.WorkbenchDocument(ctx, rememberedAccountKey(target, id))
	if err != nil || len(raw) == 0 {
		return err
	}
	saved, err := s.readRememberedAccount(ctx, target, id)
	if err != nil {
		return err
	}
	if !sameAccountIdentity(account, saved.Input.Credentials) || !sameAccountIdentity(account, credentials) {
		return ErrIdentity
	}
	saved.GenerationCredentials = generationCredential(credentials)
	raw, err = json.Marshal(saved)
	if err != nil {
		return err
	}
	_, err = s.private.SaveWorkbenchDocument(ctx, rememberedAccountKey(target, id), saved.Public.Revision, raw)
	return err
}
