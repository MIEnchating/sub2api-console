package onboarding

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

type probeAnimationPromptKey struct{}

// UseAnimationService shares prompt settings, concurrency and artifact validation.
// Configure once before serving requests.
func (s *Service) UseAnimationService(service *modelcheck.Service) { s.animations = service }

func (s *Service) runProbeAnimation(ctx context.Context, credential probeCredential, model string) (ProbeResult, error) {
	result := ProbeResult{Status: "failed", RequestModel: model}
	if s.animations == nil {
		return result, errors.New("动画测试服务尚未就绪，请稍后重试")
	}
	platform := "openai"
	if credential.candidate.Platform != nil && strings.TrimSpace(*credential.candidate.Platform) != "" {
		platform = strings.ToLower(strings.TrimSpace(*credential.candidate.Platform))
		if platform == "claude" {
			platform = "anthropic"
		}
	}
	prompt, ok := ctx.Value(probeAnimationPromptKey{}).(string)
	if !ok {
		prompt = s.animations.AnimationPromptSettings().Prompt
	}
	animation, err := s.animations.RunEndpointAnimation(ctx, modelcheck.AnimationCustomEndpoint{BaseURL: credential.auth.BaseURL, APIKey: credential.key.Secret, Platform: platform, Model: model}, prompt)
	if animation.RequestID != "" {
		result.Animation = &animation
	}
	result.ActualModel, result.LatencyMS = animation.ResponseModel, animation.DurationMS
	if err != nil {
		result.Message = err.Error()
		return result, err
	}
	result.Status, result.Message = "passed", "动画测试完成"
	return result, nil
}
