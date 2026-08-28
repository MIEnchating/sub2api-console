package business

import (
	"context"
	"testing"
)

func TestGroupAllocationUsesCurrentGroupDecisionMetrics(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE local_groups SET platform='openai',rate_multiplier='0.15' WHERE remote_id='1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateGroupPolicy(ctx, "1", map[string]any{
		"enabled": true, "strategy": "reliability", "min_pool_size": 1, "weight_budget": 500,
		"balanced_price_ratio": 0.5, "breaker_enabled": true, "recovery_enabled": true,
		"weights_enabled": true, "scaling_enabled": true, "probe_enabled": true,
		"probe_interval_seconds": 420, "probe_model": "gpt-5.1-codex",
	}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE routing_decisions SET
		priority=136,schedulable=1,role='healthy',routing_state='healthy',rank=1,reason='健康渠道',
		updated_at='2026-08-27T08:00:00Z',payload_json=? WHERE account_id='41' AND group_name='codex'`,
		`{"weight":117,"rate":"1.00","desired_concurrency":32}`); err != nil {
		t.Fatal(err)
	}

	allocation, err := store.GroupAllocation(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if allocation.GroupName != "codex" || allocation.AccountCount != 1 || len(allocation.Channels) != 1 {
		t.Fatalf("unexpected allocation summary: %#v", allocation)
	}
	if allocation.Platform == nil || *allocation.Platform != "openai" || allocation.RateMultiplier == nil || *allocation.RateMultiplier != "0.15" {
		t.Fatalf("group metadata missing: %#v", allocation)
	}
	if allocation.ProbeIntervalSeconds != 420 || allocation.WeightBudget != 500 || allocation.TotalWeight != 117 || !allocation.HasAllocation || allocation.Status == "" {
		t.Fatalf("group scheduling summary missing: %#v", allocation)
	}
	if allocation.HealthyAccounts != 1 || allocation.AvailableAccounts != 1 || allocation.UnavailableAccounts != 0 {
		t.Fatalf("decision state summary mismatch: %#v", allocation)
	}
	channel := allocation.Channels[0]
	if channel.HealthScore == nil || *channel.HealthScore != 82.5 || channel.Weight == nil || *channel.Weight != 117 {
		t.Fatalf("decision metrics missing: %#v", channel)
	}
	if channel.AssignedConcurrency == nil || *channel.AssignedConcurrency != 32 || allocation.AssignedConcurrency != 32 {
		t.Fatalf("assigned concurrency missing: %#v", allocation)
	}
	if channel.Rate == nil || *channel.Rate != "1.00" || channel.Priority == nil || *channel.Priority != 136 {
		t.Fatalf("routing detail missing: %#v", channel)
	}
}

func TestGroupAllocationDoesNotExposeLegacyAllocationBeforeCurrentEpoch(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `DELETE FROM app_state WHERE key='routing-decision-epoch'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE routing_decisions SET payload_json='{"strategy":"balanced","weight":0.5}' WHERE group_name='codex'`); err != nil {
		t.Fatal(err)
	}
	if err := initializeRoutingDecisionEpoch(ctx, store.db); err != nil {
		t.Fatal(err)
	}

	allocation, err := store.GroupAllocation(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if allocation.HasAllocation || allocation.TotalWeight != 0 || allocation.Channels[0].Weight != nil {
		t.Fatalf("legacy allocation remained visible: %#v", allocation)
	}
	if allocation.WeightBudget != 400 {
		t.Fatalf("effective budget missing without allocation: %#v", allocation)
	}
}

func TestGroupAllocationDoesNotExposeDecisionsBeforeCurrentModeEpoch(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE app_state SET updated_at='2026-08-26T11:00:00Z'
		WHERE key='routing-decision-epoch'`); err != nil {
		t.Fatal(err)
	}

	allocation, err := store.GroupAllocation(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(allocation.Channels) != 1 {
		t.Fatalf("unexpected channels: %#v", allocation.Channels)
	}
	channel := allocation.Channels[0]
	if channel.Weight != nil || channel.Priority != nil || channel.AssignedConcurrency != nil || channel.UpdatedAt != nil {
		t.Fatalf("stale routing decision leaked into allocation: %#v", channel)
	}
}

func TestGroupAllocationCountsUnprobedSchedulableAccountAsAvailable(t *testing.T) {
	schedulable := true
	allocation := GroupAllocation{}
	classifyAllocationChannel(&allocation, GroupAllocationChannel{
		Health: AccountStateUnknown, SampleCount: 0, Schedulable: &schedulable,
	})
	if allocation.PendingAccounts != 1 || allocation.AvailableAccounts != 1 || allocation.UnavailableAccounts != 0 {
		t.Fatalf("unprobed account classification=%#v", allocation)
	}
}

func TestGroupAllocationUsesEffectiveAccountStateInsteadOfPendingDecision(t *testing.T) {
	account := &accountProjection{AccountStatus: AccountStatus{Health: AccountStateHealthy}}
	if got := allocationHealth(account); got != AccountStateHealthy {
		t.Fatalf("effective health=%q", got)
	}
	if got := allocationHealth(nil); got != AccountStateUnknown {
		t.Fatalf("missing account health=%q", got)
	}
}
