package routingwrite_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func reductionWriteTarget(t *testing.T, fixture *upstreamCapacityFixture, accountID string, concurrency int64) business.AccountRoutingTarget {
	t.Helper()
	accounts, err := fixture.store.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.ID == accountID {
			return business.AccountRoutingTarget{
				AccountID: accountID, GroupNames: []string{"capacity-group"}, Concurrency: &concurrency,
				UpstreamReductionID: account.UpstreamID, UpstreamReductionLimit: account.UpstreamConcurrencyLimit,
			}
		}
	}
	t.Fatalf("account %s missing from capacity inventory", accountID)
	return business.AccountRoutingTarget{}
}

func setReductionWritePolicy(t *testing.T, fixture *upstreamCapacityFixture, enabled bool) {
	t.Helper()
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
		"auto_apply": map[string]any{"concurrency": false, "schedulable": false, "priority": false, "load_factor": false},
		"advanced_policy": map[string]any{
			"upstream_concurrency": map[string]any{"enabled": enabled},
			"writeback":            map[string]any{"verification": false},
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestUpstreamReductionEnabledWithGenericWritesDisabledLowersConcurrency(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, true)
	setReductionWritePolicy(t, fixture, true)
	target := reductionWriteTarget(t, fixture, "41", 2)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 2 || result.Changed != 1 || result.Failed != 0 {
		t.Fatalf("dedicated reduction did not lower over-capacity account: states=%v result=%+v", fixture.states, result)
	}
}

func TestUpstreamReductionCapacityPausePreservesPositiveConcurrencyAndRecoverableState(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(1), 1, 1, true)
	setReductionWritePolicy(t, fixture, true)
	target := reductionWriteTarget(t, fixture, "41", 1)
	paused := false
	target.Concurrency, target.Schedulable, target.DesiredHealth = nil, &paused, business.AccountStateConcurrencyLimited
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	if fixture.states["41"]["schedulable"] != false || fixture.states["41"]["concurrency"] != 1 || result.Changed != 1 {
		t.Errorf("capacity pause must retain a positive limit: states=%v result=%+v", fixture.states, result)
	}
	fixture.mu.Unlock()
	accounts, err := fixture.store.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.ID == "41" && account.EffectiveState != business.AccountStateConcurrencyLimited {
			t.Errorf("capacity pause lost its recoverable state: %+v", account)
		}
	}
}

func TestUpstreamReductionStaleFiniteLimitStillPermitsConfirmedReduction(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, true)
	setReductionWritePolicy(t, fixture, true)
	target := reductionWriteTarget(t, fixture, "41", 2)
	if _, err := fixture.store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "capacity.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyStatus: business.UpstreamConcurrencyUnknown, ProfileUserID: "17"},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 2 || result.Changed != 1 {
		t.Fatalf("stale finite limit blocked a confirmed downward change: states=%v result=%+v", fixture.states, result)
	}
}

func TestUpstreamReductionUnsafeOrNoLongerNeededTargetNeverWrites(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		adjust func(*testing.T, *upstreamCapacityFixture, *business.AccountRoutingTarget)
	}{
		{name: "dedicated switch disabled", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			setReductionWritePolicy(t, fixture, false)
		}},
		{name: "monitoring mode", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			if _, err := fixture.store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "marker has different stable upstream", adjust: func(_ *testing.T, _ *upstreamCapacityFixture, target *business.AccountRoutingTarget) {
			target.UpstreamReductionID = "another-upstream"
		}},
		{name: "marker omits finite limit", adjust: func(_ *testing.T, _ *upstreamCapacityFixture, target *business.AccountRoutingTarget) {
			target.UpstreamReductionLimit = nil
		}},
		{name: "latest limit changed", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			reductionSyncLimit(t, fixture, writeCapacityPointer(12), "17")
		}},
		{name: "latest profile is unlimited", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			reductionSyncLimit(t, fixture, writeCapacityPointer(0), "17")
		}},
		{name: "changed identity has unknown limit", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			reductionSyncLimit(t, fixture, nil, "18")
		}},
		{name: "upstream is no longer Sub2API", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			if _, err := fixture.store.UpdateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
				Host: "capacity.example", BaseURL: "https://capacity.example", UpstreamType: "newapi", AuthMode: "newapi_user_login", RechargeRate: "1",
			}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "latest remote capacity already fits", adjust: func(_ *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			fixture.states["42"]["concurrency"] = 2
		}},
		{name: "paused sibling is not active capacity", adjust: func(_ *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			fixture.states["42"]["schedulable"] = false
		}},
		{name: "remote target already paused", adjust: func(_ *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			fixture.states["41"]["schedulable"] = false
		}},
		{name: "snapshot request fails", adjust: func(_ *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			fixture.failSnapshot = true
		}},
		{name: "snapshot omits sibling", adjust: func(_ *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			fixture.omitSibling = true
		}},
		{name: "sibling capacity cannot be confirmed", adjust: func(_ *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			fixture.states["42"]["concurrency"] = nil
		}},
		{name: "account manually excluded", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			if _, err := fixture.store.SetAccountScopeControl(t.Context(), "41", "exclude", "test"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "group outside scheduling scope", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"excluded_group_ids": []any{"7"}}, "test"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "manual priority protected", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			if _, err := fixture.store.AssignManualPriority(t.Context(), "41", 1, "10", 8, false, "test"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "manual pause policy protects active remote account", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			reductionSetScopeProtection(t, fixture, "paused_account_ids")
		}},
		{name: "manual fuse policy protects active remote account", adjust: func(t *testing.T, fixture *upstreamCapacityFixture, _ *business.AccountRoutingTarget) {
			reductionSetScopeProtection(t, fixture, "manual_fused_account_ids")
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, true)
			setReductionWritePolicy(t, fixture, true)
			target := reductionWriteTarget(t, fixture, "41", 2)
			scenario.adjust(t, fixture, &target)
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.states["41"]["concurrency"] != 8 || result.RemoteWrite || result.Changed != 0 {
				t.Fatalf("unsafe reduction changed account: states=%v result=%+v err=%v", fixture.states, result, err)
			}
		})
	}
}

