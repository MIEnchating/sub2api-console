package onboarding_test

import (
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestManualCreationAboveSharedLimitIsRejectedBeforeRemoteWrites(t *testing.T) {
	calls := 0
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "unexpected write", 500) }))
	t.Cleanup(admin.Close)
	service, repository, keys, request := newService(t, admin.URL, admin.URL)
	limit, requested := int64(5), int64(6)
	_, err := repository.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}})
	if err != nil {
		t.Fatal(err)
	}
	request.Concurrency = &requested
	_, err = service.Onboard(t.Context(), request)
	if err == nil || calls != 0 || keys.creates != 0 {
		t.Fatalf("over-budget creation reached remote: calls=%d keys=%+v err=%v", calls, keys, err)
	}
}

func TestCreationPreviewSplitsRemainingCapacityAfterExistingPausedAccount(t *testing.T) {
	service, repository, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	limit := int64(13)
	platform, status, rate := "gemini", "active", "0.2"
	_, err := repository.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test",
		Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"},
		Catalog: &business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{
			{GroupID: "6", Name: "first", Platform: &platform, Status: &status, RawRate: &rate},
			{GroupID: "7", Name: "second", Platform: &platform, Status: &status, RawRate: &rate},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	current := int64(4)
	if err := repository.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{
		OperationID: "existing", AccountID: "41", AccountName: "existing", Platform: "gemini", UpstreamHost: "upstream.test", UpstreamType: "sub2api", UpstreamKeyID: "81", UpstreamGroupID: "8", UpstreamGroupName: "existing", LocalGroupID: "3", LocalGroupName: "gemini-平价", Multiplier: "0.2", Concurrency: &current, Schedulable: false, ReadbackConfirmed: true,
	}); err != nil {
		t.Fatal(err)
	}
	second := request
	second.UpstreamGroupID = "7"
	result, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0].Concurrency == nil || *result[0].Concurrency != 5 || result[1].Concurrency == nil || *result[1].Concurrency != 4 {
		t.Fatalf("remaining 9 slots must split 5/4: %+v", result)
	}
}

func TestCreationPreviewRejectsManualBatchSumAboveRemainingCapacity(t *testing.T) {
	service, repository, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	limit, manual := int64(5), int64(3)
	_, err := repository.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}})
	if err != nil {
		t.Fatal(err)
	}
	request.Concurrency = &manual
	if _, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request, request}); err == nil {
		t.Fatal("manual batch exceeded shared limit")
	}
}

func TestCreationPreviewRejectsStaleLimitInsteadOfFallingBackToDefaults(t *testing.T) {
	service, repository, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	if err := repository.RecordUpstreamSyncFailure(t.Context(), "upstream.test", "balance", "isolated sync failure", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request}); err == nil {
		t.Fatal("stale limit authorized creation")
	}
}

func TestCreationPreviewUnlimitedUpstreamKeepsConfiguredDefault(t *testing.T) {
	service, _, _, request := newService(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	result, err := service.PreviewConcurrency(t.Context(), []onboarding.Request{request})
	if err != nil {
		t.Fatal(err)
	}
	if result[0].Concurrency == nil || *result[0].Concurrency != 10 {
		t.Fatalf("unlimited default: %+v", result)
	}
}

func TestCreationRechecksRemoteReservationBeforeCreatingKey(t *testing.T) {
	writes := 0
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":41,"concurrency":9,"schedulable":false}}`))
			return
		}
		writes++
		http.Error(w, "unexpected request", 500)
	}))
	t.Cleanup(admin.Close)
	service, repository, keys, request := newService(t, admin.URL, admin.URL)
	limit, current, manual := int64(10), int64(2), int64(5)
	_, err := repository.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{
		OperationID: "existing", AccountID: "41", AccountName: "existing", Platform: "gemini", UpstreamHost: "upstream.test", UpstreamType: "sub2api", UpstreamKeyID: "81", UpstreamGroupID: "8", UpstreamGroupName: "existing", LocalGroupID: "3", LocalGroupName: "gemini-平价", Multiplier: "0.2", Concurrency: &current, Schedulable: false, ReadbackConfirmed: true,
	}); err != nil {
		t.Fatal(err)
	}
	request.Concurrency = &manual
	if _, err := service.Onboard(t.Context(), request); err == nil || !strings.Contains(err.Error(), "剩余额度 1") {
		t.Fatalf("remote reservation was ignored: %v", err)
	}
	if writes != 0 || keys.creates != 0 {
		t.Fatalf("over-budget request wrote remotely: writes=%d keys=%d", writes, keys.creates)
	}
}

func TestFiniteCreationUsesRemainingCapacityAsDefaultAndCommitsReadback(t *testing.T) {
	created := int64(0)
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
			value, err := strconv.ParseInt(body["concurrency"].(json.Number).String(), 10, 64)
			if err != nil {
				t.Error(err)
				return
			}
			created = value
			body["id"] = json.Number("77")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(admin.Close)
	service, repository, _, request := newService(t, admin.URL, admin.URL)
	limit := int64(7)
	_, err := repository.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "upstream.test", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: &limit, ProfileUserID: "17"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Onboard(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if created != 7 || result["concurrency"] != int64(7) {
		t.Fatalf("default did not use confirmed remaining slots: created=%d result=%+v", created, result)
	}
	inventory, err := repository.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 || inventory[0].Concurrency == nil || *inventory[0].Concurrency != 7 {
		t.Fatalf("creation not reserved locally: %+v", inventory)
	}
}
