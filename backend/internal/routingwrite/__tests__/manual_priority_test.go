package routingwrite_test

import (
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"testing"
	"time"
)

func TestManualPriorityOrderingWritesOnlyPriorityAndRetainsReleaseBaseline(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
	prepareManualOrder(t, fixture)
	result, err := service.ReorderManualPriorities(t.Context(), "test")
	if err != nil || result.Changed != 2 || result.Failed != 0 {
		t.Fatalf("reorder failed: %+v %v", result, err)
	}
	for _, id := range []string{"41", "42"} {
		want := int64(3)
		concurrency := 20
		if id == "42" {
			want = 1
			concurrency = 30
		}
		account, err := fixture.store.Account(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if account.Priority == nil || *account.Priority != want || account.Concurrency == nil || *account.Concurrency != int64(concurrency) || account.LoadFactor == nil || *account.LoadFactor != "10" || account.Schedulable == nil || !*account.Schedulable {
			t.Fatalf("other fields changed: %+v", account)
		}
		release, err := fixture.store.ManualPriorityRelease(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if release.Priority != 1000 {
			t.Fatalf("release baseline changed: %+v", release)
		}
	}
	again, err := service.ReorderManualPriorities(t.Context(), "test")
	if err != nil || again.Changed != 0 {
		t.Fatalf("stable order rewrote accounts: %+v %v", again, err)
	}
}
func TestManualPriorityOrderingDoesNotWriteWhenDisabledOrMonitoring(t *testing.T) {
	for _, scenario := range []string{"disabled", "monitoring"} {
		t.Run(scenario, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
			prepareManualOrder(t, fixture)
			if scenario == "disabled" {
				_, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"manual_priority": map[string]any{"latency_priority_enabled": false}}}, "test")
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := fixture.store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
					t.Fatal(err)
				}
			}
			result, err := service.ReorderManualPriorities(t.Context(), "test")
			if err != nil || result.RemoteWrite || result.Changed != 0 {
				t.Fatalf("unauthorized write: %+v %v", result, err)
			}
		})
	}
}
func TestManualPriorityOrderingStopsOnUnconfirmedReadback(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
	prepareManualOrder(t, fixture)
	fixture.ignoreParameters = true
	result, err := service.ReorderManualPriorities(t.Context(), "test")
	if err != nil || result.Failed != 1 || result.Changed != 0 || len(result.Results) != 1 {
		t.Fatalf("readback was not enforced: %+v %v", result, err)
	}
	account, err := fixture.store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if *account.Priority != 1 {
		t.Fatal("unconfirmed write changed local priority")
	}
}
func prepareManualOrder(t *testing.T, fixture *upstreamCapacityFixture) {
	t.Helper()
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"manual_priority": map[string]any{"latency_priority_enabled": true}, "weights": map[string]any{"performance_min_samples": 2},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		priority := int64(1)
		latency := "2000"
		if id == "42" {
			priority = 3
			latency = "500"
		}
		if _, err := fixture.store.AssignManualPriority(t.Context(), id, priority, "10", 20, true, "test"); err != nil {
			t.Fatal(err)
		}
		fixture.states[id]["priority"] = priority
		for i := range 2 {
			_, err := fixture.store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: id, GroupName: "capacity-group", Result: "通过", LatencyP95: &latency, SampleCount: 1, Attempts: 1, ObservedAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), EvidenceKey: fmt.Sprintf("%s-%d", id, i), Payload: map[string]any{"model": "same-model", "latency_metric": "first_token", "first_token_ms": latency}}})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{{"id": "7", "name": "capacity-group"}}, "test"); err != nil {
		t.Fatal(err)
	}
}
