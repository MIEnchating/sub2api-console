package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
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

func TestTaskLogSummaryAndSearchAvoidLoadingUnmatchedResults(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, task := range []Task{
		{ID: "task-1", Skill: "console", Operation: "inspect", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{"run_key": "run-1", "account_name": "账号一", "request_id": "request-target", "large": strings.Repeat("x", 4096)}, CreatedAt: now, UpdatedAt: now},
		{ID: "task-2", Skill: "console", Operation: "inspect", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{"request_id": "request-other", "large": strings.Repeat("y", 4096)}, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	limit := 20
	summaries, err := store.ListLogSummaries(ctx, &limit)
	if err != nil || len(summaries) != 2 {
		t.Fatalf("summaries=%#v err=%v", summaries, err)
	}
	if summaries[0].Result["large"] != nil || summaries[0].Result["request_id"] != nil {
		t.Fatalf("log summary loaded full task result: %#v", summaries[0].Result)
	}
	if summaries[0].Result["run_key"] != "run-1" || summaries[0].Result["object_label"] != "账号一" {
		t.Fatalf("log linking metadata missing: %#v", summaries[0].Result)
	}
	matched, err := store.SearchLogs(ctx, "REQUEST-TARGET", &limit)
	if err != nil || len(matched) != 1 || matched[0].ID != "task-1" || matched[0].Result["request_id"] != "request-target" {
		t.Fatalf("matched=%#v err=%v", matched, err)
	}
}

func TestTaskLogSummaryPreservesExplicitObjectLabel(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.Save(ctx, Task{
		ID: "task-object", Skill: "console", Operation: "cleanup", Status: "succeeded",
		Progress: 100, Message: "完成", Result: map[string]any{"object_label": "未绑定 Key"},
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListLogSummaries(ctx, nil)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("summaries=%#v err=%v", summaries, err)
	}
	if summaries[0].Result["object_label"] != "未绑定 Key" {
		t.Fatalf("explicit object label missing: %#v", summaries[0].Result)
	}
}

func TestListBySkillOnlyLoadsMatchingTaskResults(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, task := range []Task{
		{ID: "model-check", Skill: "sub2api-model-check", Operation: "check", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{"account_ids": []string{"41"}}, CreatedAt: "2026-08-31T01:00:00Z", UpdatedAt: "2026-08-31T02:00:00Z"},
		{ID: "inspection", Skill: "sub2api-inspection", Operation: "inspect", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{"account_id": "42"}, CreatedAt: "2026-08-31T01:00:00Z", UpdatedAt: "2026-08-31T03:00:00Z"},
	} {
		if err := store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.ListBySkill(ctx, "sub2api-model-check")
	if err != nil || len(rows) != 1 || rows[0].ID != "model-check" {
		t.Fatalf("unexpected skill tasks: %#v err=%v", rows, err)
	}
	if _, err := store.ListBySkill(ctx, " "); err == nil {
		t.Fatal("empty skill must be rejected")
	}
}

func TestLatestByOperationReturnsNewestMatchingStatus(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, task := range []Task{
		{ID: "older", Skill: "billing", Operation: "revenue-calculation", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{"report_date": "2026-08-28"}, CreatedAt: "2026-08-29T00:00:00Z", UpdatedAt: "2026-08-29T00:00:01Z"},
		{ID: "failed-newer", Skill: "billing", Operation: "revenue-calculation", Status: "failed", Progress: 100, Message: "失败", Result: map[string]any{}, CreatedAt: "2026-08-30T00:00:00Z", UpdatedAt: "2026-08-30T00:00:01Z"},
		{ID: "other", Skill: "billing", Operation: "price-group-allocation", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{}, CreatedAt: "2026-08-31T00:00:00Z", UpdatedAt: "2026-08-31T00:00:01Z"},
	} {
		if err := store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	task, err := store.LatestByOperation(ctx, "revenue-calculation", "succeeded")
	if err != nil || task.ID != "older" || task.Result["report_date"] != "2026-08-28" {
		t.Fatalf("task=%#v err=%v", task, err)
	}
	if _, err := store.LatestByOperation(ctx, "revenue-calculation", "cancelled"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing status err=%v", err)
	}
}

func TestOpenCreatesAndRepairsTaskIndexes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ix_tasks_updated_at", "ix_tasks_status_updated_at", "ix_tasks_skill_updated_at", "ix_tasks_operation_status_updated_at", "ix_tasks_log_listing", "ix_tasks_log_search"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&count); err != nil || count != 1 {
			t.Fatalf("task index %s missing: count=%d err=%v", name, count, err)
		}
	}
	if _, err := store.db.Exec(`DROP INDEX ix_tasks_log_listing`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var repaired int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ix_tasks_log_listing'`).Scan(&repaired); err != nil || repaired != 1 {
		t.Fatalf("reopening an existing task database did not repair indexes: count=%d err=%v", repaired, err)
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
	for _, row := range []struct {
		id     string
		result string
	}{
		{id: "wrong-shape", result: `[]`},
		{id: "trailing-garbage", result: `{} garbage`},
	} {
		if _, err := db.Exec(`INSERT INTO tasks VALUES(?, 'console','inspect','succeeded',100,'done',?,?,?)`, row.id, row.result, now, now); err != nil {
			t.Fatal(err)
		}
		task, err := store.Get(context.Background(), row.id)
		if err != nil || task.Status != "failed" || task.Result["storage_corrupt"] != true {
			t.Fatalf("%s unexpected corrupt projection: %#v err=%v", row.id, task, err)
		}
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
