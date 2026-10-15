package modelcheck_test

import (
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"net/http"
	"testing"
	"time"
)

func qualitySourceTask(id string) taskstore.Task {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return taskstore.Task{ID: id, Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "succeeded", Message: "动画检测完成", CreatedAt: now, UpdatedAt: now,
		Result: map[string]any{"remote_write": false, "precise_value": json.Number("9007199254740993"), "animations": []any{
			map[string]any{"account_id": "1", "account_name": "同名账号", "model": "test-model", "request_id": "request-1", "status": "succeeded", "svg": fixtureSVG, "completed_at": now},
			map[string]any{"account_id": "2", "account_name": "同名账号", "model": "test-model", "request_id": "request-2", "status": "succeeded", "svg": fixtureSVG, "completed_at": now},
		}}}
}

func qualitySourceFixture(t *testing.T) *fixture {
	t.Helper()
	return setup(t, 2, "openai", func(http.ResponseWriter, *http.Request) { t.Error("quality annotation must not call upstream") })
}
