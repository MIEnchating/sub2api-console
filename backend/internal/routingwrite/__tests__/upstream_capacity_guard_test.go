package routingwrite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestUpstreamCapacityWriteRechecksSiblingCapacityAfterTargetWasCalculated(t *testing.T) {
	for _, refreshProjection := range []bool{false, true} {
		name := "external manual change"
		if refreshProjection {
			name = "another Console task committed its write"
		}
		t.Run(name, func(t *testing.T) {
			store, err := business.Open(filepath.Join(t.TempDir(), "upstream-capacity.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{
				"auto_apply":      map[string]any{"concurrency": true, "schedulable": true},
				"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true}},
			}, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
				Host: "capacity.example", BaseURL: "https://capacity.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
			}); err != nil {
				t.Fatal(err)
			}
			limit := int64(10)
			if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
				Host: "capacity.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"},
			}); err != nil {
				t.Fatal(err)
			}
			fixture := &upstreamCapacityFixture{states: map[string]map[string]any{}}
			for _, id := range []string{"41", "42"} {
				fixture.states[id] = map[string]any{
					"id": id, "name": "capacity-" + id, "schedulable": true,
					"priority": 1000, "load_factor": 10, "concurrency": 4, "status": "active",
					"groups": []any{json.Number("7")}, "upstream_host": "capacity.example", "upstream_type": "sub2api",
				}
			}
			groups := []map[string]any{{"id": json.Number("7"), "name": "capacity-group"}}
			if _, err := store.SyncManagementSnapshot(t.Context(), fixture.accounts(), groups, "test"); err != nil {
				t.Fatal(err)
			}
			// This target fits the calculated snapshot: 6 + 4 equals the user limit.
			desired := int64(6)
			targets := map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
			}
			// A different writer consumes the last two slots before Apply acquires its lease.
			fixture.states["42"]["concurrency"] = 6
			if refreshProjection {
				if _, err := store.SyncManagementSnapshot(t.Context(), fixture.accounts(), groups, "other-task"); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(fixture)
			t.Cleanup(server.Close)
			result, applyErr := routingwrite.New(routingTarget{url: server.URL}, store).Apply(t.Context(), targets, "test")
			if len(result.Results) != 1 || result.Results[0].Reason == nil || !result.Results[0].Skipped || result.Results[0].Error != nil {
				t.Fatalf("missing capacity waiting result: %+v", result)
			}
			for _, detail := range []string{"上限 10", "已分配及预留 10", "本次需新增 2", "剩余 0", "未执行"} {
				if !strings.Contains(*result.Results[0].Reason, detail) {
					t.Fatalf("missing %s in %s", detail, *result.Results[0].Reason)
				}
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.states["41"]["concurrency"] != 4 {
				t.Fatalf("stale expansion exceeded the shared limit after sibling consumed capacity: states=%v result=%+v err=%v", fixture.states, result, applyErr)
			}
		})
	}
}

func TestUpstreamCapacityWriteAppliesOnlyGrowthThatFitsCurrentCapacity(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		limit       *int64
		current     int
		other       int
		desired     int64
		readFails   bool
		omitSibling bool
		want        int
	}{
		{name: "finite growth fits", limit: writeCapacityPointer(10), current: 4, other: 4, desired: 6, want: 6},
		{name: "finite reduction remains possible above quota", limit: writeCapacityPointer(10), current: 8, other: 8, desired: 6, readFails: true, want: 6},
		{name: "unknown quota blocks growth", current: 4, other: 4, desired: 6, want: 4},
		{name: "unknown quota permits reduction", current: 4, other: 4, desired: 3, readFails: true, want: 3},
		{name: "explicit unlimited permits growth", limit: writeCapacityPointer(0), current: 4, other: 4, desired: 50, readFails: true, want: 50},
		{name: "snapshot failure blocks growth", limit: writeCapacityPointer(10), current: 4, other: 4, desired: 6, readFails: true, want: 4},
		{name: "missing sibling blocks growth", limit: writeCapacityPointer(10), current: 4, other: 4, desired: 6, omitSibling: true, want: 4},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, scenario.limit, scenario.current, scenario.other, true)
			fixture.failSnapshot, fixture.omitSibling = scenario.readFails, scenario.omitSibling
			_, _ = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &scenario.desired},
			}, "test")
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if actual := fixture.states["41"]["concurrency"]; actual != scenario.want {
				t.Fatalf("capacity write=%v, want %d", actual, scenario.want)
			}
			if scenario.desired < int64(scenario.current) && fixture.snapshotReads != 0 {
				t.Fatal("a capacity reduction unnecessarily required a full remote snapshot")
			}
		})
	}
}

