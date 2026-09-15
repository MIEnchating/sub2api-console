package onboarding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

type observedCreationLeaseStore struct {
	*business.Store
	attempted chan bool
	once      sync.Once
}

func (store *observedCreationLeaseStore) AcquireMutationLease(ctx context.Context, ownerID string, resources []string, now time.Time, ttl time.Duration) (bool, error) {
	acquired, err := store.Store.AcquireMutationLease(ctx, ownerID, resources, now, ttl)
	store.once.Do(func() { store.attempted <- acquired })
	return acquired, err
}

func TestNewAccountCreationWaitsUntilRoutingCapacityReservationIsReleased(t *testing.T) {
	for _, schedulable := range []bool{false, true} {
		name := "paused creation reserves configured capacity"
		if schedulable {
			name = "active creation adds schedulable capacity"
		}
		t.Run(name, func(t *testing.T) {
			var creates atomic.Int32
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if strings.HasSuffix(request.URL.Path, "/models/sync-upstream-preview") {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"models": []string{"gemini-2.5-flash"}}})
					return
				}
				if request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts" {
					var account map[string]any
					decoder := json.NewDecoder(request.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&account); err != nil {
						t.Error(err)
						return
					}
					creates.Add(1)
					account["id"] = json.Number("77")
					_ = json.NewEncoder(w).Encode(map[string]any{"data": account})
					return
				}
				http.NotFound(w, request)
			}))
			t.Cleanup(admin.Close)
			attempted := make(chan bool, 1)
			service, repository, _, request := newService(t, admin.URL, admin.URL, func(store *business.Store) onboarding.Repository {
				return &observedCreationLeaseStore{Store: store, attempted: attempted}
			})
			request.Schedulable = schedulable
			_, release, err := mutationguard.Acquire(t.Context(), repository, "routing-global-concurrency")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = release() }()
			completed := make(chan error, 1)
			go func() {
				_, err := service.Onboard(t.Context(), request)
				completed <- err
			}()
			if acquired := <-attempted; acquired {
				t.Error("new account creation acquired its lease while routing capacity was still reserved")
			}
			if count := creates.Load(); count != 0 {
				t.Errorf("created %d accounts before the capacity reservation was released", count)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			if err := <-completed; err != nil {
				t.Fatal(err)
			}
			if count := creates.Load(); count != 1 {
				t.Fatalf("expected one account after releasing capacity, got %d", count)
			}
		})
	}
}
