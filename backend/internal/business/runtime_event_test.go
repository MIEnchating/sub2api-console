package business

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
)

func TestRecordRuntimeEventPersistsBackgroundTaskID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "runtime-event.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := taskcontext.WithID(context.Background(), "task-42")
	if _, err := store.RecordRuntimeEvent(ctx, "test.completed", "succeeded", "测试完成", map[string]any{"account_id": "41"}); err != nil {
		t.Fatal(err)
	}
	var encoded string
	if err := store.db.QueryRowContext(ctx, `SELECT payload_json FROM runtime_events ORDER BY source_id LIMIT 1`).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(encoded), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["task_id"] != "task-42" {
		t.Fatalf("runtime event task_id=%#v", payload["task_id"])
	}
}