func reductionSetScopeProtection(t *testing.T, fixture *upstreamCapacityFixture, field string) {
	t.Helper()
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
		"advanced_policy": map[string]any{"scope": map[string]any{field: []any{"41"}}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestUpstreamReductionRechecksPolicyAndIdentityAfterAcquiringLeases(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		policyChanged bool
		change        func(*testing.T, *upstreamCapacityFixture)
	}{
		{name: "dedicated switch disabled while waiting", policyChanged: true, change: func(t *testing.T, fixture *upstreamCapacityFixture) {
			setReductionWritePolicy(t, fixture, false)
		}},
		{name: "manual pause added while waiting", policyChanged: true, change: func(t *testing.T, fixture *upstreamCapacityFixture) {
			reductionSetScopeProtection(t, fixture, "paused_account_ids")
		}},
		{name: "manual fuse added while waiting", policyChanged: true, change: func(t *testing.T, fixture *upstreamCapacityFixture) {
			reductionSetScopeProtection(t, fixture, "manual_fused_account_ids")
		}},
		{name: "finite limit changed while waiting", change: func(t *testing.T, fixture *upstreamCapacityFixture) {
			reductionSyncLimit(t, fixture, writeCapacityPointer(12), "17")
		}},
		{name: "account changed upstream while waiting", change: func(t *testing.T, fixture *upstreamCapacityFixture) {
			fixture.states["41"]["upstream_host"] = ""
			if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{
				{"id": json.Number("7"), "name": "capacity-group"},
			}, "other-task"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, true)
			setReductionWritePolicy(t, fixture, true)
			target := reductionWriteTarget(t, fixture, "41", 2)
			repository := &changedCapacityMembershipStore{Store: fixture.store, change: func() {
				scenario.change(t, fixture)
			}}
			service := routingwrite.New(routingTarget{url: fixture.serverURL}, repository)
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.states["41"]["concurrency"] != 8 || result.RemoteWrite || result.Changed != 0 {
				t.Fatalf("stale plan bypassed changed policy or ownership: states=%v result=%+v err=%v", fixture.states, result, err)
			}
			if scenario.policyChanged {
				if err != nil || result.Failed != 0 || len(result.Results) != 1 || !result.Results[0].Skipped || result.Results[0].Reason == nil {
					t.Fatalf("policy change must defer the old reduction target: %+v err=%v", result, err)
				}
			} else if err == nil && result.Failed == 0 {
				t.Fatalf("changed authorization must report a rejected plan: %+v", result)
			}
		})
	}
}

