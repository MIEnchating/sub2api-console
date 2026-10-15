package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const animationMaximumAttempts = 3

type retryableAnimationError struct {
	err        error
	retryAfter time.Duration
}

func (err retryableAnimationError) Error() string { return err.err.Error() }
func (err retryableAnimationError) Unwrap() error { return err.err }

func animationRetryableReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return err
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return retryableAnimationError{err: err}
	}
	if errors.As(err, &network) && network.Temporary() {
		return retryableAnimationError{err: err}
	}
	return err
}

func animationRetryableStatusError(status int, header http.Header, raw []byte, err error) error {
	if status != http.StatusRequestTimeout && status != http.StatusTooManyRequests && status != 500 && status != 502 && status != 503 && status != 504 {
		return err
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) == nil {
		nested, _ := payload["error"].(map[string]any)
		for _, key := range []string{"code", "type"} {
			code := stringField(nested, key)
			if code == "insufficient_quota" || code == "billing_hard_limit_reached" || code == "invalid_api_key" {
				return err
			}
		}
	}
	return retryableAnimationError{err: err, retryAfter: animationRetryAfter(header.Get("Retry-After"))}
}

func animationRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		// Longer delays stop this task rather than retrying before the server's
		// minimum delay. Bound before conversion to avoid duration overflow.
		if seconds > 15 {
			return 16 * time.Second
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(time.Until(at), 0)
	}
	return 0
}

func animationRetryDelay(err error, retryCount int) (time.Duration, bool) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 0, false
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return 0, false
	}
	var retryable retryableAnimationError
	if errors.As(err, &retryable) {
		if retryable.retryAfter > 15*time.Second {
			return 0, false
		}
		if retryable.retryAfter > 0 {
			return retryable.retryAfter, true
		}
		return time.Duration(1<<retryCount) * 250 * time.Millisecond, true
	}
	return 0, false
}

// runAnimationWithRetry sends a new complete generation only after a clearly
// transient failure. Partial output is discarded; no upstream conversation is
// persisted or replayed.
func (s *Service) runAnimationWithRetry(ctx context.Context, account selectedAccount, timeout int, custom *AnimationCustomEndpoint, questions []string, result *AnimationResult, onPhase ...func()) error {
	result.ReasoningEffort = "low"
	if result.Prompt == "" {
		result.Prompt = animationPrompt
	}
	if result.Mode == precheckMode {
		result.ReasoningEffort = "medium"
		result.Prompt = ""
		result.Usage = &AnimationUsage{}
		started, err := s.runAnimationTarget(ctx, account, time.Duration(timeout)*time.Second, time.Duration(timeout)*time.Second, custom, questions, result, onPhase...)
		result.DurationMS = started.Milliseconds()
		result.GenerationDurationMS = result.DurationMS
		result.Usage.complete()
		return err
	}
	requestID := result.RequestID
	var elapsed time.Duration
	budget := time.Duration(timeout) * time.Second
	for attempt := 0; attempt < animationMaximumAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			result.DurationMS = elapsed.Milliseconds()
			return err
		}
		if elapsed >= budget {
			result.DurationMS = elapsed.Milliseconds()
			return context.DeadlineExceeded
		}
		result.RetryCount = attempt
		result.RequestID = requestID
		if attempt > 0 {
			result.RequestID = fmt.Sprintf("%s-retry-%d", requestID, attempt)
			result.Phase = "queued"
			for _, notify := range onPhase {
				if notify != nil {
					notify()
				}
			}
		}
		result.Usage = &AnimationUsage{}
		attemptDuration, err := s.runAnimationTarget(ctx, account, budget-elapsed, budget, custom, questions, result, onPhase...)
		elapsed += attemptDuration
		result.DurationMS = elapsed.Milliseconds()
		result.GenerationDurationMS = attemptDuration.Milliseconds()
		result.Usage.complete()
		if err == nil {
			// Publish generation independently from the optional quality review.
			result.Status = "succeeded"
			result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
			reviewStarted := time.Now()
			quality := s.autoAnimationQuality(ctx, *result, func() {
				result.Phase = "reviewing"
				for _, notify := range onPhase {
					if notify != nil {
						notify()
					}
				}
			})
			elapsed += time.Since(reviewStarted)
			result.Quality = &quality
			result.DurationMS = elapsed.Milliseconds()
			return nil
		}
		delay, retry := animationRetryDelay(err, attempt)
		if !retry || attempt+1 == animationMaximumAttempts {
			return err
		}
		timer := time.NewTimer(min(delay, max(budget-elapsed, 0)))
		waitStarted := time.Now()
		select {
		case <-ctx.Done():
			timer.Stop()
			elapsed += time.Since(waitStarted)
			result.DurationMS = elapsed.Milliseconds()
			return ctx.Err()
		case <-timer.C:
			elapsed += time.Since(waitStarted)
		}
	}
	result.DurationMS = elapsed.Milliseconds()
	return errors.New("动画检测重试次数已耗尽")
}
