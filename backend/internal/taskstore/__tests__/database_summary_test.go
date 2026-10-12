package taskstore_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"testing"
)

func TestDatabaseContractTaskSummariesSearchPublicMetadata(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			task := taskstore.Task{ID: "summary", Skill: "contract", Operation: "active-probe", Status: "succeeded", Message: "完成", CreatedAt: "2026-10-12T12:00:00Z", UpdatedAt: "2026-10-12T12:00:00Z", Result: map[string]any{"platform": "openai", "model": "fixture-model", "account_id": "account-41", "private": "not-searchable"}}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			rows, err := store.ListConsoleSummaries(t.Context(), nil)
			if err != nil || len(rows) != 1 || rows[0].Result["object_label"] != "account-41" {
				t.Fatalf("console projection: %v %v", rows, err)
			}
			rows, err = store.SearchLogs(t.Context(), "FIXTURE-MODEL", nil)
			if err != nil || len(rows) != 1 || rows[0].Result["model"] != "fixture-model" {
				t.Fatalf("model search: %v %v", rows, err)
			}
			rows, err = store.SearchLogs(t.Context(), "not-searchable", nil)
			if err != nil || len(rows) != 0 {
				t.Fatalf("private field searched: %v %v", rows, err)
			}
		})
	}
}
