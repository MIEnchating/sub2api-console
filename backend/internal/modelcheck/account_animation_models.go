package modelcheck

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// AccountAnimationModels reads upstream models for bound API keys and the
// management picker catalog for OAuth accounts. Catalog entries are suggestions,
// not evidence of successful generation.
func (s *Service) AccountAnimationModels(ctx context.Context, accountID string) ([]string, error) {
	if !stablePositiveID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	detail, err := s.accounts.Account(ctx, accountID)
	if err != nil {
		return nil, errors.New("账号详情读取失败，请刷新账号后重试")
	}
	if detail == nil {
		return nil, errors.New("账号不存在，请刷新账号列表")
	}
	account := directAccountSelection(detail.AccountStatus, detail)
	guarded, release, err := s.acquirePreparedAccounts(ctx, []selectedAccount{account})
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()
	if account.AccountType == "oauth" {
		return s.oauthAnimationModels(guarded, account)
	}
	credential, err := s.resolveCredential(guarded, account)
	if err != nil {
		return nil, err
	}
	return s.CustomAnimationModels(guarded, AnimationCustomEndpoint{
		BaseURL: credential.BaseURL, APIKey: credential.Secret, Platform: credential.Platform,
	})
}

func (s *Service) oauthAnimationModels(ctx context.Context, account selectedAccount) ([]string, error) {
	if account.Platform != "openai" {
		return nil, errors.New("当前仅支持 OpenAI OAuth 账号的动画模型读取")
	}
	store, ok := s.credentials.(targetguard.Store)
	if !ok {
		return nil, errors.New("OAuth 账号的管理目标尚未配置")
	}
	target, err := targetguard.Expected(ctx, store)
	if err != nil {
		return nil, errors.New("OAuth 账号的管理目标读取失败，请检查管理配置")
	}
	client, err := adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 2,
	}, nil)
	if err != nil {
		return nil, errors.New("OAuth 账号的管理配置无效，请检查管理地址和密钥")
	}
	remote, err := client.Account(ctx, account.ID)
	if err != nil {
		return nil, errors.New("OAuth 账号读取失败，请检查管理连接后重试")
	}
	if stringField(remote, "type") != "oauth" || stringField(remote, "platform") != "openai" {
		return nil, errors.New("账号类型或平台已变化，请刷新账号后重试")
	}
	// Read the same catalog as the management account picker without triggering
	// capability synchronization or persistence. Sub2API can supply configured
	// or default suggestions when live discovery is unavailable.
	models, err := client.AccountModels(ctx, account.ID)
	if err != nil {
		return nil, errors.New("OAuth 模型目录读取失败，请检查授权、账号代理或管理连接后重试；也可手动输入模型 ID")
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, target), store); err != nil {
		return nil, errors.New("管理目标在模型读取期间已变化，请重新获取模型")
	}
	return models, nil
}
