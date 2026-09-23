package routingwrite_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func concurrencyRestoreFixture(t *testing.T) (*routingwrite.Service, *upstreamCapacityFixture, business.AccountRoutingTarget) {
	t.Helper()
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(1), 100, 100, true)
	disableCapacityFixtureGroupScaling(t, fixture)
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"scaling": map[string]any{"enabled": false}, "upstream_concurrency": map[string]any{"enabled": false},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	// Reproduce a previous scheduler-owned reduction, preserving its real
	// baseline and confirmed managed value through the normal writer.
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(1)},
	}, "test")
	if err != nil || result.Changed != 1 {
		t.Fatalf("prepare managed reduction: %+v %v", result, err)
	}
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
		"auto_apply":       map[string]any{"concurrency": false, "load_factor": true},
		"cooldown_seconds": 0,
	}, "test"); err != nil {
		t.Fatal(err)
	}
	calculated, err := routing.NewService(fixture.store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	target := calculated.AccountTargets["41"]
	return service, fixture, target
}

func TestDisabledCapacityRestoresManagedConcurrencyAndStillAllocatesLoad(t *testing.T) {
	service, fixture, target := concurrencyRestoreFixture(t)
	if target.Concurrency == nil || *target.Concurrency != 100 {
		t.Fatalf("disabled capacity retained stale managed reduction: %+v", target)
	}
	if target.LoadFactor == nil || *target.LoadFactor == "10" {
		t.Fatalf("capacity restoration must preserve independent load allocation: %+v", target)
	}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil || result.Failed != 0 || fixture.states["41"]["concurrency"] != 100 {
		t.Fatalf("restore original concurrency despite disabled scaling switch: %+v %v state=%+v", result, err, fixture.states["41"])
	}
	if fixture.states["41"]["load_factor"] == 10 {
		t.Fatalf("load allocation was not written: %+v", fixture.states["41"])
	}
	calculated, err := routing.NewService(fixture.store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := calculated.AccountTargets["41"]; target.Concurrency != nil {
		t.Fatalf("restored concurrency must remain stable on the next round: %+v", target)
	}
}

func TestConcurrencyRestorationRejectsChangedScopeBaselineOrRemoteValue(t *testing.T) {
	for _, scenario := range []string{"remote edited", "remote paused", "target changed", "baseline removed", "capacity reenabled", "monitor mode", "readback mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			service, fixture, target := concurrencyRestoreFixture(t)
			wantConcurrency := 1
			switch scenario {
			case "remote edited":
				fixture.states["41"]["concurrency"], wantConcurrency = 3, 3
			case "remote paused":
				fixture.states["41"]["schedulable"] = false
			case "target changed":
				target.Concurrency = writeCapacityPointer(200)
			case "baseline removed":
				baseline, _, err := fixture.store.RoutingBaseline(t.Context(), "41")
				if err != nil {
					t.Fatal(err)
				}
				if err := fixture.store.DeleteRoutingBaseline(t.Context(), "41", baseline.TargetFingerprint); err != nil {
					t.Fatal(err)
				}
			case "capacity reenabled":
				if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true}}}, "test"); err != nil {
					t.Fatal(err)
				}
				// Even a caller without a calculation fingerprint cannot turn a
				// restoration flag into an override of the latest capacity scope.
				target.CalculatedPolicyFingerprint = ""
			case "monitor mode":
				if _, err := fixture.store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
					t.Fatal(err)
				}
			case "readback mismatch":
				fixture.ignoreParameters = true
			}
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if fixture.states["41"]["concurrency"] != wantConcurrency || result.Changed != 0 {
				t.Fatalf("restoration bypassed changed authorization: %+v state=%+v", result, fixture.states["41"])
			}
			if scenario == "readback mismatch" {
				baseline, _, err := fixture.store.RoutingBaseline(t.Context(), "41")
				if err != nil || result.Failed != 1 || baseline.ManagedConcurrency == nil || *baseline.ManagedConcurrency != 1 {
					t.Fatalf("unconfirmed restore must preserve retry evidence: %+v %+v %v", result, baseline, err)
				}
			} else if result.RemoteWrite {
				t.Fatalf("invalid restoration must not submit fields: %+v", result)
			}
		})
	}
}
