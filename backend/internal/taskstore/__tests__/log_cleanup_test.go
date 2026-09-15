package taskstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

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
