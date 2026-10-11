package taskrecovery_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestCorruptStartupTaskFailsIndividuallyAndHealthyTaskStillRecovers(t *testing.T) {
	for _, fixture := range []struct{ name, statement string }{
		{"checkpoint", "UPDATE task_recovery SET payload='not-json' WHERE task_id='broken'"},
		{"result", "UPDATE tasks SET result_json='not-json' WHERE id='broken'"},
		{"null result", "UPDATE tasks SET result_json='null' WHERE id='broken'"},
		{"array result", "UPDATE tasks SET result_json='[]' WHERE id='broken'"},
		{"message", "UPDATE tasks SET message='' WHERE id='broken'"},
		{"timestamp", "UPDATE tasks SET updated_at='invalid' WHERE id='broken'"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tasks.db")
			store, err := taskstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			for _, id := range []string{"broken", "healthy"} {
				task := taskstore.Task{ID: id, Skill: "test", Operation: "test-recovery", Status: "running", Progress: 40, Message: "处理中", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"completed": 1}}
				if err := taskstore.WithRecovery(&task, []string{"41"}); err != nil {
					t.Fatal(err)
				}
				if err := store.Save(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(t.Context(), fixture.statement); err != nil {
				t.Fatal(err)
			}
			pending, err := store.PendingRecovery(t.Context())
			if err != nil || len(pending) != 2 {
				t.Fatalf("single corrupt task blocked startup: pending=%v err=%v", pending, err)
			}
			var recovered []string
			taskrecovery.Run(t.Context(), store, pending, map[string]taskrecovery.Handler{
				"test-recovery": func(_ context.Context, task taskstore.Task) error {
					recovered = append(recovered, task.ID)
					task.Status = "succeeded"
					task.Message = "已恢复"
					return store.Save(t.Context(), task)
				},
			})
			if len(recovered) != 1 || recovered[0] != "healthy" {
				t.Fatalf("corrupt task executed or healthy task lost: %v", recovered)
			}
			broken, err := store.Get(t.Context(), "broken")
			if err != nil || broken.Status != "failed" || !strings.Contains(broken.Message, "任务恢复数据损坏") {
				t.Fatalf("missing durable corruption reason: task=%+v err=%v", broken, err)
			}
			if fixture.name == "checkpoint" && broken.Result["completed"] != json.Number("1") {
				t.Fatalf("checkpoint failure discarded completed results: %+v", broken.Result)
			}
			remaining, err := store.PendingRecovery(t.Context())
			if err != nil || len(remaining) != 0 {
				t.Fatalf("failed or completed task retried: pending=%v err=%v", remaining, err)
			}
		})
	}
}
