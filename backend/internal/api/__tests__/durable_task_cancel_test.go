package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type rejectedCancelStore struct{ *taskstore.Store }

func (*rejectedCancelStore) CancelTask(context.Context, string) (bool, error) {
	return false, errors.New("isolated cancellation write failed")
}

func TestTaskCancellationPersistenceFailureDoesNotSignalWorkerOrClaimSuccess(t *testing.T) {
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
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "group", Skill: "fixture", Operation: "account-model-automation-group", Status: "running", Message: "执行中", Result: map[string]any{}, CreatedAt: stamp, UpdatedAt: stamp}
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	observer := &durableCancelObserver{store: store}
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{Tasks: &rejectedCancelStore{store}, TaskCanceller: observer})
	req := httptest.NewRequest(http.MethodDelete, "/api/tasks/group", nil)
	req.Header.Set("Authorization", "Bearer isolated-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusInternalServerError || observer.calls != 0 {
		t.Fatalf("failed persistence accepted: response=%d worker calls=%d", response.Code, observer.calls)
	}
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || got.Status != "running" {
		t.Fatalf("failed persistence changed task: %s %v", got.Status, err)
	}
}

type durableCancelObserver struct {
	store    *taskstore.Store
	observed taskstore.Task
	err      error
	calls    int
}

func (r *durableCancelObserver) CancelTask(id string) bool {
	r.calls++
	r.observed, r.err = r.store.Get(context.Background(), id)
	return true
}

func TestTaskCancellationWithoutRecoveryIsDurableBeforeWorkerSignal(t *testing.T) {
	for _, status := range []string{"queued", "running", "waiting_input"} {
		t.Run(status, func(t *testing.T) {
			dir := t.TempDir()
			private, err := configstore.Open(filepath.Join(dir, "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer private.Close()
			path := filepath.Join(dir, "tasks.db")
			store, err := taskstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			now := time.Now().UTC().Format(time.RFC3339Nano)
			task := taskstore.Task{ID: "group", Skill: "sub2api-account-model-automation", Operation: "account-model-automation-group", Status: status, Message: "正在执行成员", Progress: 50, Result: map[string]any{"completed_member": "first"}, CreatedAt: now, UpdatedAt: now}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			observer := &durableCancelObserver{store: store}
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{Tasks: store, TaskCanceller: observer})
			req := httptest.NewRequest(http.MethodDelete, "/api/tasks/group", nil)
			req.Header.Set("Authorization", "Bearer isolated-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusAccepted || observer.calls != 1 || observer.err != nil || observer.observed.Status != "cancelled" {
				t.Fatalf("worker signalled before durable cancellation: response=%d calls=%d status=%s err=%v", response.Code, observer.calls, observer.observed.Status, observer.err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := taskstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if _, err := reopened.RecoverInterrupted(t.Context()); err != nil {
				t.Fatal(err)
			}
			got, err := reopened.Get(t.Context(), task.ID)
			if err != nil || got.Status != "cancelled" || got.Result["completed_member"] != "first" || got.Result["cancelled"] != true {
				t.Fatalf("restart lost cancellation or results: %#v err=%v", got, err)
			}
		})
	}
}
