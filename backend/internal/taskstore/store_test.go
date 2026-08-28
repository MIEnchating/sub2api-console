package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskHistoryRecoveryAndStrictJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for index := 0; index < 1100; index++ {
		status := "succeeded"
		if index == 0 {
			status = "running"
		}
		if err := store.Save(ctx, Task{ID: stringID(index), Skill: "console", Operation: "inspect", Status: status, Progress: 50, Message: "运行", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := store.List(ctx, nil)
	if err != nil || len(tasks) != 1100 {
		t.Fatalf("history len=%d err=%v", len(tasks), err)
	}
	if recovered, err := store.RecoverInterrupted(ctx); err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	task, err := store.Get(ctx, "task-0")
	if err != nil || task.Status != "failed" || task.Result["interrupted"] != true {
		t.Fatalf("unexpected recovered task: %#v err=%v", task, err)
	}
	invalid := Task{ID: "invalid", Skill: "console", Operation: "inspect", Status: "failed", Progress: 100, Message: "failed", Result: map[string]any{"sentinel": math.Inf(1)}, CreatedAt: now, UpdatedAt: now}
	if err := store.Save(ctx, invalid); err == nil {
		t.Fatal("non-finite result must be rejected")
	}
}

func TestCorruptStoredResultProjectsFailedTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().UTC().Format(time.RFC3339Nano)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO tasks VALUES('broken','console','inspect','succeeded',100,'done','[]',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	task, err := store.Get(context.Background(), "broken")
	if err != nil || task.Status != "failed" || task.Result["storage_corrupt"] != true {
		t.Fatalf("unexpected corrupt projection: %#v err=%v", task, err)
	}
}

func TestClearLogsDeletesOnlyExpiredTerminalTasks(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	old := "2026-07-01T00:00:00Z"
	recent := "2026-08-26T00:00:00Z"
	for _, task := range []Task{
		{ID: "old", Skill: "console", Operation: "sync", Status: "succeeded", Progress: 100, Message: "done", Result: map[string]any{}, CreatedAt: old, UpdatedAt: old},
		{ID: "recent", Skill: "console", Operation: "sync", Status: "failed", Progress: 100, Message: "failed", Result: map[string]any{}, CreatedAt: recent, UpdatedAt: recent},
		{ID: "active", Skill: "console", Operation: "sync", Status: "running", Progress: 50, Message: "running", Result: map[string]any{}, CreatedAt: old, UpdatedAt: old},
	} {
		if err := store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	deleted, protected, err := store.ClearLogs(ctx, &cutoff)
	if err != nil || deleted != 1 || protected != 1 {
		t.Fatalf("deleted=%d protected=%d err=%v", deleted, protected, err)
	}
	rows, err := store.List(ctx, nil)
	if err != nil || len(rows) != 2 || rows[0].ID != "recent" || rows[1].ID != "active" {
		t.Fatalf("unexpected remaining tasks: %#v err=%v", rows, err)
	}
}

func stringID(index int) string {
	return "task-" + fmt.Sprint(index)
}
