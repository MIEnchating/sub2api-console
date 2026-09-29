package modelcheck

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func classifyAnimationFailure(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "已取消") {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "超时") {
		return "timeout"
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "凭据") || strings.Contains(message, "鉴权") || strings.Contains(message, "api key") || strings.Contains(message, "unauthorized") || strings.Contains(message, "http 401") || strings.Contains(message, "http 403") {
		return "credential"
	}
	if strings.Contains(message, "上游") || strings.Contains(message, "http 5") || strings.Contains(message, "接口返回") || strings.Contains(message, "无法连接") {
		return "upstream"
	}
	if strings.Contains(message, "响应") || strings.Contains(message, "参数") || strings.Contains(message, "base url") || strings.Contains(message, "格式") {
		return "validation"
	}
	return "local"
}

func (s *Service) persistAnimationEvidence(ctx context.Context, taskID string, results []AnimationResult) error {
	writer, ok := s.accounts.(animationEvidenceWriter)
	if !ok || len(results) == 0 {
		return nil
	}
	evidence := make([]business.AnimationEvidence, 0, len(results))
	for _, result := range results {
		verdict, failureKind := "error", ""
		errorMessage := result.Error
		if result.Mode == precheckMode && result.Precheck != nil {
			verdict = result.Precheck.Verdict
			if verdict == "error" {
				for _, question := range result.Precheck.Questions {
					if question.Error != "" {
						errorMessage = question.Error
						break
					}
				}
			}
		} else if result.Status == "succeeded" {
			verdict = "passed"
		} else {
			failureKind = classifyAnimationFailure(errors.New(result.Error))
		}
		if verdict == "error" {
			failureKind = classifyAnimationFailure(errors.New(errorMessage))
		}
		if result.Status == "succeeded" && verdict != "passed" && verdict != "not_passed" {
			failureKind = ""
		}
		observed := result.CompletedAt
		if strings.TrimSpace(observed) == "" {
			observed = time.Now().UTC().Format(time.RFC3339Nano)
		}
		evidence = append(evidence, business.AnimationEvidence{
			AccountID: result.AccountID, TaskID: taskID, Mode: result.Mode, Verdict: verdict,
			FailureKind: failureKind, Error: errorMessage, ObservedAt: observed,
		})
	}
	return writer.PersistAnimationEvidence(ctx, evidence)
}
