package taskrecovery_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestRecoveryDoesNotDispatchRemovedOrCorruptCheckpoint(t *testing.T) {
	for _, fixture := range []struct{ name, statement, status string }{
		{"removed", "DELETE FROM task_recovery WHERE task_id='pending'", "queued"},
		{"corrupt", "UPDATE task_recovery SET payload='not-json' WHERE task_id='pending'", "failed"},
		{"already running", "UPDATE tasks SET status='running' WHERE id='pending'", "running"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recovery.db")
			store, err := taskstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			ctx := context.Background()
			task := taskstore.Task{ID: "pending", Skill: "test", Operation: "test-recovery", Status: "running", Progress: 40, Message: "处理中", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"completed": 1}}
			if err := taskstore.WithRecovery(&task, []string{"41"}); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(ctx, task); err != nil {
				t.Fatal(err)
			}
			pending, err := store.PendingRecovery(ctx)
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(ctx, fixture.statement); err != nil {
				t.Fatal(err)
			}
			taskrecovery.Run(ctx, store, pending, map[string]taskrecovery.Handler{"test-recovery": func(context.Context, taskstore.Task) error {
				t.Fatal("unavailable checkpoint was dispatched")
				return nil
			}})
			current, err := store.Get(ctx, task.ID)
			if err != nil || current.Status != fixture.status || current.Result["completed"] != json.Number("1") {
				t.Fatalf("checkpoint handling lost state: %#v, %v", current, err)
			}
			if fixture.status == "failed" && current.Message != "任务恢复失败：任务恢复数据损坏：恢复参数不是有效 JSON" {
				t.Fatalf("missing failure reason: %s", current.Message)
			}
		})
	}
}
