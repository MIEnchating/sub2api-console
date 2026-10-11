package taskstore_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestLogCleanupRemovesRecoveryAndOperationBindingsWithDeletedTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stamp := "2026-01-01T00:00:00Z"
	for _, id := range []string{"expired", "active"} {
		status := "succeeded"
		if id == "active" {
			status = "running"
		}
		if err := store.Save(t.Context(), taskstore.Task{ID: id, Skill: "fixture", Operation: "fixture", Status: status, Message: "fixture", Result: map[string]any{}, CreatedAt: stamp, UpdatedAt: stamp}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO task_recovery(task_id,payload) VALUES(?,?)", id, "{}"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO active_task_operations(operation,task_id) VALUES(?,?)", id, id); err != nil {
			t.Fatal(err)
		}
	}
	deleted, protected, err := store.ClearLogs(t.Context(), nil)
	if err != nil || deleted != 1 || protected != 1 {
		t.Fatalf("cleanup = deleted %d, protected %d, error %v", deleted, protected, err)
	}
	for _, table := range []string{"tasks", "task_recovery", "active_task_operations"} {
		var id string
		var count int
		column := "id"
		if table != "tasks" {
			column = "task_id"
		}
		if err := db.QueryRow("SELECT COUNT(*),MIN("+column+") FROM "+table).Scan(&count, &id); err != nil || count != 1 || id != "active" {
			t.Fatalf("%s retained = %d rows, id %s, error %v", table, count, id, err)
		}
	}
	if err := store.Save(t.Context(), taskstore.Task{ID: "expired", Skill: "fixture", Operation: "fixture", Status: "succeeded", Message: "late", Result: map[string]any{}, CreatedAt: stamp, UpdatedAt: stamp}); !errors.Is(err, taskstore.ErrTaskDeleted) {
		t.Fatalf("deleted task revived: %v", err)
	}
}

func TestLogCleanupFailedBatchRollsBackAndCanRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stamp := "2026-01-01T00:00:00Z"
	task := taskstore.Task{ID: "expired", Skill: "fixture", Operation: "fixture", Status: "succeeded", Message: "fixture", Result: map[string]any{}, CreatedAt: stamp, UpdatedAt: stamp}
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO task_recovery(task_id,payload) VALUES('expired','{}')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TRIGGER fail_cleanup BEFORE DELETE ON tasks BEGIN SELECT RAISE(ABORT,'fixture failure'); END"); err != nil {
		t.Fatal(err)
	}
	if deleted, _, err := store.ClearLogs(t.Context(), nil); err == nil || deleted != 0 {
		t.Fatalf("failed batch = deleted %d, error %v", deleted, err)
	}
	for _, table := range []string{"tasks", "task_recovery"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s after rollback = %d, error %v", table, count, err)
		}
	}
	var tombstones int
	if err := db.QueryRow("SELECT COUNT(*) FROM deleted_tasks").Scan(&tombstones); err != nil || tombstones != 0 {
		t.Fatalf("tombstones after rollback = %d, error %v", tombstones, err)
	}
	if _, err := db.Exec("DROP TRIGGER fail_cleanup"); err != nil {
		t.Fatal(err)
	}
	if deleted, _, err := store.ClearLogs(t.Context(), nil); err != nil || deleted != 1 {
		t.Fatalf("retry = deleted %d, error %v", deleted, err)
	}
}

func TestLogCleanupCommitsBoundedBatchesBeforeReportingProgress(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stamp := "2026-01-01T00:00:00Z"
	for i := 0; i < 2001; i++ {
		task := taskstore.Task{ID: fmt.Sprintf("expired-%04d", i), Skill: "fixture", Operation: "fixture", Status: "succeeded", Message: "done", Result: map[string]any{}, CreatedAt: stamp, UpdatedAt: stamp}
		if err := store.Save(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	deleted, _, err := store.ClearLogsWithProgress(ctx, &cutoff, func(deleted, protected int64) error {
		if deleted != 2000 {
			t.Fatalf("first batch=%d", deleted)
		}
		if _, err := store.Get(t.Context(), "expired-0000"); !errors.Is(err, taskstore.ErrNotFound) {
			t.Fatalf("progress before commit: %v", err)
		}
		active := taskstore.Task{ID: "active", Skill: "fixture", Operation: "fixture", Status: "running", Message: "active", Result: map[string]any{}, CreatedAt: stamp, UpdatedAt: stamp}
		if err := store.Save(t.Context(), active); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || deleted != 2000 {
		t.Fatalf("deleted=%d error=%v", deleted, err)
	}
	deleted, protected, err := store.ClearLogs(t.Context(), &cutoff)
	if err != nil || deleted != 1 || protected != 1 {
		t.Fatalf("retry=%d protected=%d error=%v", deleted, protected, err)
	}
}

func TestLogCleanupRejectsLateFinalizationAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tasks.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	task := taskstore.Task{
		ID: "completed-oauth", Skill: "account-workbench", Operation: "account-workbench-oauth",
		Status: "succeeded", Progress: 100, Message: "授权完成", Result: map[string]any{},
		CreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano), UpdatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano),
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	deleted, _, err := store.ClearLogs(ctx, &now)
	if err != nil || deleted != 1 {
		t.Fatalf("cleanup = %d, %v; want one deleted task", deleted, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	task.Message = "短信订单核对完成"
	task.UpdatedAt = now.Format(time.RFC3339Nano)
	if err := store.Save(ctx, task); !errors.Is(err, taskstore.ErrTaskDeleted) {
		t.Fatalf("late finalization after log cleanup = %v; want ErrTaskDeleted", err)
	}
	if _, err := store.Get(ctx, task.ID); !errors.Is(err, taskstore.ErrNotFound) {
		t.Fatalf("cleaned history became visible again: %v", err)
	}
}

func TestLogCleanupPreservesUpdatesToActiveAndRetainedTasks(t *testing.T) {
	ctx := context.Background()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cutoff := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	for _, fixture := range []struct {
		id, status string
		updatedAt  time.Time
	}{
		{id: "expired", status: "failed", updatedAt: cutoff.Add(-time.Hour)},
		{id: "active", status: "running", updatedAt: cutoff.Add(-time.Hour)},
		{id: "retained", status: "succeeded", updatedAt: cutoff},
	} {
		stamp := fixture.updatedAt.Format(time.RFC3339Nano)
		if err := store.Save(ctx, taskstore.Task{
			ID: fixture.id, Skill: "inspection", Operation: "inspection", Status: fixture.status,
			Message: "任务状态", Result: map[string]any{}, CreatedAt: stamp, UpdatedAt: stamp,
		}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, protected, err := store.ClearLogs(ctx, &cutoff)
	if err != nil || deleted != 1 || protected != 1 {
		t.Fatalf("cleanup = deleted %d, protected %d, error %v", deleted, protected, err)
	}
	for _, id := range []string{"active", "retained"} {
		task, err := store.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		task.Status, task.Progress, task.Message = "succeeded", 100, "已完成核对"
		task.UpdatedAt = cutoff.Add(time.Minute).Format(time.RFC3339Nano)
		if err := store.Save(ctx, task); err != nil {
			t.Fatalf("cleanup blocked retained task %s: %v", id, err)
		}
	}
}
