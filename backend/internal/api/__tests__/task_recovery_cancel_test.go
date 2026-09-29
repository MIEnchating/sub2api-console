package api_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestPendingRecoveryCancellationAuthenticatesAndPersistsBeforeDispatch(t *testing.T) {
	dir := t.TempDir()
	private, err := configstore.Open(filepath.Join(dir, "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	store, err := taskstore.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := taskrunner.New(context.Background())
	defer runner.Cancel()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "pending", Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "queued", Message: "等待恢复", Result: map[string]any{"completed": 1}, CreatedAt: now, UpdatedAt: now}
	if err := taskstore.WithRecovery(&task, []string{"41"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{Tasks: store, TaskCanceller: runner})
	for _, authenticated := range []bool{false, true} {
		request := httptest.NewRequest("DELETE", "/api/tasks/pending", nil)
		if authenticated {
			request.Header.Set("Authorization", "Bearer isolated-token")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		expected := 401
		if authenticated {
			expected = 202
		}
		if response.Code != expected {
			t.Fatalf("response %d: %s", response.Code, response.Body.String())
		}
	}
	pending, err := store.PendingRecovery(context.Background())
	if err != nil || len(pending) != 0 {
		t.Fatalf("cancelled recovery dispatched: %#v %v", pending, err)
	}
	final, err := store.Get(context.Background(), task.ID)
	if err != nil || final.Status != "cancelled" || final.Result["completed"] == nil {
		t.Fatalf("cancellation lost results: %#v %v", final, err)
	}
}
