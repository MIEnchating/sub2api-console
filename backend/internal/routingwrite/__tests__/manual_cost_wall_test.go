package routingwrite_test

import (
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"net/http"
	"net/http/httptest"
	"testing"
)

func prepareManualCost(t *testing.T, fixture *upstreamCapacityFixture) {
	t.Helper()
	fixture.states["41"]["rate_multiplier"] = "0.32"
	fixture.states["41"]["group_ids"] = []any{"7"}
	fixture.states["41"]["priority"] = 1
	if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{{"id": "7", "name": "capacity-group", "rate_multiplier": "0.25"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.AssignManualPriority(t.Context(), "41", 1, "10", 20, true, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestManualCostWallWritesOnlySchedulingAndRecoversAfterExplicitExemption(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
	prepareManualCost(t, fixture)
	result, err := service.EnforceManualCostWall(t.Context(), []string{"41"}, "test")
	if err != nil || result.Changed != 1 || result.Failed != 0 {
		t.Fatalf("cost write: %+v %v", result, err)
	}
	account, err := fixture.store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Health != "cost_blocked" || account.Schedulable == nil || *account.Schedulable || *account.Priority != 1 || *account.LoadFactor != "10" || *account.Concurrency != 20 || account.ManualPriority == nil {
		t.Fatalf("manual settings not preserved: %+v", account)
	}
	again, err := service.EnforceManualCostWall(t.Context(), []string{"41"}, "test")
	if err != nil || again.RemoteWrite {
		t.Fatalf("repeated confirmed pause: %+v %v", again, err)
	}
	if err := fixture.store.SetAccountIgnoreCostWall(t.Context(), "41", true, "test"); err != nil {
		t.Fatal(err)
	}
	restored, err := service.EnforceManualCostWall(t.Context(), []string{"41"}, "test")
	if err != nil || restored.Changed != 1 || restored.Failed != 0 || fixture.states["41"]["schedulable"] != true {
		t.Fatalf("restore: %+v %v", restored, err)
	}
}

func TestManualCostWallRejectsStalePriceOrMembershipBeforeWriting(t *testing.T) {
	for _, field := range []string{"rate_multiplier", "group_ids"} {
		t.Run(field, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
			prepareManualCost(t, fixture)
			if field == "rate_multiplier" {
				fixture.states["41"][field] = "0.1"
			} else {
				fixture.states["41"][field] = []any{"8"}
			}
			result, err := service.EnforceManualCostWall(t.Context(), []string{"41"}, "test")
			if err != nil || result.Failed != 1 || result.RemoteWrite || fixture.states["41"]["schedulable"] != true {
				t.Fatalf("stale cost plan wrote: %+v %v", result, err)
			}
		})
	}
}

func TestManualCostWallPreservesMonitoringModeAndUnselectedAccounts(t *testing.T) {
	for _, scenario := range []string{"monitoring", "unselected"} {
		t.Run(scenario, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
			prepareManualCost(t, fixture)
			ids := []string{"42"}
			if scenario == "monitoring" {
				ids = nil
				if _, err := fixture.store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
					t.Fatal(err)
				}
			}
			result, err := service.EnforceManualCostWall(t.Context(), ids, "test")
			if err != nil || result.RemoteWrite || fixture.states["41"]["schedulable"] != true {
				t.Fatalf("unexpected write: %+v %v", result, err)
			}
		})
	}
}

func TestManualCostWallDoesNotCommitUnconfirmedSchedulingReadback(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
	prepareManualCost(t, fixture)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41" && fixture.states["41"]["schedulable"] == false {
			row := map[string]any{}
			for k, v := range fixture.states["41"] {
				row[k] = v
			}
			row["schedulable"] = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": row})
			return
		}
		fixture.ServeHTTP(w, r)
	}))
	defer server.Close()
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	result, err := service.EnforceManualCostWall(t.Context(), []string{"41"}, "test")
	if err != nil || result.Failed != 1 || result.Changed != 0 {
		t.Fatalf("readback not enforced: %+v %v", result, err)
	}
	account, err := fixture.store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Schedulable == nil || !*account.Schedulable || account.Health == "cost_blocked" {
		t.Fatalf("unconfirmed result committed: %+v", account)
	}
}
