package taskstore_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestRecoveryPreparationPreservesCheckpointAndExactCompletedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	task := taskstore.Task{ID: "recover", Skill: "test", Operation: "account-rate-sync", Status: "running", Progress: 40, Message: "处理中", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{
		"request_id": json.Number("9007199254740993"), "amount": json.Number("1.234567890123456789"), "completed": []any{map[string]any{"account_id": "41", "status": "succeeded"}},
	}}
	if err := taskstore.WithRecovery(&task, []string{"41", "42"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const checkpoint = `{ "version": 1, "input": ["41", "42"] }`
	if _, err := db.Exec(`UPDATE task_recovery SET payload=? WHERE task_id='recover'; DELETE FROM active_task_operations`, checkpoint); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingRecovery(t.Context())
	if err != nil || len(pending) != 1 || pending[0].ID != task.ID {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	stored, err := store.RecoveryTask(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "queued" || stored.Progress != 40 || stored.Result["interrupted"] != true || stored.Result["request_id"] != json.Number("9007199254740993") || stored.Result["amount"] != json.Number("1.234567890123456789") {
		t.Fatalf("preparation changed completed values: %+v", stored)
	}
	completed := stored.Result["completed"].([]any)
	if len(completed) != 1 || completed[0].(map[string]any)["account_id"] != "41" || completed[0].(map[string]any)["status"] != "succeeded" {
		t.Fatalf("completed account lost: %v", completed)
	}
	updated, err := time.Parse(time.RFC3339Nano, stored.UpdatedAt)
	if err != nil || !updated.After(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("preparation did not advance timestamp: %s, %v", stored.UpdatedAt, err)
	}
	var raw string
	if err := db.QueryRow(`SELECT payload FROM task_recovery WHERE task_id='recover'`).Scan(&raw); err != nil || raw != checkpoint {
		t.Fatalf("preparation rewrote checkpoint: %q, %v", raw, err)
	}
	task.ID = "competing"
	if err := store.Save(t.Context(), task); !errors.Is(err, taskstore.ErrOperationActive) {
		t.Fatalf("recovered task lost operation exclusion: %v", err)
	}
}
