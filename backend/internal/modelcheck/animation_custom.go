package modelcheck

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// Custom credentials live only in the running task's memory, never its stored result.
type AnimationCustomEndpoint struct {
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Platform string `json:"platform"`
	Model    string `json:"model"`
}

func prepareCustomAnimation(request AnimationRequest) (AnimationRequest, []selectedAccount, error) {
	if len(request.Targets) != 0 {
		return request, nil, errors.New("自定义接口检测不能同时选择账号")
	}
	custom, err := validateAnimationEndpoint(*request.Custom)
	if err != nil {
		return request, nil, err
	}
	custom.Model = strings.TrimSpace(custom.Model)
	if custom.Model == "" || utf8.RuneCountInString(custom.Model) > 256 || strings.ContainsFunc(custom.Model, unicode.IsControl) {
		return request, nil, errors.New("请输入 1 到 256 个字符的有效模型 ID")
	}
	if strings.Contains(custom.Model, custom.APIKey) {
		return request, nil, errors.New("模型 ID 不能包含 API Key，请检查填写内容")
	}
	id, err := randomTaskID()
	if err != nil {
		return request, nil, err
	}
	id = "custom-" + id
	request.Custom = &custom
	request.Targets = []AnimationTarget{{AccountID: id, Model: custom.Model, Endpoint: custom.BaseURL, Platform: custom.Platform}}
	return request, []selectedAccount{{ID: id, Name: "自定义接口", Platform: custom.Platform}}, nil
}

func validateAnimationEndpoint(custom AnimationCustomEndpoint) (AnimationCustomEndpoint, error) {
	baseURL, err := configstore.ValidateBaseURL(custom.BaseURL)
	if err != nil || len(baseURL) > 2048 {
		return custom, errors.New("Base URL 必须是完整的 http 或 https 地址，不能包含用户名、密码、查询参数或片段")
	}
	custom.BaseURL = baseURL
	custom.APIKey = strings.TrimSpace(custom.APIKey)
	if custom.APIKey == "" || len(custom.APIKey) > 4096 || strings.ContainsFunc(custom.APIKey, func(r rune) bool { return r < 33 || r > 126 }) {
		return custom, errors.New("请输入有效的 API Key，不能包含空白或非 ASCII 字符，长度不能超过 4096 字节")
	}
	decodedURL, err := url.PathUnescape(custom.BaseURL)
	if err != nil || strings.Contains(custom.BaseURL, custom.APIKey) || strings.Contains(decodedURL, custom.APIKey) {
		return custom, errors.New("Base URL 不能包含 API Key，请检查填写内容")
	}
	if custom.Platform != "openai" && custom.Platform != "anthropic" {
		return custom, errors.New("自定义接口类型必须是 OpenAI 或 Anthropic")
	}
	return custom, nil
}

func (s *Service) animationCredential(ctx context.Context, account selectedAccount, custom *AnimationCustomEndpoint) (context.Context, func(), directCredential, error) {
	if custom != nil {
		return ctx, func() {}, directCredential{BaseURL: custom.BaseURL, Secret: custom.APIKey, Platform: custom.Platform}, nil
	}
	guarded, release, err := s.acquirePreparedAccounts(ctx, []selectedAccount{account})
	if err != nil {
		return ctx, nil, directCredential{}, err
	}
	credential, err := s.resolveCredential(guarded, account)
	if err != nil {
		release()
		return ctx, nil, directCredential{}, err
	}
	return guarded, func() { _ = release() }, credential, nil
}