func TestUpstreamCapacityRecoveryRechecksActualAccountConcurrency(t *testing.T) {
	for _, other := range []int{6, 8} {
		t.Run(fmt.Sprintf("sibling concurrency %d", other), func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, other, false)
			enabled := true
			_, _ = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: &enabled},
			}, "test")
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if actual := fixture.states["41"]["schedulable"]; actual != (other == 6) {
				t.Fatalf("recovery did not honor real remaining capacity: %v", fixture.states)
			}
		})
	}
}

func TestUpstreamCapacityBatchCannotSpendTheSameHeadroomTwice(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, true)
	desired := int64(6)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
		"42": {AccountID: "42", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	total := fixture.states["41"]["concurrency"].(int) + fixture.states["42"]["concurrency"].(int)
	waiting := 0
	for _, item := range result.Results {
		if item.Skipped && item.Error == nil {
			waiting++
		}
	}
	if total != 10 || result.Changed != 1 || result.Failed != 0 || waiting != 1 {
		t.Fatalf("batch reused capacity: total=%d result=%+v", total, result)
	}
	if fixture.snapshotReads != 1 {
		t.Fatalf("batch did not share one remote snapshot: %d reads", fixture.snapshotReads)
	}
}

func TestUpstreamCapacityBatchDoesNotSpendAnUnconfirmedReduction(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 6, 4, true)
	reduced, expanded := int64(4), int64(6)
	_, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &reduced},
		"42": {AccountID: "42", GroupNames: []string{"capacity-group"}, Concurrency: &expanded},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 4 || fixture.states["42"]["concurrency"] != 4 {
		t.Fatalf("an unconfirmed reduction funded another target: %v", fixture.states)
	}
}

func TestUpstreamCapacitySeparateTasksShareOneUpstreamReservation(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, true)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for _, id := range []string{"41", "42"} {
		wait.Go(func() {
			<-start
			desired := int64(6)
			_, _ = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
				id: {AccountID: id, GroupNames: []string{"capacity-group"}, Concurrency: &desired},
			}, "test")
		})
	}
	close(start)
	wait.Wait()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	total := fixture.states["41"]["concurrency"].(int) + fixture.states["42"]["concurrency"].(int)
	if total != 10 {
		t.Fatalf("separate task reservations reused upstream headroom: %v", fixture.states)
	}
}

func TestUpstreamCapacityRecoveryStopsWhenSnapshotCannotConfirmSibling(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, false)
	fixture.omitSibling = true
	enabled := true
	_, _ = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: &enabled},
	}, "test")
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["schedulable"] != false {
		t.Fatal("recovery enabled traffic without a complete upstream capacity snapshot")
	}
}

func TestUpstreamCapacityRecoveryDoesNotEnableAnAccountWhenConcurrencyWriteFails(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 20, 4, false)
	fixture.failParameters = true
	enabled, concurrency := true, int64(6)
	_, _ = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: &enabled, Concurrency: &concurrency},
	}, "test")
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["schedulable"] != false {
		t.Fatal("a failed concurrency reduction enabled the account with its old oversized capacity")
	}
}

