package routingwrite_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
)

type manualCostChangingLeaseStore struct {
	*business.Store
	beforeAcquire func(context.Context) error
}

func (store *manualCostChangingLeaseStore) AcquireMutationLease(ctx context.Context, owner string, resources []string, now time.Time, ttl time.Duration) (bool, error) {
	if store.beforeAcquire != nil {
		change := store.beforeAcquire
		store.beforeAcquire = nil
		if err := change(ctx); err != nil {
			return false, err
		}
	}
	return store.Store.AcquireMutationLease(ctx, owner, resources, now, ttl)
}

func TestManualCostWallRecalculatesCurrentDataAfterWaitingForLease(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		change      func(*upstreamCapacityFixture)
		wall        string
		wantChanged int
	}{
		{"unrelated account renamed still enforces cost wall", func(f *upstreamCapacityFixture) { f.states["42"]["name"] = "renamed" }, "0.25", 1},
		{"account becomes profitable and stays enabled", func(f *upstreamCapacityFixture) { f.states["41"]["rate_multiplier"] = "0.1" }, "0.25", 0},
		{"group price rises and account stays enabled", func(*upstreamCapacityFixture) {}, "0.5", 0},
		{"manual pause is preserved", func(f *upstreamCapacityFixture) { f.states["41"]["schedulable"] = false }, "0.25", 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
			prepareManualCost(t, fixture)
			repository := &manualCostChangingLeaseStore{Store: fixture.store, beforeAcquire: func(ctx context.Context) error {
				scenario.change(fixture)
				_, err := fixture.store.SyncManagementSnapshot(ctx, fixture.accounts(), []map[string]any{
					{"id": "7", "name": "capacity-group", "rate_multiplier": scenario.wall},
				}, "test")
				return err
			}}
			result, err := routingwrite.New(routingTarget{url: fixture.serverURL}, repository).EnforceManualCostWall(t.Context(), []string{"41"}, "test")
			if err != nil || result.Failed != 0 || result.Changed != scenario.wantChanged {
				t.Fatalf("latest data was not recalculated: result=%+v err=%v", result, err)
			}
			if scenario.wantChanged == 0 && result.RemoteWrite {
				t.Fatalf("fresh snapshot should prevent a stale write: %+v", result)
			}
		})
	}
}

func TestManualCostWallReacquiresExpandedScopeAndPreservesExplicitSelection(t *testing.T) {
	for _, selectedOnly := range []bool{false, true} {
		name := "new manual account is locked before recalculation"
		if selectedOnly {
			name = "new manual account outside selection is not written"
		}
		t.Run(name, func(t *testing.T) {
			_, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
			prepareManualCost(t, fixture)
			repository := &manualCostChangingLeaseStore{Store: fixture.store, beforeAcquire: func(ctx context.Context) error {
				fixture.states["42"]["rate_multiplier"] = "0.32"
				fixture.states["42"]["group_ids"] = []any{"7"}
				fixture.states["42"]["priority"] = 2
				if _, err := fixture.store.SyncManagementSnapshot(ctx, fixture.accounts(), []map[string]any{
					{"id": "7", "name": "capacity-group", "rate_multiplier": "0.25"},
				}, "test"); err != nil {
					return err
				}
				_, err := fixture.store.AssignManualPriority(ctx, "42", 2, "10", 30, true, "test")
				return err
			}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					locked, err := fixture.store.AcquireMutationLease(r.Context(), "competing-cost-wall", []string{mutationguard.Account("42")}, time.Now(), time.Minute)
					if err != nil {
						t.Error(err)
					}
					if locked {
						_ = fixture.store.ReleaseMutationLease(r.Context(), "competing-cost-wall", []string{mutationguard.Account("42")})
					}
					if !selectedOnly && locked {
						t.Error("expanded cost-wall scope was written without locking the new account")
					}
				}
				fixture.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			var ids []string
			wantChanged := 2
			if selectedOnly {
				ids, wantChanged = []string{"41"}, 1
			}
			result, err := routingwrite.New(routingTarget{url: server.URL}, repository).EnforceManualCostWall(t.Context(), ids, "test")
			if err != nil || result.Failed != 0 || result.Changed != wantChanged {
				t.Fatalf("scope was not safely recalculated: result=%+v err=%v", result, err)
			}
			if selectedOnly && fixture.states["42"]["schedulable"] != true {
				t.Fatal("unselected manual account was changed")
			}
		})
	}
}

func TestManualCostWallStopsWithoutWritingWhenScopeKeepsExpanding(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, nil, 20, 30, true)
	prepareManualCost(t, fixture)
	repository := &manualCostChangingLeaseStore{Store: fixture.store}
	nextID := 42
	var addManualAccount func(context.Context) error
	addManualAccount = func(ctx context.Context) error {
		if nextID > 44 {
			return nil
		}
		id := strconv.Itoa(nextID)
		row := map[string]any{}
		for key, value := range fixture.states["41"] {
			row[key] = value
		}
		row["id"], row["name"], row["priority"] = id, "new-manual-"+id, nextID-40
		fixture.states[id] = row
		if _, err := fixture.store.SyncManagementSnapshot(ctx, fixture.accounts(), []map[string]any{
			{"id": "7", "name": "capacity-group", "rate_multiplier": "0.25"},
		}, "test"); err != nil {
			return err
		}
		if _, err := fixture.store.AssignManualPriority(ctx, id, int64(nextID-40), "10", 20, true, "test"); err != nil {
			return err
		}
		nextID++
		repository.beforeAcquire = addManualAccount
		return nil
	}
	repository.beforeAcquire = addManualAccount
	result, err := routingwrite.New(routingTarget{url: fixture.serverURL}, repository).EnforceManualCostWall(t.Context(), nil, "test")
	if err == nil || !strings.Contains(err.Error(), "范围持续变化") || result.RemoteWrite {
		t.Fatalf("unstable scope must stop before writing: result=%+v err=%v", result, err)
	}
	resources := []string{mutationguard.AccountCatalog(), mutationguard.Account("41"), mutationguard.ManagementTarget()}
	acquired, err := fixture.store.AcquireMutationLease(t.Context(), "after-aborted-cost-wall", resources, time.Now(), time.Minute)
	if err != nil || !acquired {
		t.Fatalf("aborted recalculation retained its leases: acquired=%v err=%v", acquired, err)
	}
	if err := fixture.store.ReleaseMutationLease(t.Context(), "after-aborted-cost-wall", resources); err != nil {
		t.Fatal(err)
	}
}
