package business_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractOverlappingLeaseClaimsHaveOneWinner(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			now := time.Date(2026, 10, 12, 0, 0, 0, 123456789, time.UTC)
			const clients = 8
			start := make(chan struct{})
			winners := make(chan string, clients)
			var workers sync.WaitGroup
			for i := 0; i < clients; i++ {
				workers.Go(func() {
					<-start
					owner := fmt.Sprintf("claim-%d", i)
					resources := []string{"contract/first", "contract/second"}
					if i%2 == 0 {
						resources[0], resources[1] = resources[1], resources[0]
					}
					ok, err := store.AcquireMutationLease(t.Context(), owner, resources, now, time.Minute)
					if err != nil {
						t.Errorf("claim: %v", err)
					}
					if ok {
						winners <- owner
					}
				})
			}
			close(start)
			workers.Wait()
			close(winners)
			if len(winners) != 1 {
				t.Fatalf("winners=%d", len(winners))
			}
			owner := <-winners
			defer store.ReleaseMutationLease(t.Context(), owner, []string{"contract/first", "contract/second"})
			if ok, err := store.RenewMutationLease(t.Context(), owner, []string{"contract/first", "contract/second"}, now.Add(time.Minute-time.Nanosecond), time.Minute); err != nil || !ok {
				t.Fatalf("renew one nanosecond before expiry: %v %v", ok, err)
			}
			if ok, err := store.RenewMutationLease(t.Context(), "stale", []string{"contract/first"}, now, time.Minute); err != nil || ok {
				t.Fatalf("stale owner renewed: %v %v", ok, err)
			}
		})
	}
}

func TestDatabaseContractExpiredInspectionLeaseCannotRenew(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			defer store.ReleaseInspectionLease(t.Context(), "contract-inspection")
			now := time.Date(2026, 10, 12, 0, 0, 0, 123456789, time.UTC)
			if ok, err := store.AcquireInspectionLease(t.Context(), "contract-inspection", 1, "isolated.example", now, now, time.Minute); err != nil || !ok {
				t.Fatalf("acquire: %v %v", ok, err)
			}
			if ok, err := store.RenewInspectionLease(t.Context(), "contract-inspection", now.Add(time.Minute), time.Minute); err != nil || ok {
				t.Fatalf("expired owner renewed: %v %v", ok, err)
			}
		})
	}
}