func TestUpstreamCapacityConfigurationErrorPreventsPartialAutomaticWrites(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, true)
	configurationError := "请同时启用并发与可调度状态自动写入后执行"
	desired := int64(6)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired, ConfigurationError: &configurationError},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || len(result.Results) != 1 || result.Results[0].Error == nil || *result.Results[0].Error != configurationError {
		t.Fatalf("configuration error was hidden by partial success: %+v", result)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 4 {
		t.Fatal("a configuration error still allowed a partial concurrency write")
	}
}

func TestUpstreamCapacityCountsSiblingAccountsWithoutAnyGroup(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 6, true)
	fixture.states["42"]["groups"] = []any{}
	if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{{"id": json.Number("7"), "name": "capacity-group"}}, "test"); err != nil {
		t.Fatal(err)
	}
	desired := int64(6)
	_, _ = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
	}, "test")
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 4 {
		t.Fatal("ungrouped sibling capacity was omitted from the shared budget")
	}
}

func TestUpstreamCapacityRecoveryUsesNewerConcurrencySeenInBatchSnapshot(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, false)
	fixture.snapshotConcurrency = map[string]int{"41": 20}
	enabled := true
	_, _ = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: &enabled},
	}, "test")
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["schedulable"] != false {
		t.Fatal("newer oversized concurrency observed in the shared snapshot was ignored during recovery")
	}
}

type changedCapacityMembershipStore struct {
	*business.Store
	change func()
}

func (store *changedCapacityMembershipStore) RoutingCapacityAccounts(ctx context.Context) ([]business.RoutingAccount, error) {
	accounts, err := store.Store.RoutingCapacityAccounts(ctx)
	if store.change != nil {
		store.change()
		store.change = nil
	}
	return accounts, err
}

func TestUpstreamCapacityMembershipIsRecheckedAfterAcquiringLeases(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, true)
	repository := &changedCapacityMembershipStore{Store: fixture.store, change: func() {
		fixture.states["42"]["upstream_host"] = ""
		if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{{"id": json.Number("7"), "name": "capacity-group"}}, "other-task"); err != nil {
			t.Fatal(err)
		}
	}}
	desired := int64(6)
	_, err := routingwrite.New(routingTarget{url: fixture.serverURL}, repository).Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
	}, "test")
	if err == nil {
		t.Fatal("changed upstream membership was accepted under leases for the old members")
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 4 {
		t.Fatal("membership change was detected after already expanding capacity")
	}
}

func TestUpstreamCapacityUsesLatestUserLimitAfterAcquiringLeases(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, true)
	repository := &changedCapacityMembershipStore{Store: fixture.store, change: func() {
		limit := int64(8)
		if _, err := fixture.store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
			Host: "capacity.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"},
		}); err != nil {
			t.Fatal(err)
		}
	}}
	desired := int64(6)
	_, _ = routingwrite.New(routingTarget{url: fixture.serverURL}, repository).Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
	}, "test")
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 4 {
		t.Fatal("expansion used the earlier user limit after waiting for leases")
	}
}

func TestUpstreamCapacityStaleLimitBlocksGrowthAndRecoveryButAllowsReduction(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		active  bool
		desired *int64
		enable  *bool
		want    int
	}{
		{name: "growth", active: true, desired: writeCapacityPointer(6), want: 4},
		{name: "recovery", enable: writeEnabledPointer(true), want: 4},
		{name: "reduction", active: true, desired: writeCapacityPointer(3), want: 3},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, scenario.active)
			if err := fixture.store.RecordUpstreamSyncFailure(t.Context(), "capacity.example", "balance", "profile unavailable", false); err != nil {
				t.Fatal(err)
			}
			_, _ = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: scenario.desired, Schedulable: scenario.enable},
			}, "test")
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.states["41"]["concurrency"] != scenario.want || fixture.states["41"]["schedulable"] != scenario.active {
				t.Fatalf("stale capacity guard applied unsafe target: %v", fixture.states)
			}
		})
	}
}

func writeEnabledPointer(value bool) *bool { return &value }
