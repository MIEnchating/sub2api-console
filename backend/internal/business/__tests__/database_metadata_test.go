package business_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractConcurrentModelMetadataUpdatesRetainBothLists(t *testing.T) {
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
			if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "881", "name": "model-concurrency"}}, nil, "contract"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := store.SyncCompleteManagementSnapshot(t.Context(), nil, nil, "contract"); err != nil {
					t.Error(err)
				}
			}()
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for index := range 8 {
				model := fmt.Sprintf("model-%d", index)
				start, results := make(chan struct{}), make(chan error, 2)
				go func() { <-start; results <- store.SaveAccountModels(t.Context(), "881", []string{model}) }()
				go func() { <-start; results <- store.SaveAccountEnabledModels(t.Context(), "881", []string{model}) }()
				close(start)
				for range 2 {
					if err := <-results; err != nil {
						t.Fatal(err)
					}
				}
				var raw string
				if err := db.QueryRowContext(t.Context(), `SELECT metadata_json FROM accounts WHERE id='881'`).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var metadata map[string]json.RawMessage
				if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"known_models", "enabled_models"} {
					var values []string
					if err := json.Unmarshal(metadata[key], &values); err != nil || len(values) != 1 || values[0] != model {
						t.Fatalf("concurrent metadata update lost %s: %s", key, metadata[key])
					}
				}
			}
		})
	}
}
