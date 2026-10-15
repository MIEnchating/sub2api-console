package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// Runs within the generation task; failed reviews never replay generation or
// become human training labels. Reviews share the animation concurrency budget.
func (s *Service) autoAnimationQuality(ctx context.Context, result AnimationResult, notify func()) animationquality.Prediction {
	settings := s.AnimationQualitySettings()
	if !settings.Enabled {
		return s.animationQuality(result)
	}
	return s.reviewAnimationQuality(ctx, result, settings, notify)
}

func (s *Service) reviewAnimationQuality(ctx context.Context, result AnimationResult, settings AnimationQualitySettings, notify func()) animationquality.Prediction {
	prediction := animationquality.Prediction{Status: "unknown", Benchmark: animationquality.Benchmark(result.Prompt), Source: "model_review", RubricVersion: animationquality.ReviewRubricVersion, ModelVersion: settings.Model, ReviewerAccountID: settings.AccountID, SettingsVersion: settings.Version, ReviewedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if result.SourceTruncated || len(result.Source) > animationquality.MaxSourceBytes || strings.TrimSpace(result.Source) == "" || strings.TrimSpace(result.Prompt) == "" {
		prediction.Reason = "source_unavailable"
		return prediction
	}
	if notify != nil {
		notify()
	}
	// The configured timeout belongs to the request, not the shared queue.
	// Waiting still obeys the owning task's deadline and cancellation.
	releaseSlot, err := s.reserveAnimationSlot(ctx)
	if err != nil {
		prediction.Reason = "review_queue_timeout"
		if errors.Is(ctx.Err(), context.Canceled) {
			prediction.Reason = "review_cancelled"
		}
		return prediction
	}
	defer releaseSlot()
	if ctx.Err() != nil {
		prediction.Reason = qualityReviewRequestFailure(ctx, ctx, ctx.Err(), "review_failed")
		return prediction
	}
	if s.AnimationQualitySettings() != settings {
		prediction.Reason = "review_settings_changed"
		return prediction
	}
	account, err := s.qualityReviewer(ctx, settings.AccountID)
	if err != nil {
		prediction.Reason = qualityReviewRequestFailure(ctx, ctx, err, "review_account_unavailable")
		return prediction
	}
	guarded, release, err := s.acquirePreparedAccounts(ctx, []selectedAccount{account})
	if err != nil {
		prediction.Reason = qualityReviewRequestFailure(ctx, ctx, err, "review_account_unavailable")
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			prediction.Reason = "review_queue_timeout"
		}
		return prediction
	}
	releaseCredentials := sync.OnceFunc(func() { _ = release() })
	defer releaseCredentials()
	// Both capacity and account-lease waits precede the execution deadline.
	reviewCtx, cancel := modelrequest.Start(ctx, time.Duration(settings.TimeoutSeconds)*time.Second, true)
	defer cancel()
	deadline, _ := modelrequest.ExecutionDeadline(reviewCtx)
	credentialCtx, cancelCredentials := context.WithDeadline(guarded, deadline)
	defer cancelCredentials()
	credential, oauth, err := s.prepareQualityReviewer(credentialCtx, account)
	// The lease protects credential resolution, not this read-only model call.
	// Keeping it across the stream would serialize every review on one account.
	releaseCredentials()
	if err != nil {
		prediction.Reason = qualityReviewRequestFailure(ctx, reviewCtx, err, "review_account_unavailable")
		return prediction
	}
	data, _ := json.Marshal(struct {
		Prompt string `json:"original_prompt"`
		Source string `json:"source"`
	}{result.Prompt, result.Source})
	output, actual, err := s.sendQualityReview(reviewCtx, account, credential, oauth, settings, animationquality.ReviewInstruction+string(data), result.RequestID+"-quality")
	if err != nil {
		prediction.Reason = qualityReviewRequestFailure(ctx, reviewCtx, err, "review_failed")
		return prediction
	}
	response := animationquality.ParseReview(output, result.Prompt, result.Source)
	current, err := s.qualityReviewer(reviewCtx, settings.AccountID)
	if err != nil || current != account || reviewCtx.Err() != nil {
		prediction.Reason = qualityReviewRequestFailure(ctx, reviewCtx, err, "review_account_unavailable")
		return prediction
	}
	if s.AnimationQualitySettings() != settings {
		prediction.Reason = "review_settings_changed"
		return prediction
	}
	prediction.Status = response.Status
	prediction.Reason = response.Reason
	prediction.Explanation = response.Explanation
	prediction.Issues = response.Issues
	prediction.Basis = response.Basis
	prediction.ResponseModel = safeCredentialText(actual)
	return prediction
}

func qualityReviewRequestFailure(taskCtx, requestCtx context.Context, err error, fallback string) string {
	if errors.Is(taskCtx.Err(), context.Canceled) {
		return "review_cancelled"
	}
	if errors.Is(taskCtx.Err(), context.DeadlineExceeded) {
		return "review_task_timeout"
	}
	var network net.Error
	if errors.Is(requestCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
		return "review_request_timeout"
	}
	return fallback
}
func (s *Service) prepareQualityReviewer(ctx context.Context, account selectedAccount) (directCredential, *oauthCredential, error) {
	if account.AccountType == "oauth" {
		store, ok := s.credentials.(targetguard.Store)
		if !ok {
			return directCredential{}, nil, errors.New("OAuth 评审账号的管理目标尚未配置")
		}
		pinned, err := targetguard.Pin(ctx, store)
		if err != nil {
			return directCredential{}, nil, err
		}
		credential, err := s.resolveOAuthCredential(pinned, account, true)
		return directCredential{}, credential, err
	}
	credential, err := s.resolveCredential(ctx, account)
	return credential, nil, err
}

func (s *Service) sendQualityReview(ctx context.Context, account selectedAccount, credential directCredential, oauth *oauthCredential, settings AnimationQualitySettings, prompt, requestID string) (string, string, error) {
	if oauth != nil {
		return s.runOAuthPromptTarget(ctx, account, oauth, settings.TimeoutSeconds, settings.Model, requestID, prompt)
	}
	client := &http.Client{Timeout: time.Duration(settings.TimeoutSeconds) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	output, actual, err := sendAnimation(ctx, client, credential, settings.Model, prompt, requestID, &AnimationUsage{}, nil)
	if strings.Contains(output, credential.Secret) || strings.Contains(actual, credential.Secret) {
		return "", "", errors.New("评审结果包含凭据")
	}
	return output, actual, err
}
