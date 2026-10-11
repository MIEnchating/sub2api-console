package modelcheck

import (
	"context"
	"time"
)

// RunEndpointAnimation runs inside a caller-owned task. Credentials remain in
// memory; the caller owns their lifetime and persists only the public result.
func (s *Service) RunEndpointAnimation(ctx context.Context, endpoint AnimationCustomEndpoint, prompt string) (AnimationResult, error) {
	request, accounts, err := prepareCustomAnimation(AnimationRequest{Custom: &endpoint})
	if err != nil {
		return AnimationResult{}, err
	}
	if err = validateAnimationPrompt(prompt); err != nil {
		return AnimationResult{}, err
	}
	result := AnimationResult{AccountName: "接入前探活", Model: request.Custom.Model, Endpoint: request.Custom.BaseURL, Platform: request.Custom.Platform, RequestID: accounts[0].ID, Mode: "animation", Status: "failed", Prompt: prompt}
	err = s.runAnimationWithRetry(ctx, accounts[0], 120, request.Custom, nil, &result)
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
