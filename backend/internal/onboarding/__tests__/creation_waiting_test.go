package onboarding_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

func reserveAllCreationCapacity(t *testing.T, repository *business.Store) {
	t.Helper()
	limit := int64(5)
	if _, err := repository.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{
		OperationID: "existing", AccountID: "41", AccountName: "existing", Platform: "gemini",
		UpstreamHost: "upstream.test", UpstreamType: "sub2api", UpstreamKeyID: "81", UpstreamGroupID: "8",
		LocalGroupID: "3", LocalGroupName: "gemini-平价", Multiplier: "0.2", Concurrency: &limit,
		Schedulable: true, ReadbackConfirmed: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticPausedCreationPreviewWithNoCapacityReturnsPositiveWaitingAllocation(t *testing.T) {
	service, repository, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	reserveAllCreationCapacity(t, repository)
	allocations, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(allocations)
	if err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 1 || allocations[0].Concurrency == nil || *allocations[0].Concurrency != 1 || !strings.Contains(string(encoded), `"waiting_for_capacity":true`) {
		t.Fatalf("paused creation needs a positive configuration and explicit waiting state: %s", encoded)
	}
}

func TestWaitingCreationConfirmationPreservesPreviewWithoutReservingCapacity(t *testing.T) {
	service, repository, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	reserveAllCreationCapacity(t, repository)
	value := int64(1)
	request.Concurrency, request.WaitingForCapacity = &value, true
	allocations, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request})
	if err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 1 || !allocations[0].WaitingForCapacity || allocations[0].Concurrency == nil || *allocations[0].Concurrency != 1 {
		t.Fatalf("confirmation lost the stopped waiting allocation: %+v", allocations)
	}
}

func TestAutomaticBatchWithOneSlotForTwoAccountsPreviewsBothAsWaiting(t *testing.T) {
	service, repository, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	reserveAllCreationCapacity(t, repository)
	limit := int64(6)
	platform, status, rate := "gemini", "active", "0.2"
	if _, err := repository.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"},
		Catalog: &business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{
			{GroupID: "6", Name: "first", Platform: &platform, Status: &status, RawRate: &rate},
			{GroupID: "7", Name: "second", Platform: &platform, Status: &status, RawRate: &rate},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	second := request
	second.UpstreamGroupID = "7"
	allocations, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 2 {
		t.Fatalf("expected two allocations: %+v", allocations)
	}
	for _, allocation := range allocations {
		if !allocation.WaitingForCapacity || allocation.Concurrency == nil || *allocation.Concurrency != 1 {
			t.Fatalf("underfunded batch must wait without configuring zero: %+v", allocations)
		}
	}
}

func TestWaitingCreationRejectsAnActiveRequestBeforeRemoteWrites(t *testing.T) {
	service, repository, keys, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	reserveAllCreationCapacity(t, repository)
	value := int64(1)
	request.Concurrency, request.WaitingForCapacity, request.Schedulable = &value, true, true
	if _, err := service.Onboard(t.Context(), request); err == nil || !strings.Contains(err.Error(), "保持停用") {
		t.Fatalf("waiting flag must not bypass active capacity checks: %v", err)
	}
	if keys.creates != 0 {
		t.Fatal("invalid waiting request created a key")
	}
}

func TestAutomaticActiveCreationWithNoCapacityRejectsBeforeCreatingKey(t *testing.T) {
	service, repository, keys, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	reserveAllCreationCapacity(t, repository)
	request.Schedulable = true
	if _, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request}); err == nil {
		t.Fatal("active creation must not exceed shared capacity")
	}
	if keys.creates != 0 {
		t.Fatal("preview created an upstream key")
	}
}

func TestAutomaticPausedCreationWithNoCapacityCreatesConfirmedWaitingAccount(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41" {
			_, _ = w.Write([]byte(`{"data":{"id":41,"concurrency":5,"schedulable":true}}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/models/sync-upstream-preview") {
			_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash"]}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body["schedulable"] != false || body["concurrency"] != float64(1) {
				t.Errorf("waiting account must remain stopped with concurrency 1: %v", body)
			}
			body["id"] = 77
			_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(admin.Close)
	service, repository, _, request := newService(t, admin.URL, admin.URL)
	reserveAllCreationCapacity(t, repository)
	result, err := service.Onboard(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result["waiting_for_capacity"] != true || result["schedulable"] != false {
		t.Fatalf("creation result must describe the waiting state: %+v", result)
	}
	inventory, err := repository.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range inventory {
		if account.ID == "77" && account.EffectiveState == business.AccountStateConcurrencyLimited && account.Schedulable != nil && !*account.Schedulable {
			return
		}
	}
	t.Fatalf("waiting account missing from capacity inventory: %+v", inventory)
}

func TestWaitingCreationWithUnconfirmedRemoteStopDoesNotReleaseCapacity(t *testing.T) {
	var created map[string]any
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models/sync-upstream-preview") {
			_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash"]}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts" {
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Error(err)
				return
			}
			created["id"], created["schedulable"] = 77, true
			_ = json.NewEncoder(w).Encode(map[string]any{"data": created})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/77" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": created})
			return
		}
		http.Error(w, "isolated stop failure", http.StatusBadRequest)
	}))
	t.Cleanup(admin.Close)
	service, repository, _, request := newService(t, admin.URL, admin.URL)
	reserveAllCreationCapacity(t, repository)
	value := int64(1)
	request.Concurrency, request.WaitingForCapacity = &value, true
	if _, err := service.Onboard(t.Context(), request); err == nil || !strings.Contains(err.Error(), "未确认停用") {
		t.Fatalf("active remote account must not be treated as waiting: %v", err)
	}
	inventory, err := repository.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 || inventory[0].ID != "41" {
		t.Fatalf("unconfirmed creation must not commit a released reservation: %+v", inventory)
	}
	pending, err := repository.PendingOnboarding(t.Context(), request.Host, request.UpstreamGroupID, []string{request.LocalGroupID})
	if err != nil || pending == nil || pending.UpstreamAccountID != "77" {
		t.Fatalf("unconfirmed account must retain its stable pending identity: %+v %v", pending, err)
	}
}
