package modelcheck

import (
	"context"
	"time"
)

// RunEndpointAnimation runs inside a caller-owned task. Credentials remain in
// memory; the caller owns their lifetime and persists only the public result.
func (s *Service) RunEndpointAnimation(ctx context.Context, endpoint AnimationCustomEndpoint, prompt string) (AnimationResult, error) {
	return s.runEndpointCheck(ctx, endpoint, prompt, "animation")
}

// RunEndpointCandy reuses the shared streaming question and verdict rules.
// The caller owns the task and credential cleanup; no account evidence is written.
func (s *Service) RunEndpointCandy(ctx context.Context, endpoint AnimationCustomEndpoint) (AnimationResult, error) {
	return s.runEndpointCheck(ctx, endpoint, "", precheckMode)
}

func (s *Service) runEndpointCheck(ctx context.Context, endpoint AnimationCustomEndpoint, prompt, mode string) (AnimationResult, error) {
	request, accounts, err := prepareCustomAnimation(AnimationRequest{Custom: &endpoint})
	if err != nil {
		return AnimationResult{}, err
	}
	if mode == "animation" {
		if err = validateAnimationPrompt(prompt); err != nil {
			return AnimationResult{}, err
		}
	}
	result := AnimationResult{AccountName: "接入前探活", Model: request.Custom.Model, Endpoint: request.Custom.BaseURL, Platform: request.Custom.Platform, RequestID: accounts[0].ID, Mode: mode, Status: "failed", Prompt: prompt}
	var questions []string
	if mode == precheckMode {
		questions = []string{"candy"}
	}
	err = s.runAnimationWithRetry(ctx, accounts[0], 120, request.Custom, questions, &result)
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result.Phase = "completed"
	if err != nil {
		err = animationVisibleError(err, request.Custom.APIKey)
		result.Error = err.Error()
	} else {
		result.Status = "succeeded"
	}
	return result, err
}