func reductionSyncLimit(t *testing.T, fixture *upstreamCapacityFixture, limit *int64, userID string) {
	t.Helper()
	if _, err := fixture.store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "capacity.example", Balance: &business.UpstreamBalanceObservation{
			ConcurrencyLimit: limit, ConcurrencyStatus: business.UpstreamConcurrencyUnknown, ProfileUserID: userID,
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestUpstreamReductionMarkerNeverAuthorizesGrowthRecoveryOrZero(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		active bool
		adjust func(*business.AccountRoutingTarget)
	}{
		{name: "concurrency growth", active: true, adjust: func(target *business.AccountRoutingTarget) { target.Concurrency = writeCapacityPointer(9) }},
		{name: "zero means unlimited", active: true, adjust: func(target *business.AccountRoutingTarget) { target.Concurrency = writeCapacityPointer(0) }},
		{name: "negative concurrency", active: true, adjust: func(target *business.AccountRoutingTarget) { target.Concurrency = writeCapacityPointer(-1) }},
		{name: "resume paused account", active: false, adjust: func(target *business.AccountRoutingTarget) {
			enabled := true
			target.Schedulable = &enabled
		}},
		{name: "pause without capacity state", active: true, adjust: func(target *business.AccountRoutingTarget) {
			paused := false
			target.Concurrency, target.Schedulable, target.DesiredHealth = nil, &paused, business.AccountStatePaused
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, scenario.active)
			setReductionWritePolicy(t, fixture, true)
			// Even enabled generic writes cannot turn this dedicated marker into growth.
			if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
				"auto_apply": map[string]any{"concurrency": true, "schedulable": true},
			}, "test"); err != nil {
				t.Fatal(err)
			}
			target := reductionWriteTarget(t, fixture, "41", 2)
			scenario.adjust(&target)
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.states["41"]["concurrency"] != 8 || fixture.states["41"]["schedulable"] != scenario.active || result.RemoteWrite {
				t.Fatalf("dedicated reduction authorized another action: states=%v result=%+v err=%v", fixture.states, result, err)
			}
		})
	}
}

func TestUpstreamReductionDoesNotOverridePriorityOrLoadFactorSwitches(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, true)
	setReductionWritePolicy(t, fixture, true)
	target := reductionWriteTarget(t, fixture, "41", 2)
	priority, load := int64(500), "20"
	target.Priority, target.LoadFactor = &priority, &load
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || result.Failed != 0 || len(result.Results) != 1 {
		t.Fatalf("valid reduction was blocked by disabled unrelated fields: %+v", result)
	}
	for _, field := range []string{"priority", "load_factor"} {
		if _, wrote := result.Results[0].Desired[field]; wrote {
			t.Errorf("dedicated reduction overrode disabled %s write", field)
		}
	}
}

func TestUpstreamReductionBatchUsesOneInitialCapacitySnapshotForEveryTarget(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, true)
	setReductionWritePolicy(t, fixture, true)
	var mu sync.Mutex
	initialSnapshots := 0
	wrote := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		if request.Method == http.MethodPut || request.Method == http.MethodPost {
			wrote = true
		}
		if !wrote && request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts" {
			initialSnapshots++
		}
		mu.Unlock()
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": reductionWriteTarget(t, fixture, "41", 2),
		"42": reductionWriteTarget(t, fixture, "42", 4),
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 2 || fixture.states["42"]["concurrency"] != 4 || result.Changed != 2 {
		t.Fatalf("first reduction incorrectly prevented remaining batch changes: states=%v result=%+v", fixture.states, result)
	}
	mu.Lock()
	defer mu.Unlock()
	if initialSnapshots != 1 {
		t.Fatalf("batch must verify against one initial shared capacity snapshot, got %d", initialSnapshots)
	}
}

func TestUpstreamReductionForcesReadbackWhenGenericVerificationIsDisabled(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, true)
	setReductionWritePolicy(t, fixture, true)
	var mu sync.Mutex
	wrote := false
	readAfterWrite := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if request.Method == http.MethodPut || request.Method == http.MethodPost {
			wrote = true
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(request.URL.Path, "/bulk-update") {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"success_ids": []int64{41}, "failed_ids": []int64{}}})
				return
			}
			// The endpoint acknowledges the desired value but fails to retain it.
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"id": 41, "concurrency": 2, "schedulable": true, "priority": 1000, "load_factor": 10, "status": "active",
			}})
			return
		}
		if wrote && request.Method == http.MethodGet {
			readAfterWrite = true
		}
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": reductionWriteTarget(t, fixture, "41", 2),
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !wrote || !readAfterWrite || result.Failed != 1 || result.Changed != 0 {
		t.Fatalf("unconfirmed write was reported as effective: wrote=%t readAfterWrite=%t result=%+v", wrote, readAfterWrite, result)
	}
	accounts, err := fixture.store.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.ID == "41" && (account.Concurrency == nil || *account.Concurrency != 8) {
			t.Errorf("unconfirmed reduction released local capacity: %+v", account)
		}
	}
}
