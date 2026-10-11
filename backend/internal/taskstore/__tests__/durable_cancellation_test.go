package taskstore_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDurableCancellationFailureRollsBackStateCheckpointAndReservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "active", Skill: "fixture", Operation: "account-rate-sync", Status: "queued", Progress: 30, Message: "待继续", Result: map[string]any{"completed": "one"}, CreatedAt: stamp, UpdatedAt: stamp}
	if err := taskstore.WithRecovery(&task, []string{"1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_checkpoint_delete BEFORE DELETE ON task_recovery BEGIN SELECT RAISE(ABORT,'fixture cancellation failure'); END`); err != nil {
		t.Fatal(err)
	}
	if cancelled, err := store.CancelTask(t.Context(), task.ID); err == nil || cancelled {
		t.Fatalf("failure accepted cancellation: %v %v", cancelled, err)
	}
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || !reflect.DeepEqual(got, before) {
		t.Fatalf("failed cancellation changed task: %#v %v", got, err)
	}
	if _, err := store.RecoveryTask(t.Context(), task.ID); err != nil {
		t.Fatalf("checkpoint lost: %v", err)
	}
	other := task
	other.ID = "other"
	if err := store.Save(t.Context(), other); !errors.Is(err, taskstore.ErrOperationActive) {
		t.Fatalf("failed cancellation released reservation: %v", err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_checkpoint_delete`); err != nil {
		t.Fatal(err)
	}
	if cancelled, err := store.CancelTask(t.Context(), task.ID); err != nil || !cancelled {
		t.Fatalf("retry cancellation: %v %v", cancelled, err)
	}
	if _, err := store.RecoveryTask(t.Context(), task.ID); !errors.Is(err, taskstore.ErrNotFound) {
		t.Fatalf("cancelled checkpoint still present: %v", err)
	}
	if err := store.Save(t.Context(), other); err != nil {
		t.Fatalf("successful cancellation retained reservation: %v", err)
	}
	if err := store.Save(t.Context(), task); !errors.Is(err, taskstore.ErrOperationActive) && !errors.Is(err, taskstore.ErrTaskTerminal) {
		t.Fatalf("late active write resurrected task: %v", err)
	}
}

func TestDurableCancellationLeavesTerminalAndMissingTasksUnchanged(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	for _, status := range []string{"succeeded", "failed", "partial", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			task := taskstore.Task{ID: status, Skill: "fixture", Operation: "fixture", Status: status, Message: "已有结果", Result: map[string]any{"completed": "one"}, CreatedAt: stamp, UpdatedAt: stamp}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			before, err := store.Get(t.Context(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if cancelled, err := store.CancelTask(t.Context(), task.ID); err != nil || cancelled {
				t.Fatalf("terminal cancellation: %v %v", cancelled, err)
			}
			got, err := store.Get(t.Context(), task.ID)
			if err != nil || !reflect.DeepEqual(got, before) {
				t.Fatalf("terminal result changed: %#v %v", got, err)
			}
		})
	}
	if cancelled, err := store.CancelTask(t.Context(), "missing"); err != nil || cancelled {
		t.Fatalf("missing cancellation: %v %v", cancelled, err)
	}
}
