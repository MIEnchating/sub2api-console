package routing

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func recoveryStatus(item *candidate, config engineConfig, now time.Time, withCooldown bool) *business.AccountRecovery {
	span := recoverySpan(item.rows, item.health.RecoveryPassStreak, now)
	automaticDetail := "自动恢复已开启"
	if !config.recoveryEnabled {
		automaticDetail = "自动恢复未开启，请在调度策略中开启或手动恢复"
	}
	fatalDetail := "没有致命凭据错误"
	if item.health.Fatal {
		fatalDetail = "凭据仍存在致命错误，请修复后重新探活"
	}
	result := &business.AccountRecovery{
		EvaluatedAt: now.UTC().Format(time.RFC3339Nano), Ready: true,
		Conditions: []business.RecoveryCondition{
			{Code: "automatic_recovery", Met: config.recoveryEnabled, Detail: automaticDetail},
			{Code: "health_score", Met: item.health.HealthScore >= config.recoveryTarget, Detail: fmt.Sprintf("健康分 %.4g/%.4g 分", item.health.HealthScore, config.recoveryTarget)},
			{Code: "success_streak", Met: item.health.RecoveryPassStreak >= config.recoverySuccesses, Detail: fmt.Sprintf("连续成功 %d/%d 次", item.health.RecoveryPassStreak, config.recoverySuccesses)},
			{Code: "healthy_hold", Met: span >= config.recoveryHold, Detail: fmt.Sprintf("健康保持 %.0f/%.0f 秒", math.Floor(span.Seconds()), config.recoveryHold.Seconds())},
			{Code: "fatal_error", Met: !item.health.Fatal, Detail: fatalDetail},
		},
	}
	if withCooldown {
		remaining := math.Ceil(max(0, item.fusedUntil.Sub(now).Seconds()))
		detail := "熔断冷却已结束"
		if remaining > 0 {
			detail = fmt.Sprintf("熔断冷却剩余 %.0f 秒", remaining)
		}
		result.Conditions = append(result.Conditions, business.RecoveryCondition{Code: "fuse_cooldown", Met: remaining == 0, Detail: detail})
	}
	for _, condition := range result.Conditions {
		result.Ready = result.Ready && condition.Met
	}
	return result
}

func recoveryReason(status *business.AccountRecovery) string {
	var unmet []string
	for _, condition := range status.Conditions {
		if !condition.Met {
			unmet = append(unmet, condition.Detail)
		}
	}
	return "等待恢复：" + strings.Join(unmet, "；")
}
