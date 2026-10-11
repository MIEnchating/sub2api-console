package logs_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/logs"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func cleanupFixture(t *testing.T) (*logs.Maintenance, *taskstore.Store, *sql.DB, *taskrunner.Group) {
	t.Helper()
	dir := t.TempDir()
	b, err := business.Open(filepath.Join(dir, "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	if err := b.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskstore.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tasks.Close() })
	settings, err := configstore.Open(filepath.Join(dir, "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { settings.Close() })
	db, err := sql.Open("sqlite", filepath.Join(dir, "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	runner := taskrunner.New(t.Context())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runner.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	m := logs.NewMaintenance(settings, b, tasks)
	m.UseTaskRunner(runner, tasks)
	return m, tasks, db, runner
}

func waitCleanupTask(t *testing.T, store *taskstore.Store, id string) taskstore.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		task, err := store.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status != "queued" && task.Status != "running" {
			return task
		}
		select {
		case <-ctx.Done():
			t.Fatal("cleanup did not finish")
		case <-ticker.C:
		}
	}
}

func TestCleanupTaskReturnsBeforeDeletionAndRejectsDuplicateSubmission(t *testing.T) {
	m, tasks, db, _ := cleanupFixture(t)
	if _, err := db.Exec("INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) VALUES(1,'fixture','2000-01-01T00:00:00Z','succeeded','expired')"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE runtime_events SET summary='held writer'"); err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(t.Context())
	task, err := m.Enqueue(request, 30)
	cancel()
	if err != nil || task.ID == "" || task.Status != "queued" {
		t.Fatalf("enqueue=%+v error=%v", task, err)
	}
	if _, err := m.Enqueue(t.Context(), 30); err == nil {
		t.Fatal("duplicate cleanup accepted")
	}
	status, err := m.Status(t.Context())
	if err != nil || status.TaskID == nil || *status.TaskID != task.ID {
		t.Fatalf("active task missing from status: %+v error=%v", status, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	final := waitCleanupTask(t, tasks, task.ID)
	if final.Status != "succeeded" || final.Result["total"] != json.Number("1") {
		t.Fatalf("final=%+v", final)
	}
	// The response remains a queued snapshot, with no concurrent map mutation.
	if task.Result["phase"] != "queued" {
		t.Fatalf("response mutated: %+v", task)
	}
}

func TestCleanupTaskFailureReportsCommittedRowsAndDoesNotMarkCompletion(t *testing.T) {
	m, tasks, db, _ := cleanupFixture(t)
	if _, err := db.Exec("WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2001) INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) SELECT x,'fixture','2000-01-01T00:00:00Z','succeeded','expired' FROM n"); err != nil {
		t.Fatal(err)
	}
	// Force the second batch to fail, without depending on execution timing.
	if _, err := db.Exec("CREATE TRIGGER stop_second_batch BEFORE DELETE ON runtime_events WHEN (SELECT COUNT(*) FROM runtime_events)=1 BEGIN SELECT RAISE(ABORT,'fixture second batch failed'); END"); err != nil {
		t.Fatal(err)
	}
	task, err := m.Enqueue(t.Context(), 30)
	if err != nil {
		t.Fatal(err)
	}
	final := waitCleanupTask(t, tasks, task.ID)
	if final.Status != "failed" || final.Result["total"] != json.Number("2000") {
		t.Fatalf("final=%+v", final)
	}
	status, err := m.Status(t.Context())
	if err != nil || status.LastRunAt != nil {
		t.Fatalf("status=%+v error=%v", status, err)
	}
	var remaining int
	if err := db.QueryRow("SELECT COUNT(*) FROM runtime_events").Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("remaining=%d error=%v", remaining, err)
	}
}

func TestCleanupTaskCancellationPreservesActiveRecords(t *testing.T) {
	m, tasks, db, runner := cleanupFixture(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) VALUES(1,'fixture','2000-01-01T00:00:00Z','succeeded','expired')"); err != nil {
		t.Fatal(err)
	}
	task, err := m.Enqueue(t.Context(), 30)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.CancelTask(task.ID) {
		t.Fatal("cleanup not cancellable")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	final := waitCleanupTask(t, tasks, task.ID)
	if final.Status != "cancelled" {
		t.Fatalf("final=%+v", final)
	}
}
