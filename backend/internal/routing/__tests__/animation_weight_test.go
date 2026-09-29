package routing_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

type animationRoutingRepository struct {
	policy   map[string]any
	accounts []business.RoutingAccount
	samples  []business.RoutingSample
	evidence []business.AnimationEvidence
}

func (r *animationRoutingRepository) ControlPolicy(context.Context) (map[string]any, error) {
	return r.policy, nil
}
func (r *animationRoutingRepository) RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error) {
	return r.accounts, nil
}
func (r *animationRoutingRepository) RoutingSamples(context.Context, *string, *string, string, int) ([]business.RoutingSample, error) {
	return r.samples, nil
}
func (r *animationRoutingRepository) PreviousRoutingDecisions(context.Context, *string, *string) ([]business.PreviousRoutingDecision, error) {
	return nil, nil
}
func (r *animationRoutingRepository) CleanupStates(context.Context, *string) (map[string]time.Time, error) {
	return nil, nil
}
func (r *animationRoutingRepository) PersistRoutingRound(context.Context, *string, *string, []business.RoutingEvaluationWrite, []business.RoutingDecisionWrite, []business.AccountRoutingTarget, []business.CleanupStateWrite, []business.RuntimeEventWrite, bool, time.Time) error {
	return nil
}
func (r *animationRoutingRepository) RoutingAnimationEvidence(context.Context, *string, *string) ([]business.AnimationEvidence, error) {
	return r.evidence, nil
}

func animationPolicy(action string, pass, fail float64) map[string]any {
	return map[string]any{
		"selection": map[string]any{"strategy": "balanced"},
		"weights":   map[string]any{"budget": 100, "scheduling_missing_rate_fallback": "fail_open", "performance_min_samples": 1, "change_threshold": "0.1", "cooldown_seconds": 0},
		"traffic":   map[string]any{"enabled": true}, "breaker": map[string]any{"enabled": false},
		"degrade": map[string]any{"enabled": true}, "recovery": map[string]any{}, "scaling": map[string]any{},
		"cleanup": map[string]any{"action": "none"}, "scope": map[string]any{},
		"group_policy_bindings": map[string]any{"7": map[string]any{
			"animation_enabled": true, "animation_pass_multiplier": pass, "animation_fail_multiplier": fail, "animation_failure_action": action,
		}},
	}
}

func animationAccount(id string) business.RoutingAccount {
	enabled, group, rate := true, "7", "1"
	return business.RoutingAccount{ID: id, Name: id, GroupName: "animation", GroupID: &group, Schedulable: &enabled, Multiplier: &rate, Metadata: map[string]any{}}
}

func animationSuccesses(ids ...string) []business.RoutingSample {
	now := time.Now().UTC().Add(-time.Minute)
	rows := make([]business.RoutingSample, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, business.RoutingSample{AccountID: id, GroupName: "animation", Source: "traffic", Result: "通过", ObservedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200, "input_tokens": 1, "output_tokens": 1}})
	}
	return rows
}

func TestAnimationVerdictsAdjustWeightsWithGroupConfiguredMultipliers(t *testing.T) {
	repository := &animationRoutingRepository{
		policy:   animationPolicy("ignore", 2, 0.5),
		accounts: []business.RoutingAccount{animationAccount("41"), animationAccount("42"), animationAccount("43")},
		samples:  animationSuccesses("41", "42", "43"),
		evidence: []business.AnimationEvidence{
			{AccountID: "41", GroupName: "animation", Verdict: "passed"},
			{AccountID: "42", GroupName: "animation", Verdict: "not_passed"},
			{AccountID: "43", GroupName: "animation", Verdict: "inconclusive"},
		},
	}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !(result.AccountDecisions["41"].Weight > result.AccountDecisions["43"].Weight && result.AccountDecisions["43"].Weight > result.AccountDecisions["42"].Weight) {
		t.Fatalf("animation verdict weights were not applied as configured: %+v", result.AccountDecisions)
	}
}

func TestAnimationFailureActionOnlyHandlesUpstreamFailures(t *testing.T) {
	for _, test := range []struct {
		action string
		kind   string
		state  string
		sched  bool
	}{
		{action: "ignore", kind: "upstream", state: "unknown", sched: true},
		{action: "degrade", kind: "upstream", state: "degraded", sched: true},
		{action: "fuse", kind: "upstream", state: "fused", sched: false},
		{action: "degrade", kind: "validation", state: "unknown", sched: true},
	} {
		t.Run(test.action+"-"+test.kind, func(t *testing.T) {
			accounts := []business.RoutingAccount{animationAccount("41")}
			if test.action == "fuse" && test.kind == "upstream" {
				accounts = append(accounts, animationAccount("42"))
			}
			repository := &animationRoutingRepository{
				policy: animationPolicy(test.action, 1, 1), accounts: accounts,
				evidence: []business.AnimationEvidence{{AccountID: "41", GroupName: "animation", Verdict: "error", FailureKind: test.kind, Error: "失败原因"}},
			}
			result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions["41"]
			if decision.RoutingState != test.state || decision.Schedulable != test.sched {
				t.Fatalf("unexpected failure action result: %+v", decision)
			}
		})
	}
}
