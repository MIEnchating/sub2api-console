package routing

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestRecoveryReasonExplainsUnmetConditionsAtFullScore(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	stopped := false
	item := &candidate{
		account: business.RoutingAccount{ID: "41", EffectiveState: "fused", Schedulable: &stopped, Metadata: map[string]any{}},
		rate:    big.NewRat(1, 1), health: Health{HealthScore: 100, SampleCount: 1, RecoveryPassStreak: 1},
		rows: []business.RoutingSample{{ObservedAt: now.Add(-10 * time.Second).Format(time.RFC3339Nano)}},
	}
	config := engineConfig{recoveryEnabled: true, recoveryTarget: 75, recoverySuccesses: 2, recoveryHold: time.Minute}
	previous := business.PreviousRoutingDecision{State: "fused", Payload: map[string]any{"fused_until": now.Add(30 * time.Second).Format(time.RFC3339Nano)}}
	applyInitialState(item, config, previous, now)
	for _, detail := range []string{"连续成功 1/2 次", "健康保持 10/60 秒", "熔断冷却剩余 30 秒"} {
		if !strings.Contains(item.reason, detail) {
			t.Fatalf("recovery reason %q missing %q", item.reason, detail)
		}
	}
	if item.state != "fused" || item.schedulable {
		t.Fatal("unmet recovery conditions must keep account fused")
	}
}

func TestRecoveryProgressMatchesAllGatesAndPersistsWithDecision(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	config := engineConfig{recoveryEnabled: true, recoveryTarget: 75, recoverySuccesses: 2, recoveryHold: time.Minute}
	item := &candidate{state: "fused", health: Health{HealthScore: 75, RecoveryPassStreak: 2}, fusedUntil: now,
		rows: []business.RoutingSample{{ObservedAt: now.Format(time.RFC3339Nano)}, {ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}}}
	status := publicDecision(item, "codex", config, now).Recovery
	if status == nil || !status.Ready || len(status.Conditions) != 6 || status.EvaluatedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("exact recovery boundaries should pass: %+v", status)
	}
	if decisionWrite(publicDecision(item, "codex", config, now)).Payload["recovery"] == nil {
		t.Fatal("decision persistence lost recovery progress")
	}
	for _, tc := range []struct {
		name   string
		change func(*candidate, *engineConfig)
		code   string
	}{
		{"disabled", func(_ *candidate, c *engineConfig) { c.recoveryEnabled = false }, "automatic_recovery"},
		{"score", func(i *candidate, _ *engineConfig) { i.health.HealthScore = 74 }, "health_score"},
		{"streak", func(i *candidate, _ *engineConfig) { i.health.RecoveryPassStreak = 1 }, "success_streak"},
		{"no timestamps", func(i *candidate, _ *engineConfig) { i.rows = nil }, "healthy_hold"},
		{"fatal", func(i *candidate, _ *engineConfig) { i.health.Fatal = true }, "fatal_error"},
		{"cooldown", func(i *candidate, _ *engineConfig) { i.fusedUntil = now.Add(time.Millisecond) }, "fuse_cooldown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, policy := *item, config
			tc.change(&current, &policy)
			status := recoveryStatus(&current, policy, now, true)
			if status.Ready {
				t.Fatal("unmet gate allowed recovery")
			}
			for _, condition := range status.Conditions {
				if condition.Code == tc.code && condition.Met {
					t.Fatalf("gate %s incorrectly passed", tc.code)
				}
			}
		})
	}
}

func TestRecoveryProgressIsAbsentAfterRecoveryOrForManualFuse(t *testing.T) {
	config := engineConfig{manualFusedAccounts: map[string]struct{}{"41": {}}}
	item := &candidate{account: business.RoutingAccount{ID: "41"}, state: "fused"}
	if publicDecision(item, "codex", config, time.Now()).Recovery != nil {
		t.Fatal("manual fuse must not promise automatic recovery")
	}
	item.state = "healthy"
	if publicDecision(item, "codex", engineConfig{}, time.Now()).Recovery != nil {
		t.Fatal("healthy account retained recovery progress")
	}
}
