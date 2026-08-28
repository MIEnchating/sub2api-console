package business

import "strings"

const (
	AccountStateHealthy     = "healthy"
	AccountStateDegraded    = "degraded"
	AccountStateFused       = "fused"
	AccountStateCostBlocked = "cost_blocked"
	AccountStateSurvivor    = "survivor"
	AccountStatePaused      = "paused"
	AccountStateDisabled    = "disabled"
	AccountStateExcluded    = "excluded"
	AccountStateUnknown     = "unknown"
)

// NormalizeAccountState is the single backend mapping from upstream and engine
// aliases to the states exposed by account and group read models.
func NormalizeAccountState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "healthy", "active", "available", "normal", "pass", "passed", "success", "succeeded", "ok", "通过", "正常", "健康":
		return AccountStateHealthy
	case "degraded", "observing", "降级", "观察中":
		return AccountStateDegraded
	case "fused", "fuse_pending", "hard_open", "soft_open", "熔断", "已熔断":
		return AccountStateFused
	case "cost_blocked", "cost-wall-blocked", "成本墙拦截", "已被成本墙拦截":
		return AccountStateCostBlocked
	case "survivor":
		return AccountStateSurvivor
	case "paused", "暂停", "已暂停":
		return AccountStatePaused
	case "disabled", "inactive", "停用", "已停用":
		return AccountStateDisabled
	case "excluded", "out_of_scope", "排除", "已排除":
		return AccountStateExcluded
	default:
		return AccountStateUnknown
	}
}

func accountStatePriority(state string) int {
	switch NormalizeAccountState(state) {
	case AccountStatePaused:
		return 80
	case AccountStateDisabled:
		return 70
	case AccountStateFused:
		return 60
	case AccountStateCostBlocked:
		return 55
	case AccountStateSurvivor:
		return 50
	case AccountStateDegraded:
		return 40
	case AccountStateHealthy:
		return 30
	case AccountStateUnknown:
		return 20
	case AccountStateExcluded:
		return 10
	default:
		return 0
	}
}
