package modelcheck

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
)

// CheckOAuthCandyWithProxy shares the animation precheck prompt and classifier.
func (s *Service) CheckOAuthCandyWithProxy(ctx context.Context, accountID, accountName string, credentials map[string]any, model string, timeout int, proxyURL string) (map[string]any, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = astraModel
	}
	if !validOAuthHeader(model, 200) || !utf8.ValidString(accountName) || utf8.RuneCountInString(accountName) > 200 || accountID == "" || !validOAuthHeader(accountID, 256) {
		return nil, errors.New("检测模型或账号信息无效")
	}
	if timeout < 1 || timeout > 120 {
		return nil, errors.New("检测超时必须为 1 到 120 秒")
	}
	if loginproxy.Validate(proxyURL) != nil {
		return nil, errors.New("检测代理配置无效")
	}
	credential, err := parseOAuthCredential(credentials)
	if err != nil {
		return nil, err
	}
	for _, secret := range credential.secrets {
		if strings.Contains(accountID, secret) || strings.Contains(accountName, secret) || strings.Contains(model, secret) {
			return nil, errors.New("检测账号信息或模型不能包含凭据")
		}
	}
	credential.proxyURL = proxyURL
	transport, closeTransport, err := s.animationOAuthTransport(ctx, timeout, &credential)
	if err != nil {
		return nil, err
	}
	defer closeTransport()
	result := AnimationResult{AccountID: accountID, Model: model, Mode: precheckMode, ReasoningEffort: "medium", Usage: &AnimationUsage{}}
	sender := oauthBundleSender{stream: true, client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, credential: credential, models: []string{model}, slots: make(chan struct{}, 1), usage: result.Usage}
	err = runPrecheckQuestions(ctx, []string{"candy"}, &result, func(question AstraQuestion, requestID string) (string, string, error) {
		sender.requestID = requestID
		return sender.SendWithReasoning(ctx, accountID, model, question.Question, timeout, "medium")
	})
	verdict := "MISMATCH"
	if result.Precheck.Verdict == "passed" {
		verdict = "MATCH"
	}
	report := map[string]any{"verdict": verdict, "claimed_model": model, "response_model": result.ResponseModel, "precheck": result.Precheck, "reasoning_effort": "medium", "usage": result.Usage, "transport": "oauth-direct", "production_path_equivalent": false}
	if proxyURL != "" {
		report["transport"] = "oauth-proxy"
	}
	if err != nil {
		report["verdict"] = "ERROR"
		report["error"] = safeCredentialError(err)
		for _, question := range result.Precheck.Questions {
			if question.Error != "" {
				report["error"] = question.Error
				break
			}
		}
	}
	return report, nil
}
