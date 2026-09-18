package routingwrite_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestPendingWriteIsSkippedWhenModeChangesDuringEarlierRequest(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
		"advanced_policy": map[string]any{"writeback": map[string]any{"concurrency": 1}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	var mutations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPut && mutations.Add(1) == 1 {
			if _, err := fixture.store.SetMode(request.Context(), runtimepolicy.Monitoring); err != nil {
				t.Error(err)
			}
		}
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	applied, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(5)},
		"42": {AccountID: "42", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)},
	}, "scheduler")
	if err != nil || mutations.Load() != 1 || applied.Changed != 1 || applied.Failed != 0 {
		t.Fatalf("pending writes must stop after switching to monitoring: writes=%d result=%+v err=%v", mutations.Load(), applied, err)
	}
	skipped := 0
	for _, item := range applied.Results {
		if item.Skipped && !item.RemoteWrite && item.Error == nil {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("the unsent account must have an explicit deferral: %+v", applied)
	}
}

func TestFailedWriteIsNotRetriedAfterModeChangesDuringRequest(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	var mutations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPut {
			mutations.Add(1)
			if _, err := fixture.store.SetMode(request.Context(), runtimepolicy.Monitoring); err != nil {
				t.Error(err)
			}
			http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
			return
		}
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	applied, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)},
	}, "scheduler")
	if err != nil || mutations.Load() != 1 || applied.Failed != 1 || len(applied.Results) != 1 || applied.Results[0].Skipped {
		t.Fatalf("retries must stop while retaining the original remote failure: writes=%d result=%+v err=%v", mutations.Load(), applied, err)
	}
}

func TestRecoveryDoesNotEnableTrafficWhenModeChangesAfterParameterWrite(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 8, 4, false)
	var schedulingWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/41/schedulable" {
			schedulingWrites.Add(1)
		}
		if request.Method == http.MethodPut {
			if _, err := fixture.store.SetMode(request.Context(), runtimepolicy.Monitoring); err != nil {
				t.Error(err)
			}
		}
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6), Schedulable: writeEnabledPointer(true), DesiredHealth: "healthy"},
	}, "scheduler")
	if err != nil || schedulingWrites.Load() != 0 || !result.RemoteWrite {
		t.Fatalf("recovery must stop before enabling traffic while retaining the first write outcome: %+v err=%v", result, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["schedulable"] != false || fixture.states["41"]["concurrency"] != 6 {
		t.Fatalf("revoked recovery changed traffic eligibility: %v", fixture.states["41"])
	}
}

func TestManualRestoreStopsWhenModeChangesDuringRemoteRead(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	var restoring atomic.Bool
	var restoreWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if restoring.Load() {
			if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41" {
				if _, err := fixture.store.SetMode(request.Context(), runtimepolicy.Monitoring); err != nil {
					t.Error(err)
				}
			}
			if request.Method != http.MethodGet {
				restoreWrites.Add(1)
			}
		}
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	initial, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)},
	}, "scheduler")
	if err != nil || initial.Failed != 0 || initial.Changed != 1 {
		t.Fatalf("initial ownership failed: %+v err=%v", initial, err)
	}
	restoring.Store(true)
	result, err := service.RestoreControl(t.Context(), "operator")
	if err != nil || result.RemoteWrite || restoreWrites.Load() != 0 {
		t.Fatalf("manual restore wrote after switching to monitoring: %+v writes=%d err=%v", result, restoreWrites.Load(), err)
	}
	baselines, err := fixture.store.RoutingBaselines(t.Context())
	if err != nil || len(baselines) != 1 {
		t.Fatalf("interrupted restore must retain its baseline: %+v err=%v", baselines, err)
	}
}

type cleanupAuthorizationStore struct {
	*business.Store
}

func (store *cleanupAuthorizationStore) RecordRuntimeEvent(ctx context.Context, eventType, state, summary string, payload map[string]any) (int64, error) {
	if eventType == "cleanup_delete_pending" {
		if _, err := store.SetMode(ctx, runtimepolicy.Monitoring); err != nil {
			return 0, err
		}
	}
	return store.Store.RecordRuntimeEvent(ctx, eventType, state, summary, payload)
}

func TestCleanupRevokedImmediatelyBeforeSendingIsSkippedWithoutClaimingWrite(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "already paused", true: "needs predisable"}[active], func(t *testing.T) {
			_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, active)
			var mutations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet {
					mutations.Add(1)
				}
				fixture.ServeHTTP(w, request)
			}))
			t.Cleanup(server.Close)
			service := routingwrite.New(routingTarget{url: server.URL}, &cleanupAuthorizationStore{Store: fixture.store})
			action := "delete"
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, CleanupAction: &action},
			}, "scheduler")
			if err != nil || result.Failed != 0 || result.RemoteWrite || mutations.Load() != 0 || len(result.Results) != 1 || !result.Results[0].Skipped {
				t.Fatalf("unsent cleanup must be deferred without a failure or write: %+v writes=%d err=%v", result, mutations.Load(), err)
			}
		})
	}
}
