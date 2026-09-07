package routing

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestReviewSuccessfulRecoveryProbesOverridePreFuseTraffic(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	enabled, rate := false, "1"
	repo := &routingRepositoryStub{policy: routingPolicy(), accounts: []business.RoutingAccount{{ID: "41", GroupName: "codex", Schedulable: &enabled, Multiplier: &rate, EffectiveState: "fused", Metadata: map[string]any{}}}}
	repo.samples = []business.RoutingSample{
		{AccountID: "41", Source: "active-probe", Result: "success", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}},
		{AccountID: "41", Source: "active-probe", Result: "success", ObservedAt: now.Add(-4 * time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}},
		{AccountID: "41", Source: "traffic", Result: "failed", FailureReason: "unauthorized", ObservedAt: now.Add(-10 * time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 401}},
	}
	service := NewService(repo)
	service.now = func() time.Time { return now }
	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.AccountDecisions["41"]
	if decision.RoutingState != "healthy" || !decision.Schedulable {
		t.Fatalf("successful recovery probes ignored: state=%s health=%g samples=%d reason=%s", decision.RoutingState, decision.HealthScore, decision.SampleCount, decision.Reason)
	}
}

func TestReviewRestoredBindingStillChecksNewFatalEvidence(t *testing.T) {
	config, err := parseEngineConfig(routingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	enabled := false
	item := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "codex", Schedulable: &enabled, EffectiveState: "binding_invalid", CatalogBindingState: "active", Metadata: map[string]any{}}, rate: big.NewRat(1, 1), health: Health{HealthScore: 0, SampleCount: 1, Fatal: true, LatestEvent: EventCredentialBad}}
	applyInitialState(item, config, business.PreviousRoutingDecision{}, time.Now())
	if item.schedulable {
		t.Fatalf("restored catalog binding bypassed fresh credential failure: state=%s reason=%s", item.state, item.reason)
	}
}

func TestReviewRecoveredAccountCancelsPendingAutomaticDeletion(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	policy := routingPolicy()
	policy["scoring"].(map[string]any)["short_window"] = 2
	policy["cleanup"] = map[string]any{"enabled": true, "action": "delete", "occurrences": 3, "window": 5, "min_fused_minutes": 30, "keep_last_in_group": false, "trigger_status_codes": []any{401}}
	enabled, rate := false, "1"
	repo := &routingRepositoryStub{policy: policy, accounts: []business.RoutingAccount{{ID: "41", GroupName: "codex", Schedulable: &enabled, Multiplier: &rate, EffectiveState: "fused", Metadata: map[string]any{}}}, cleanupState: map[string]time.Time{"41": now.Add(-40 * time.Minute)}}
	for _, minutes := range []int{1, 4} {
		repo.samples = append(repo.samples, business.RoutingSample{AccountID: "41", Source: "traffic", Result: "success", ObservedAt: now.Add(-time.Duration(minutes) * time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}})
	}
	for _, minutes := range []int{10, 11, 12} {
		repo.samples = append(repo.samples, business.RoutingSample{AccountID: "41", Source: "traffic", Result: "failed", FailureReason: "unauthorized", ObservedAt: now.Add(-time.Duration(minutes) * time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 401}})
	}
	service := NewService(repo)
	service.now = func() time.Time { return now }
	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision, target := result.AccountDecisions["41"], result.AccountTargets["41"]
	if decision.RoutingState != "healthy" {
		t.Fatalf("fixture should meet recovery conditions: %+v", decision)
	}
	if target.CleanupAction != nil {
		t.Fatalf("recovered account still scheduled for %s: state=%s score=%g schedulable=%v", *target.CleanupAction, decision.RoutingState, decision.HealthScore, decision.Schedulable)
	}
	if len(repo.cleanupWrites) != 1 || repo.cleanupWrites[0].EligibleSince != nil {
		t.Fatalf("recovery must clear the previous cleanup observation: %#v", repo.cleanupWrites)
	}
}

