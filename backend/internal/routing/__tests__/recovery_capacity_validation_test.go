package routing_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"testing"
)

func TestRecoveryWithScalingDisabledAndUnconfirmedUpstreamKeepsAccountPaused(t *testing.T) {
	for _, status := range []string{"unknown", "stale"} {
		t.Run(status, func(t *testing.T) {
			account := upstreamCapacityAccount("41", 3, 10)
			paused := false
			account.Schedulable = &paused
			account.UpstreamConcurrencyStatus = status
			if status == "unknown" {
				account.UpstreamConcurrencyLimit = nil
			}
			repository := upstreamCapacityFixture(t, account)
			_, err := repository.UpdatePolicy(t.Context(), map[string]any{
				"auto_apply":      map[string]any{"schedulable": true, "concurrency": true},
				"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": false}},
			}, "test")
			if err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			if target.Schedulable == nil || *target.Schedulable || target.Concurrency != nil {
				t.Fatalf("unconfirmed capacity generated a recovery write: %+v", target)
			}
		})
	}
}

func TestActiveAccountWithMissingConcurrencyDoesNotGenerateUnverifiableWrite(t *testing.T) {
	account := upstreamCapacityAccount("41", 3, 10)
	account.Concurrency = nil
	repository := upstreamCapacityFixture(t, account)
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency != nil {
		t.Fatalf("missing current capacity generated a write: %+v", target)
	}
}

func TestRecoveryWithScalingDisabledResumesAfterUpstreamIsConfirmed(t *testing.T) {
	account := upstreamCapacityAccount("41", 3, 10)
	paused := false
	account.Schedulable = &paused
	account.UpstreamConcurrencyStatus = "stale"
	repository := upstreamCapacityFixture(t, account)
	_, err := repository.UpdatePolicy(t.Context(), map[string]any{"auto_apply": map[string]any{"schedulable": true}, "advanced_policy": map[string]any{"scaling": map[string]any{"enabled": false}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	service := routing.NewService(repository)
	result, err := service.Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	applyUpstreamCapacityReadback(repository, result)
	repository.accounts[0].UpstreamConcurrencyStatus = "known"
	result, err = service.Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("confirmed capacity did not allow normal recovery: %+v", target)
	}
}
