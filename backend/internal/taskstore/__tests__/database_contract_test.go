package taskstore_test

import (
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"testing"
	"time"
)

func TestDatabaseContractTaskTerminalAndOperationProtection(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			now := time.Date(2026, 10, 12, 12, 0, 0, 123456789, time.UTC).Format(time.RFC3339Nano)
			task := taskstore.Task{ID: "contract-task", Skill: "contract", Operation: "account-rate-sync", Status: "running", Message: "运行中", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			other := task
			other.ID = "contract-other"
			if err := store.Save(t.Context(), other); !errors.Is(err, taskstore.ErrOperationActive) {
				t.Fatalf("operation overlap: %v", err)
			}
			task.Status = "succeeded"
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			task.Status = "running"
			if err := store.Save(t.Context(), task); !errors.Is(err, taskstore.ErrTaskTerminal) {
				t.Fatalf("terminal overwritten: %v", err)
			}
			if err := store.Save(t.Context(), other); err != nil {
				t.Fatal(err)
			}
			got, err := store.Get(t.Context(), task.ID)
			if err != nil || got.Status != "succeeded" || got.UpdatedAt != now {
				t.Fatalf("persisted task: %+v %v", got, err)
			}
			other.Result["exact"] = json.Number("1.234567890123456789")
			if err := taskstore.WithRecovery(&other, []string{"account-1"}); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(t.Context(), other); err != nil {
				t.Fatal(err)
			}
			pending, err := store.PendingRecovery(t.Context())
			if err != nil || len(pending) != 1 {
				t.Fatalf("recovery queue: %v %v", pending, err)
			}
			recovered, err := store.RecoveryTask(t.Context(), other.ID)
			if err != nil || recovered.Result["exact"] != json.Number("1.234567890123456789") || recovered.Result["interrupted"] != true {
				t.Fatalf("recovery precision: %+v %v", recovered, err)
			}
			cancelled, err := store.CancelRecovery(t.Context(), other.ID)
			if err != nil || !cancelled {
				t.Fatalf("cancel: %v %v", cancelled, err)
			}
			pending, err = store.PendingRecovery(t.Context())
			if err != nil || len(pending) != 0 {
				t.Fatalf("cancelled task recovered: %v %v", pending, err)
			}
		})
	}
}
