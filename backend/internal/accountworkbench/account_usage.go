package accountworkbench

import (
	"math"
	"strconv"
	"time"
)

// AccountUsage contains only the management platform's non-sensitive quota snapshot.
type AccountUsage struct {
	FiveHour  *AccountUsageWindow `json:"five_hour,omitempty"`
	SevenDay  *AccountUsageWindow `json:"seven_day,omitempty"`
	UpdatedAt string              `json:"updated_at,omitempty"`
}
type AccountUsageWindow struct {
	UsedPercent float64 `json:"used_percent"`
	ResetAt     string  `json:"reset_at,omitempty"`
}

func publicAccountUsage(extra map[string]any) *AccountUsage {
	updated := usageTimestamp(extra["codex_usage_updated_at"])
	result := &AccountUsage{UpdatedAt: updated}
	result.FiveHour = usageWindow(extra, "codex_5h_", updated)
	result.SevenDay = usageWindow(extra, "codex_7d_", updated)
	for _, prefix := range []string{"codex_primary_", "codex_secondary_"} {
		minutes := text(extra[prefix+"window_minutes"])
		if minutes == "300" && result.FiveHour == nil {
			result.FiveHour = usageWindow(extra, prefix, updated)
		}
		if minutes == "10080" && result.SevenDay == nil {
			result.SevenDay = usageWindow(extra, prefix, updated)
		}
	}
	if result.FiveHour == nil && result.SevenDay == nil {
		return nil
	}
	return result
}

func usageWindow(extra map[string]any, prefix, updated string) *AccountUsageWindow {
	used, err := strconv.ParseFloat(text(extra[prefix+"used_percent"]), 64)
	if err != nil || math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || used > 100 {
		return nil
	}
	result := &AccountUsageWindow{UsedPercent: used, ResetAt: usageTimestamp(extra[prefix+"reset_at"])}
	if result.ResetAt == "" && updated != "" {
		seconds, err := strconv.ParseInt(text(extra[prefix+"reset_after_seconds"]), 10, 64)
		if err == nil && seconds >= 0 && seconds <= 7*24*60*60 {
			observed, _ := time.Parse(time.RFC3339, updated)
			result.ResetAt = observed.Add(time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339)
		}
	}
	return result
}
func usageTimestamp(value any) string {
	parsed, err := time.Parse(time.RFC3339, text(value))
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339)
}
