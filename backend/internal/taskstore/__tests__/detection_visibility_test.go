package taskstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDetectionTasksRemainVisibleWithoutExplicitFlagAcrossLifecycleAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	operations := []string{"account-model-animation", "account-model-precheck", "account-model-combined", "managed-model-detection", "account-terminal-continuity"}
	statuses := []string{"queued", "running", "succeeded", "failed", "cancelled"}
	for _, operation := range operations {
		for _, status := range statuses {
			task := taskstore.Task{ID: operation + status, Skill: "sub2api-model-animation", Operation: operation, Status: status, Progress: 40, Message: "已完成 2/5", Result: map[string]any{"automatic": true, "animations": []string{"private-large-result"}}, CreatedAt: "2026-09-24T00:00:00Z", UpdatedAt: "2026-09-24T00:00:01Z"}
			if err := store.Save(ctx, task); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := store.Save(ctx, taskstore.Task{ID: "unrelated", Skill: "console", Message: "倍率同步", Operation: "account-rate-sync", Status: "running", Result: map[string]any{}, CreatedAt: "2026-09-24T00:00:00Z", UpdatedAt: "2026-09-24T00:00:01Z"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	rows, err := store.ListConsoleSummaries(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(operations)*len(statuses) {
		t.Fatalf("visible detection summaries = %d, want %d", len(rows), len(operations)*len(statuses))
	}
	for _, row := range rows {
		if row.ID == "unrelated" || row.Progress != 40 || row.Message != "已完成 2/5" {
			t.Fatalf("unexpected summary: %#v", row)
		}
		if _, exists := row.Result["animations"]; exists {
			t.Fatal("list exposed full generated result")
		}
	}
}