func TestReviewNeutralClientErrorsCannotTriggerCredentialOnlyDeletion(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	policy := routingPolicy()
	policy["cleanup"] = map[string]any{"enabled": true, "action": "delete", "occurrences": 3, "window": 5, "min_fused_minutes": 30, "keep_last_in_group": true, "only_auth_errors": true, "trigger_status_codes": []any{401, 403}}
	enabled, rate := true, "1"
	repo := &routingRepositoryStub{policy: policy, cleanupState: map[string]time.Time{"41": now.Add(-40 * time.Minute)}}
	for _, id := range []string{"41", "42"} {
		repo.accounts = append(repo.accounts, business.RoutingAccount{ID: id, GroupName: "codex", Schedulable: &enabled, Multiplier: &rate, EffectiveState: "healthy", Metadata: map[string]any{}})
	}
	for _, minutes := range []int{1, 2, 3} {
		repo.samples = append(repo.samples, business.RoutingSample{AccountID: "41", Source: "traffic", Result: "failed", FailureReason: "model access denied", ObservedAt: now.Add(-time.Duration(minutes) * time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 403}})
	}
	service := NewService(repo)
	service.now = func() time.Time { return now }
	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountDecisions["41"].SampleCount != 0 {
		t.Fatal("fixture should be neutral client errors")
	}
	target := result.AccountTargets["41"]
	if target.CleanupAction != nil {
		t.Fatalf("neutral 403 triggered credential-only %s; state=%s", *target.CleanupAction, target.DesiredHealth)
	}
}

func TestRecoveryProbesDoNotReplaceNewerTrafficFailure(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	traffic := business.RoutingSample{Source: "traffic", Result: "failed", ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano)}
	probe := business.RoutingSample{Source: "active-probe", Result: "success", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}
	rows := withRecoveryProbeEvidence([]business.RoutingSample{traffic}, []business.RoutingSample{traffic, probe}, now, 5*time.Minute, time.Time{})
	if len(rows) != 1 || rows[0].Source != "traffic" {
		t.Fatalf("newer traffic failure must remain authoritative: %#v", rows)
	}
}

func TestRecoveryProbeStreakExcludesProbesBeforeFuse(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	traffic := business.RoutingSample{Source: "traffic", Result: "failed", ObservedAt: now.Add(-10 * time.Minute).Format(time.RFC3339Nano)}
	before := business.RoutingSample{Source: "active-probe", Result: "success", ObservedAt: now.Add(-4 * time.Minute).Format(time.RFC3339Nano)}
	after := business.RoutingSample{Source: "active-probe", Result: "success", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}
	rows := withRecoveryProbeEvidence([]business.RoutingSample{traffic}, []business.RoutingSample{traffic, before, after}, now, 5*time.Minute, now.Add(-3*time.Minute))
	if len(rows) != 1 || rows[0].ObservedAt != "2026-09-07T11:59:00.000000000Z" {
		t.Fatalf("pre-fuse probe must not complete the new recovery streak: %#v", rows)
	}
}

func TestCleanupCredentialRequirementAlsoAppliesToExplicitGatewayCodes(t *testing.T) {
	for _, onlyAuth := range []bool{true, false} {
		t.Run(fmt.Sprintf("only_auth_%t", onlyAuth), func(t *testing.T) {
			policy := routingPolicy()
			policy["cleanup"] = map[string]any{"only_auth_errors": onlyAuth, "trigger_status_codes": []any{500}, "window": 5}
			config, err := parseEngineConfig(policy)
			if err != nil {
				t.Fatal(err)
			}
			item := &candidate{rows: []business.RoutingSample{{Source: "traffic", Result: "failed", Payload: map[string]any{"status_code": 500}}}}
			hits, _ := cleanupAccountHits([]*candidate{item}, config)
			want := 1
			if onlyAuth {
				want = 0
			}
			if hits != want {
				t.Fatalf("gateway failure count=%d want=%d", hits, want)
			}
		})
	}
}

func TestRestoredBindingPreservesManualPauseAndAllowsHealthyAccount(t *testing.T) {
	config, err := parseEngineConfig(routingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, paused := range []bool{false, true} {
		t.Run(fmt.Sprintf("paused_%t", paused), func(t *testing.T) {
			enabled := false
			item := &candidate{
				account: business.RoutingAccount{ID: "41", GroupName: "codex", Paused: paused, Schedulable: &enabled, EffectiveState: "binding_invalid", CatalogBindingState: "active", Metadata: map[string]any{}},
				rate:    big.NewRat(1, 1), health: Health{HealthScore: 100, SampleCount: 1, LatestEvent: EventHealthy},
			}
			applyInitialState(item, config, business.PreviousRoutingDecision{}, time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC))
			if item.schedulable == paused {
				t.Fatalf("restored binding violated pause state: paused=%t state=%s schedulable=%t", paused, item.state, item.schedulable)
			}
		})
	}
}
