package modelcheck

import (
	"context"
	"net/http"
	"time"
)

// RunEndpointVerification runs the regular model check with its default settings
// inside a caller-owned task. The caller owns credential cleanup and persistence;
// no managed-account evidence is written.
func (s *Service) RunEndpointVerification(ctx context.Context, endpoint AnimationCustomEndpoint, progress func(VerificationReport), behavior ...bool) (map[string]any, error) {
	request, _, err := prepareCustomAnimation(AnimationRequest{Custom: &endpoint})
	if err != nil {
		return nil, err
	}
	challenges, err := verificationChallenges()
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout:       45 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	credential := credentialResolution{value: directCredential{
		BaseURL: request.Custom.BaseURL, Secret: request.Custom.APIKey, Platform: request.Custom.Platform,
	}}
	return runVerification(ctx, client, client, credential, targetRequest{
		AccountName: "接入前探活", Model: request.Custom.Model, Rounds: 1, TimeoutSeconds: 45,
		PublicReference: len(behavior) > 1 && behavior[1], Fingerprint: true, BehaviorFingerprint: len(behavior) > 0 && behavior[0], Challenges: [][]verificationChallenge{challenges}, Progress: progress,
	})
}
