package business_test

import (
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestSyncedUserLimitFlowsThroughStrategyIntoPersistedAccountTargets(t *testing.T) {
	store, db := concurrencyStore(t)
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{
		"global_strategy": "price_first",
		"auto_apply":      map[string]any{"concurrency": true, "schedulable": true},
		"advanced_policy": map[string]any{"scaling": map[string]any{
			"enabled": true, "global_max_concurrency": 100, "min_per_account": 1, "max_per_account": 100,
			"step_up": 100, "step_down": 100,
		}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,upstream_type,concurrency,schedulable,multiplier,updated_at) VALUES
		('41','lower-cost','fixture.example','sub2api',1,1,'1','now'),
		('42','higher-cost','fixture.example',NULL,1,1,'4','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7'),('42','codex','7');`); err != nil {
		t.Fatal(err)
	}
	limit := int64(20)
	if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "fixture.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
			AccountID: id, GroupName: "codex", Result: "通过", EvidenceKey: id,
			ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 10, "output_tokens": 5},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	cheaper, expensive := result.AccountTargets["41"].Concurrency, result.AccountTargets["42"].Concurrency
	if cheaper == nil || expensive == nil || *cheaper <= *expensive || *cheaper+*expensive != 20 {
		t.Fatalf("synced budget was not allocated by price strategy: %#v", result.AccountTargets)
	}
	summary, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Hosts[0].TargetConcurrency == nil || *summary.Hosts[0].TargetConcurrency != 20 || summary.Hosts[0].AllocatedConcurrency == nil || *summary.Hosts[0].AllocatedConcurrency != 2 {
		t.Fatalf("calculation incorrectly changed confirmed capacity or omitted persisted targets: %#v", summary.Hosts[0])
	}
}

func TestConfirmedConcurrencyPauseIsVisibleAndReleasesCapacityForOtherScopes(t *testing.T) {
	store, db := monitoringProjectionStore(t)
	if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "fixture.example", BaseURL: "https://fixture.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET upstream_host='fixture.example',upstream_type='sub2api',
		concurrency=5,schedulable=0,routing_state='concurrency_limited' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Health != business.AccountStateConcurrencyLimited || account.Paused != nil && *account.Paused {
		t.Fatalf("automatic capacity pause was hidden or became a manual pause: %#v", account)
	}
	inventory, err := store.RoutingCapacityAccounts(t.Context())
	if err != nil || len(inventory) != 1 || inventory[0].EffectiveState != business.AccountStateConcurrencyLimited || inventory[0].Schedulable == nil || *inventory[0].Schedulable {
		t.Fatalf("confirmed pause was lost from the global inventory: %#v, %v", inventory, err)
	}
}

func TestIndependentAllocationUsesSyncedLimitWhileGlobalScalingAndConcurrencyWritesAreOff(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{
		"global_strategy": "price_first",
		"auto_apply":      map[string]any{"concurrency": false, "schedulable": false},
		"advanced_policy": map[string]any{
			"upstream_concurrency": map[string]any{"enabled": true},
			"scaling":              map[string]any{"enabled": false, "global_max_concurrency": 100, "step_down": 1},
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,upstream_type,concurrency,schedulable,multiplier,updated_at) VALUES
		('41','lower-cost','fixture.example','sub2api',10,1,'1','now'),
		('42','higher-cost','fixture.example','sub2api',10,1,'4','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7'),('42','codex','7');`); err != nil {
		t.Fatal(err)
	}
	limit := int64(6)
	if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "fixture.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
			AccountID: id, GroupName: "codex", Result: "通过", EvidenceKey: id,
			ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 10, "output_tokens": 5},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	cheaper, expensive := result.AccountTargets["41"].Concurrency, result.AccountTargets["42"].Concurrency
	if cheaper == nil || expensive == nil || *cheaper <= *expensive || *cheaper+*expensive != limit {
		t.Fatalf("independent limit reduction did not follow current strategy: %#v", result.AccountTargets)
	}
	summary, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Hosts[0].TargetConcurrency == nil || *summary.Hosts[0].TargetConcurrency != limit || summary.Hosts[0].AllocatedConcurrency == nil || *summary.Hosts[0].AllocatedConcurrency != 20 {
		t.Fatalf("independent reduction preview changed confirmed capacity or lost its target: %#v", summary.Hosts[0])
	}
}
