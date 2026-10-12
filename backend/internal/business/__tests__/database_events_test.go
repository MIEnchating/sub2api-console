package business_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractConcurrentRuntimeEventsReturnTheirOwnID(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var workers sync.WaitGroup
			start := make(chan struct{})
			for i := 0; i < 12; i++ {
				workers.Go(func() {
					<-start
					want := fmt.Sprintf("concurrent-%d", i)
					id, err := store.RecordRuntimeEvent(t.Context(), "contract.concurrent", "succeeded", want, map[string]any{"index": i})
					if err != nil {
						t.Errorf("record: %v", err)
						return
					}
					var got string
					if err := db.QueryRowContext(t.Context(), `SELECT summary FROM runtime_events WHERE source_id=?`, id).Scan(&got); err != nil || got != want {
						t.Errorf("event %d returned another record: %q %v", id, got, err)
					}
				})
			}
			close(start)
			workers.Wait()
		})
	}
}
