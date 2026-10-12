package business_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractConcurrentManualPriorityHonorsGroupCapacity(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			accounts := []map[string]any{}
			for i := range 8 {
				accounts = append(accounts, map[string]any{"id": fmt.Sprint(870 + i), "name": fmt.Sprint("manual-", i), "group_ids": []any{"970"}})
			}
			if _, err := store.SyncManagementSnapshot(t.Context(), accounts, []map[string]any{{"id": "970", "name": "manual-group"}}, "contract"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := store.SyncCompleteManagementSnapshot(t.Context(), nil, nil, "contract"); err != nil {
					t.Error(err)
				}
			}()
			start, results := make(chan struct{}), make(chan error, len(accounts))
			for i := range accounts {
				go func() {
					<-start
					_, err := store.AssignManualPriority(t.Context(), fmt.Sprint(870+i), 1, "100", 100, false, "contract")
					results <- err
				}()
			}
			close(start)
			winners := 0
			for range accounts {
				err := <-results
				if err == nil {
					winners++
				} else if !strings.Contains(err.Error(), "最多 3 个账号") {
					t.Error(err)
				}
			}
			if winners != 3 {
				t.Fatalf("manual slot admitted %d accounts, want 3", winners)
			}
		})
	}
}
