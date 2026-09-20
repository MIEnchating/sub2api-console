package onboarding_test

import (
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAccountOutsideSharedScopeUsesConfiguredConcurrencyWithoutWaiting(t *testing.T) {
	service, repo, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	limit := int64(1)
	if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": false}, "upstream_concurrency": map[string]any{"enabled": true, "account_mode": "upstreams", "upstream_ids": []any{"other"}}}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request, request})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range result {
		if item.WaitingForCapacity || item.Concurrency == nil || *item.Concurrency != 10 {
			t.Fatalf("out of scope default: %+v", result)
		}
	}
}

func TestCreationOutsideSharedScopeCommitsConfiguredConcurrencyWithoutWaiting(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models/sync-upstream-preview") {
			_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash"]}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts" {
			var body map[string]any
			decoder := json.NewDecoder(r.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body["concurrency"] != json.Number("10") {
				t.Errorf("ordinary creation concurrency: %v", body["concurrency"])
			}
			body["id"] = json.Number("77")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(admin.Close)
	service, repo, _, request := newService(t, admin.URL, admin.URL)
	limit := int64(1)
	if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": false}}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := service.Onboard(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result["waiting_for_capacity"] != false || result["concurrency"] != int64(10) {
		t.Fatalf("ordinary creation result: %+v", result)
	}
	inventory, err := repo.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 || inventory[0].Concurrency == nil || *inventory[0].Concurrency != 10 || inventory[0].EffectiveState == business.AccountStateConcurrencyLimited {
		t.Fatalf("ordinary creation projection: %+v", inventory)
	}
}

func TestNewAccountAllocationChoiceAndScalingDetermineWhetherQuotaApplies(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		master, selected, scaling bool
		override                  *bool
		waiting                   bool
	}{
		{name: "selected upstream inherited", master: true, selected: true, waiting: true},
		{name: "explicit account off", master: true, selected: true, override: allocationBool(false)},
		{name: "explicit account on", master: true, override: allocationBool(true), waiting: true},
		{name: "master off wins", override: allocationBool(true)},
		{name: "scaling still applies outside shared scope", master: true, scaling: true, waiting: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
			limit := int64(1)
			if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}}); err != nil {
				t.Fatal(err)
			}
			mode := "selected"
			if tc.selected {
				mode = "all"
			}
			if _, err := repo.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": tc.scaling}, "upstream_concurrency": map[string]any{"enabled": tc.master, "account_mode": mode, "account_ids": []any{}}}}, "test"); err != nil {
				t.Fatal(err)
			}
			request.AllocationOverride = tc.override
			result, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request, request})
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range result {
				if item.WaitingForCapacity != tc.waiting {
					t.Fatalf("quota applicability: %+v", result)
				}
			}
		})
	}
}
func allocationBool(value bool) *bool { return &value }

func TestMixedNewAccountScopeReservesDisabledAccountBeforeAllocatingSelectedAccount(t *testing.T) {
	service, repo, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	limit := int64(13)
	if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}}); err != nil {
		t.Fatal(err)
	}
	outside := request
	outside.AllocationOverride = allocationBool(false)
	result, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request, outside})
	if err != nil {
		t.Fatal(err)
	}
	if *result[0].Concurrency != 3 || *result[1].Concurrency != 10 {
		t.Fatalf("selected allocation must reserve disabled new sibling: %+v", result)
	}
}

func TestNewSelectedAccountReservesOldWaitingAccountAfterItLeavesScope(t *testing.T) {
	service, repo, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	limit, current := int64(10), int64(1)
	if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{OperationID: "waiting", AccountID: "41", AccountName: "waiting", Platform: "gemini", UpstreamHost: "upstream.test", UpstreamType: "sub2api", UpstreamKeyID: "81", UpstreamGroupID: "8", UpstreamGroupName: "existing", LocalGroupID: "3", LocalGroupName: "gemini-平价", Multiplier: "0.2", Concurrency: &current, Schedulable: false, ReadbackConfirmed: true, WaitingForCapacity: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true, "account_overrides": map[string]any{"41": false}}}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request})
	if err != nil {
		t.Fatal(err)
	}
	if *result[0].Concurrency != 9 {
		t.Fatalf("old out-of-scope wait must reserve its configured slot: concurrency=%d", *result[0].Concurrency)
	}
}

func TestNewAccountWithScalingEnabledRespectsGlobalBudgetDuringPreview(t *testing.T) {
	service, repo, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	limit := int64(100)
	if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": true, "global_max_concurrency": 3}}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request, request})
	if err != nil {
		t.Fatal(err)
	}
	if *result[0].Concurrency != 2 || *result[1].Concurrency != 1 {
		t.Fatalf("new accounts bypassed global limit: %d/%d", *result[0].Concurrency, *result[1].Concurrency)
	}
}

func TestCreationWithScalingEnabledCommitsWithinGlobalBudget(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models/sync-upstream-preview") {
			_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash"]}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts" {
			var body map[string]any
			decoder := json.NewDecoder(r.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body["concurrency"] != json.Number("3") {
				t.Errorf("ordinary creation concurrency: %v", body["concurrency"])
			}
			body["id"] = json.Number("77")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(admin.Close)
	service, repo, _, request := newService(t, admin.URL, admin.URL)
	limit := int64(100)
	if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": true, "global_max_concurrency": 3}}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := service.Onboard(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result["waiting_for_capacity"] != false || result["concurrency"] != int64(3) {
		t.Fatalf("ordinary creation result: %+v", result)
	}
	inventory, err := repo.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 || inventory[0].Concurrency == nil || *inventory[0].Concurrency != 3 || inventory[0].EffectiveState == business.AccountStateConcurrencyLimited {
		t.Fatalf("ordinary creation projection: %+v", inventory)
	}
}

func TestManualCreationOverGlobalBudgetStopsBeforeCreatingUpstreamKey(t *testing.T) {
	service, repo, keys, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	if _, err := repo.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": true, "global_max_concurrency": 3}}}, "test"); err != nil {
		t.Fatal(err)
	}
	value := int64(4)
	request.Concurrency = &value
	_, err := service.Onboard(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), "全局") || keys.creates != 0 {
		t.Fatalf("global cap must reject before writes: %v keys=%d", err, keys.creates)
	}
}
