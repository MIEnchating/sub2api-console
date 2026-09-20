package probe

import "context"

func scheduledProbeSkip(ctx context.Context, target Target, config Config) *Result {
	if config.beforeAttempt == nil {
		return nil
	}
	reason, err := config.beforeAttempt(ctx)
	if err != nil {
		result := skippedProbeResult(target, "探活执行条件读取失败，已跳过本次探活")
		result.FailureCode = "probe_schedule_unavailable"
		return &result
	}
	if reason != "" {
		result := skippedProbeResult(target, reason)
		return &result
	}
	return nil
}
