package business

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	AccountBlockDisabled          = "disabled"
	AccountBlockUnschedulable     = "unschedulable"
	AccountBlockExpired           = "expired"
	AccountBlockOverloaded        = "overloaded"
	AccountBlockRateLimited       = "rate_limited"
	AccountBlockTempUnschedulable = "temp_unschedulable"
	AccountBlockQuotaExceeded     = "quota_exceeded"
)

// AccountUpstreamBlock mirrors the account filters used by Sub2API routing.
// It deliberately excludes Console health decisions such as fused or paused.
func AccountUpstreamBlock(metadata map[string]any, schedulable *bool, now time.Time) string {
	block, _ := AccountUpstreamBlockDetails(metadata, schedulable, now)
	return block
}

// AccountUpstreamBlockDetails returns both the Sub2API routing block and a
// user-facing explanation. It deliberately excludes Console health decisions.
func AccountUpstreamBlockDetails(metadata map[string]any, schedulable *bool, now time.Time) (string, string) {
	statusText := strings.TrimSpace(metadataText(metadata, "status"))
	status := strings.ToLower(statusText)
	if status != "" && status != "active" {
		return AccountBlockDisabled, "账号已在 Sub2API 停用（状态 " + statusText + "），未记录停用原因"
	}
	if schedulable != nil && !*schedulable {
		return AccountBlockUnschedulable, "Sub2API 调度开关已关闭，但未记录触发原因"
	}
	if metadataBoolValue(metadata, "auto_pause_on_expired") {
		if expiresAt, ok := metadataTimeValue(metadata, "expires_at"); ok && !expiresAt.After(now) {
			return AccountBlockExpired, "账号已到期"
		}
	}
	if metadataWindowActive(metadata, "overload_until", now) {
		return AccountBlockOverloaded, accountBlockWindowReason("上游过载退避中", metadata, "overload_until")
	}
	if metadataWindowActive(metadata, "rate_limit_reset_at", now) {
		return AccountBlockRateLimited, accountBlockWindowReason("限流中", metadata, "rate_limit_reset_at")
	}
	if metadataWindowActive(metadata, "temp_unschedulable_until", now) {
		reason := metadataText(metadata, "temp_unschedulable_reason")
		if reason == "" {
			reason = "临时不可调度"
		}
		return AccountBlockTempUnschedulable, accountBlockWindowReason(reason, metadata, "temp_unschedulable_until")
	}
	accountType := strings.ToLower(strings.TrimSpace(metadataText(metadata, "type", "account_type")))
	if accountType == "apikey" || accountType == "bedrock" {
		for _, item := range []struct {
			label string
			keys  [2]string
		}{
			{label: "总配额", keys: [2]string{"quota_limit", "quota_used"}},
			{label: "日配额", keys: [2]string{"quota_daily_limit", "quota_daily_used"}},
			{label: "周配额", keys: [2]string{"quota_weekly_limit", "quota_weekly_used"}},
		} {
			limit, limitOK := metadataNumberValue(metadata, item.keys[0])
			used, usedOK := metadataNumberValue(metadata, item.keys[1])
			if limitOK && usedOK && limit > 0 && used >= limit {
				return AccountBlockQuotaExceeded, item.label + "已用尽"
			}
		}
	}
	return "", ""
}

func accountBlockWindowReason(prefix string, metadata map[string]any, key string) string {
	value, ok := metadataTimeValue(metadata, key)
	if !ok {
		return prefix
	}
	return prefix + "，" + value.Local().Format("01-02 15:04") + " 恢复"
}

func metadataText(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, found := metadata[key]; found && value != nil {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				return text
			}
		}
	}
	return ""
}

func metadataBoolValue(metadata map[string]any, key string) bool {
	value, found := metadata[key]
	if !found || value == nil {
		return false
	}
	if typed, ok := value.(bool); ok {
		return typed
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(value)))
	return err == nil && parsed
}

func metadataNumberValue(metadata map[string]any, key string) (float64, bool) {
	value, found := metadata[key]
	if !found || value == nil {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func metadataWindowActive(metadata map[string]any, key string, now time.Time) bool {
	value, ok := metadataTimeValue(metadata, key)
	return ok && value.After(now)
}

func metadataTimeValue(metadata map[string]any, key string) (time.Time, bool) {
	value, found := metadata[key]
	if !found || value == nil {
		return time.Time{}, false
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "0" || strings.EqualFold(text, "null") {
		return time.Time{}, false
	}
	if seconds, err := strconv.ParseFloat(text, 64); err == nil {
		whole, fraction := math.Modf(seconds)
		return time.Unix(int64(whole), int64(fraction*float64(time.Second))).UTC(), true
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}
