package onboarding

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func (s *Service) runProbeCandy(ctx context.Context, credential probeCredential, model string) (ProbeResult, error) {
	result := ProbeResult{Status: "failed", RequestModel: model}
	if s.animations == nil {
		return result, errors.New("糖果题测试服务尚未就绪，请稍后重试")
	}
	platform := "openai"
	if credential.candidate.Platform != nil && strings.TrimSpace(*credential.candidate.Platform) != "" {
		platform = strings.ToLower(strings.TrimSpace(*credential.candidate.Platform))
		if platform == "claude" {
			platform = "anthropic"
		}
	}
	check, err := s.animations.RunEndpointCandy(ctx, modelcheck.AnimationCustomEndpoint{BaseURL: credential.auth.BaseURL, APIKey: credential.key.Secret, Platform: platform, Model: model})
	if check.RequestID != "" {
		result.Animation = &check
	}
	result.ActualModel, result.LatencyMS = check.ResponseModel, check.DurationMS
	if check.Precheck != nil && len(check.Precheck.Questions) > 0 {
		question := check.Precheck.Questions[0]
		result.ResponseText = question.Answer
		if question.Error != "" {
			result.Message = question.Error
			return result, errors.New(question.Error)
		}
	}
	if err != nil {
		result.Message = err.Error()
		return result, err
	}
	if check.Precheck == nil {
		return result, errors.New("糖果题未返回判定结果，请重试")
	}
	if check.Precheck.Verdict == "passed" {
		result.Status, result.Message = "passed", "糖果题测试通过"
	} else {
		result.Message = "糖果题测试未通过：回答不符合预期 21，可重试核对"
	}
	return result, nil
}
