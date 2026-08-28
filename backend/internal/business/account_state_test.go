package business

import (
	"testing"
	"time"
)

func TestNormalizeAccountStateUsesCanonicalStates(t *testing.T) {
	tests := map[string]string{
		"active":       AccountStateHealthy,
		"pass":         AccountStateHealthy,
		"success":      AccountStateHealthy,
		"ok":           AccountStateHealthy,
		"observing":    AccountStateDegraded,
		"hard_open":    AccountStateFused,
		"survivor":     AccountStateSurvivor,
		"paused":       AccountStatePaused,
		"inactive":     AccountStateDisabled,
		"out_of_scope": AccountStateExcluded,
		"unexpected":   AccountStateUnknown,
	}
	for input, want := range tests {
		if got := NormalizeAccountState(input); got != want {
			t.Fatalf("NormalizeAccountState(%q)=%q want=%q", input, got, want)
		}
	}
}

func TestGroupRuntimeStatusMatchesGuardianAvailabilityStates(t *testing.T) {
	disabled := GroupStatus{AccountCount: 2, SchedulingClosed: 2, DisabledAccounts: 2, ParticipationStatus: "participating"}
	if got := groupRuntimeStatus(disabled, false, 1); got != "all_fused" {
		t.Fatalf("disabled group status=%q", got)
	}
	fused := GroupStatus{AccountCount: 2, SchedulingClosed: 2, FusedAccounts: 2, ParticipationStatus: "participating"}
	if got := groupRuntimeStatus(fused, false, 1); got != "all_fused" {
		t.Fatalf("fused group status=%q", got)
	}
	healthy := GroupStatus{AccountCount: 1, SchedulingOpen: 1, HealthyAccounts: 1, ParticipationStatus: "participating"}
	if got := groupRuntimeStatus(healthy, false, 1); got != "healthy" {
		t.Fatalf("effective healthy group status=%q", got)
	}
	rateLimited := GroupStatus{
		Name: "codex", AccountCount: 1, SchedulingOpen: 1, DegradedAccounts: 1,
		RateLimitedAccounts: 1, ParticipationStatus: "participating",
	}
	if got := groupRuntimeStatus(rateLimited, false, 1); got != "rate_limited" {
		t.Fatalf("rate-limited-only group status=%q", got)
	}
	account := accountProjection{
		AccountStatus: AccountStatus{Health: "degraded"},
		latestEvents:  map[string]string{"codex": "rate_limited_or_exhausted"},
	}
	classified := GroupStatus{Name: "codex"}
	classifyGroupAccount(&classified, account, time.Time{})
	if classified.DegradedAccounts != 1 || classified.RateLimitedAccounts != 1 {
		t.Fatalf("rate-limited account classification=%#v", classified)
	}
	pending := GroupStatus{AccountCount: 2, PendingAccounts: 2, AvailableAccounts: 2, ParticipationStatus: "participating"}
	if got := groupRuntimeStatus(pending, false, 1); got != "healthy" {
		t.Fatalf("all-pending group must remain healthy until evidence arrives: %q", got)
	}
}

func TestAvailableGroupAccountsDoesNotCountBlockedSurvivor(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	schedulable := true
	account := accountProjection{
		AccountStatus: AccountStatus{ID: "41", Groups: []string{"codex"}, Health: "survivor", Schedulable: &schedulable},
		metadataRaw:   `{"status":"inactive"}`,
	}
	if got := availableGroupAccounts([]accountProjection{account}, "codex", 3, now); got != 0 {
		t.Fatalf("已停用的保底账号不能计入可用池：%d", got)
	}
	account.metadataRaw = `{"status":"active","rate_limit_reset_at":"2026-08-27T13:00:00Z"}`
	if got := availableGroupAccounts([]accountProjection{account}, "codex", 3, now); got != 0 {
		t.Fatalf("限流窗口中的保底账号不能计入可用池：%d", got)
	}
}

func TestGroupAccountMetadataScopeMatchesRoutingScope(t *testing.T) {
	upstreamType := "newapi"
	account := accountProjection{
		AccountStatus: AccountStatus{ID: "41", UpstreamType: &upstreamType},
		metadataRaw:   `{"type":"oauth","platform":"openai"}`,
	}
	control := map[string]any{"scope": map[string]any{
		"account_types": []any{"apikey"}, "platforms": []any{"openai"},
	}}
	if groupAccountMetadataManaged(account, control) {
		t.Fatal("类型不匹配账号不应进入分组聚合")
	}
	control["scope"].(map[string]any)["account_types"] = []any{"oauth"}
	if !groupAccountMetadataManaged(account, control) {
		t.Fatal("类型和平台均匹配的账号应进入分组聚合")
	}
}

func TestAccountUpstreamBlockUsesSub2APIRuntimeFields(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	schedulable := true
	if got := AccountUpstreamBlock(map[string]any{"status": "active", "rate_limit_reset_at": now.Add(time.Hour).Format(time.RFC3339Nano)}, &schedulable, now); got != AccountBlockRateLimited {
		t.Fatalf("rate limit block=%q", got)
	}
	if got := AccountUpstreamBlock(map[string]any{"status": "active", "type": "apikey", "quota_limit": 100, "quota_used": 100}, &schedulable, now); got != AccountBlockQuotaExceeded {
		t.Fatalf("quota block=%q", got)
	}
}

func TestAccountUpstreamBlockReasonExplainsWhyTrafficIsStopped(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	schedulable := true
	metadata := map[string]any{
		"status":                    "active",
		"temp_unschedulable_until":  now.Add(time.Hour).Format(time.RFC3339Nano),
		"temp_unschedulable_reason": "令牌刷新失败",
	}
	block, reason := AccountUpstreamBlockDetails(metadata, &schedulable, now)
	if block != AccountBlockTempUnschedulable || reason != "令牌刷新失败，08-27 13:00 恢复" {
		t.Fatalf("block details=%q/%q", block, reason)
	}
}

func TestAccountUpstreamBlockReasonPrefersSchedulingSwitchLikeSub2API(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	schedulable := false
	tests := []struct {
		name     string
		metadata map[string]any
	}{
		{
			name: "temporary block",
			metadata: map[string]any{
				"status":                    "active",
				"temp_unschedulable_until":  now.Add(time.Hour).Format(time.RFC3339Nano),
				"temp_unschedulable_reason": "余额不足",
			},
		},
		{
			name: "rate limit",
			metadata: map[string]any{
				"status":              "active",
				"rate_limit_reset_at": now.Add(30 * time.Minute).Format(time.RFC3339Nano),
			},
		},
		{
			name: "quota exhausted",
			metadata: map[string]any{
				"status": "active", "type": "apikey", "quota_limit": 100, "quota_used": 100,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block, reason := AccountUpstreamBlockDetails(test.metadata, &schedulable, now)
			if block != AccountBlockUnschedulable || reason != "Sub2API 调度开关已关闭，但未记录触发原因" {
				t.Fatalf("block details=%q/%q", block, reason)
			}
		})
	}
}

func TestAccountUpstreamBlockReasonDoesNotInventReasonForSchedulingSwitch(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	schedulable := false
	block, reason := AccountUpstreamBlockDetails(map[string]any{"status": "active"}, &schedulable, now)
	if block != AccountBlockUnschedulable || reason != "Sub2API 调度开关已关闭，但未记录触发原因" {
		t.Fatalf("block details=%q/%q", block, reason)
	}
}
