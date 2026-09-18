package modelcheck

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func (s *Service) prepareOAuthTarget(ctx context.Context, accounts []selectedAccount) (*configstore.TargetSettings, error) {
	for _, account := range accounts {
		if account.AccountType != "oauth" {
			continue
		}
		store, ok := s.credentials.(targetguard.Store)
		if !ok {
			return nil, errors.New("OAuth 账号检测的管理目标尚未配置")
		}
		target, err := targetguard.Expected(ctx, store)
		if err != nil {
			return nil, errors.New("OAuth 账号检测的管理目标读取失败，请检查管理配置")
		}
		return &target, nil
	}
	return nil, nil
}

func (s *Service) resolveOAuthAccountCredential(ctx context.Context, account selectedAccount) (*oauthCredential, error) {
	if !strings.EqualFold(account.Platform, "openai") {
		return nil, errors.New("当前仅支持 OpenAI OAuth 账号的行为检测，请选择 OpenAI 账号")
	}
	store, ok := s.credentials.(targetguard.Store)
	if !ok {
		return nil, errors.New("OAuth 账号检测的管理目标尚未配置")
	}
	target, err := targetguard.Settings(ctx, store)
	if err != nil {
		return nil, errors.New("OAuth 账号检测的管理目标读取失败，请检查管理配置")
	}
	client, err := adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 1,
	}, nil)
	if err != nil {
		return nil, errors.New("OAuth 账号检测的管理配置无效，请检查管理地址和密钥")
	}
	remote, err := client.Account(ctx, account.ID)
	if err != nil {
		return nil, errors.New("OAuth 账号凭据读取失败，请检查管理连接和账号是否存在")
	}
	if stringField(remote, "type") != "oauth" || stringField(remote, "platform") != "openai" {
		return nil, errors.New("账号类型或平台已变化，请刷新账号后重新检测")
	}
	if proxyID := remote["proxy_id"]; proxyID != nil && proxyID != "" {
		return nil, errors.New("OAuth 账号配置了出站代理，当前直连检测无法使用该代理，请在账号工作台使用显式检测代理")
	}
	credentials, _ := remote["credentials"].(map[string]any)
	credential, err := parseOAuthCredential(credentials)
	if err != nil {
		return nil, err
	}
	for _, secret := range credential.secrets {
		if strings.Contains(account.Name, secret) {
			return nil, errors.New("OAuth 账号名称包含凭据，请修改账号名称后重试")
		}
	}
	return &credential, nil
}

func runPreparedCheck(ctx context.Context, directClient, oauthClient *http.Client, credential credentialResolution, prepared preparedRun, input targetRequest) (map[string]any, error) {
	if credential.err != nil {
		return nil, credential.err
	}
	var sender interface {
		bundleSender
		reasoningSender
	} = directBundleSender{client: directClient, credential: credential.value}
	if credential.oauth != nil {
		if checkerForModel(input.Model, prepared.claudeProfiles, prepared.solProfile) == "claude" {
			return nil, errors.New("OpenAI OAuth 账号不支持 Claude 模型，请选择 OpenAI 检测模型")
		}
		sender = oauthBundleSender{
			client: oauthClient, credential: *credential.oauth,
			models: append(append([]string(nil), prepared.solProfile.Models...), astraModel), slots: make(chan struct{}, 2),
		}
	}
	var result map[string]any
	var err error
	switch checkerForModel(input.Model, prepared.claudeProfiles, prepared.solProfile) {
	case "astra":
		result, err = runAstraCheck(ctx, sender, input)
	case "claude":
		result, err = runClaudeCheck(ctx, sender, prepared.claudeProfiles, input)
	default:
		result, err = runSolCheck(ctx, sender, prepared.solProfile, input)
	}
	if err == nil && credential.oauth != nil {
		result["transport"] = "oauth-direct"
		result["production_path_equivalent"] = false
	}
	return result, err
}
