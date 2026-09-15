package routing_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestCapacityRecoveryLowersPausedConcurrencyToConfirmedGlobalHeadroomBeforeResuming(t *testing.T) {
	for _, missingConcurrency := range []bool{false, true} {
		name := "retained concurrency exceeds remaining global capacity"
		if missingConcurrency {
			name = "missing concurrency is initialized within remaining global capacity"
		}
		t.Run(name, func(t *testing.T) {
			paused, other := upstreamCapacityAccount("41", 20, 100), upstreamCapacityAccount("42", 90, 100)
			inactive := false
			paused.Schedulable, paused.EffectiveState = &inactive, "concurrency_limited"
			if missingConcurrency {
				paused.Concurrency = nil
			}
			other.UpstreamID = "upstream-2"
			other.GroupName, other.GroupID = "", nil
			repository := upstreamCapacityFixture(t, paused, other)
			if _, err := repository.UpdatePolicy(t.Context(), map[string]any{"auto_apply": map[string]any{"schedulable": true, "concurrency": true}}, "test"); err != nil {
				t.Fatal(err)
			}
			service := routing.NewService(repository)
			scope := routing.Scope{AccountID: &paused.ID}
			result, err := service.Calculate(t.Context(), scope, true)
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			if target.Concurrency == nil || *target.Concurrency != 10 || target.Schedulable == nil || *target.Schedulable {
				t.Fatalf("paused capacity must be prepared within the ten confirmed global slots before reopening: %+v", target)
			}

			applyUpstreamCapacityReadback(repository, result)
			result, err = service.Calculate(t.Context(), scope, true)
			if err != nil {
				t.Fatal(err)
			}
			target = result.AccountTargets["41"]
			if target.Schedulable == nil || !*target.Schedulable || target.DesiredHealth == "concurrency_limited" {
				t.Fatalf("confirmed ten-slot configuration must recover without waiting for unnecessary extra capacity: %+v", target)
			}
		})
	}
}

func TestCapacityRecoveryKeepsPausedConfigurationWhenGlobalHeadroomIsZero(t *testing.T) {
	paused, other := upstreamCapacityAccount("41", 20, 100), upstreamCapacityAccount("42", 100, 100)
	inactive := false
	paused.Schedulable, paused.EffectiveState = &inactive, "concurrency_limited"
	other.UpstreamID = "upstream-2"
	other.GroupName, other.GroupID = "", nil
	repository := upstreamCapacityFixture(t, paused, other)
	if _, err := repository.UpdatePolicy(t.Context(), map[string]any{"auto_apply": map[string]any{"schedulable": true, "concurrency": true}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{AccountID: &paused.ID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency != nil || target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("zero global headroom must retain a capacity pause without writing unlimited zero: %+v", target)
	}
}
