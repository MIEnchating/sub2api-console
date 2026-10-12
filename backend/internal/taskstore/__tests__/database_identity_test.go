package taskstore_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDatabaseContractTaskUpdateKeepsOriginalOperationReservation(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			task := taskstore.Task{ID: "identity", Skill: "contract", Operation: "account-rate-sync", Status: "running", Message: "运行中", Result: map[string]any{}, CreatedAt: "2026-10-12T12:00:00Z", UpdatedAt: "2026-10-12T12:00:00Z"}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			task.Operation = "upstream-sync"
			task.Skill = "changed"
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			got, err := store.Get(t.Context(), task.ID)
			if err != nil || got.Operation != "account-rate-sync" || got.Skill != "contract" {
				t.Fatalf("immutable identity: %+v %v", got, err)
			}
			task.ID = "different-operation"
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatalf("unrelated operation was reserved by update: %v", err)
			}
		})
	}
}
