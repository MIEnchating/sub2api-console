package taskstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestServiceStopDoesNotLookLikeUserCancellation(t *testing.T) {
	group := taskrunner.New(context.Background())
	started := make(chan struct{})
	done := make(chan taskstore.Task, 1)
	if err := group.GoTask("restart", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		task := taskstore.Task{Status: "running", Progress: 40, Result: map[string]any{"completed": 2}}
		taskstore.MarkCancelled(ctx, &task, "用户已取消")
		done <- task
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	group.Cancel()
	task := <-done
	if task.Status == "cancelled" || task.Result["interrupted"] != true {
		t.Fatalf("service stop lost interruption reason: %#v", task)
	}
	if err := group.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRestartPreservesCompletedBackgroundResults(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "batch", Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "running", Message: "检测中", Progress: 50, Result: map[string]any{"completed": 1, "animations": []any{map[string]any{"account_id": "1", "status": "succeeded"}}}, CreatedAt: now, UpdatedAt: now}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverInterrupted(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result["animations"] == nil {
		t.Fatal("restart discarded completed account results")
	}
}

func TestRecoverableTaskSurvivesGracefulStopAndRepeatedDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tasks.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "recover", Skill: "test", Operation: "account-rate-sync", Status: "running", Progress: 40, Message: "同步中", Result: map[string]any{"completed": 2}, CreatedAt: now, UpdatedAt: now}
	if err := taskstore.WithRecovery(&task, map[string]any{"account_ids": []string{"1", "2"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	stopped, cancel := context.WithCancelCause(ctx)
	cancel(taskcontext.ErrInterrupted)
	taskstore.MarkCancelled(stopped, &task, "")
	if task.Status != "queued" || task.Progress != 40 {
		t.Fatalf("lost progress: %#v", task)
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	store.Close()
	for range 2 {
		store, err = taskstore.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecoverInterrupted(ctx); err != nil {
			t.Fatal(err)
		}
		pending, err := store.PendingRecovery(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 1 || pending[0].ID != task.ID || pending[0].Progress != 40 {
			t.Fatalf("not resumable: %#v", pending)
		}
		raw, err := json.Marshal(pending[0])
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "account_ids") || strings.Contains(string(raw), "Recovery") {
			t.Fatal("public task exposed recovery inputs")
		}
		store.Close()
	}
}

func TestManualCancellationPersistsBeforeWorkerShutdownAndIsNeverRecovered(t *testing.T) {
	ctx := context.Background()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "cancel", Skill: "test", Operation: "account-rate-sync", Status: "running", Message: "同步中", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err := taskstore.WithRecovery(&task, []string{"1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.CancelRecovery(ctx, task.ID)
	if err != nil || !cancelled {
		t.Fatalf("cancel: %v %v", cancelled, err)
	}
	// A stale worker cannot recreate a checkpoint after cancellation was accepted.
	if err := store.Save(ctx, task); !errors.Is(err, taskstore.ErrTaskTerminal) {
		t.Fatalf("stale save: %v", err)
	}
	if _, err := store.RecoverInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingRecovery(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("cancelled task resumed: %#v %v", pending, err)
	}
	got, err := store.Get(ctx, task.ID)
	if err != nil || got.Status != "cancelled" {
		t.Fatalf("cancellation lost: %#v %v", got, err)
	}
}

func TestHardCrashKeepsRecoverableTaskAndOrdinaryFailureDoesNotRetry(t *testing.T) {
	for _, status := range []string{"queued", "running", "failed", "succeeded"} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			now := time.Now().UTC().Format(time.RFC3339Nano)
			task := taskstore.Task{ID: "crash", Skill: "test", Operation: "account-model-animation", Status: status, Message: "检测", Result: map[string]any{"completed": 1}, CreatedAt: now, UpdatedAt: now}
			if err := taskstore.WithRecovery(&task, []string{"1"}); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(ctx, task); err != nil {
				t.Fatal(err)
			}
			if _, err := store.RecoverInterrupted(ctx); err != nil {
				t.Fatal(err)
			}
			pending, err := store.PendingRecovery(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if status == "queued" || status == "running" {
				want = 1
			}
			if len(pending) != want {
				t.Fatalf("pending=%d want %d", len(pending), want)
			}
		})
	}
}

func TestShutdownBeforeDispatchKeepsZeroProgressForRecovery(t *testing.T) {
	for _, phase := range []string{"launch", "save-running"} {
		t.Run(phase, func(t *testing.T) {
			store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			now := time.Now().UTC().Format(time.RFC3339Nano)
			task := taskstore.Task{ID: "queued", Skill: "test", Operation: "account-model-animation", Status: "queued", Message: "等待启动", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}
			if err := taskstore.WithRecovery(&task, []string{"1"}); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if phase == "launch" {
				taskstore.PersistLaunchFailure(store, task, taskrunner.ErrStopped)
			} else {
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(taskcontext.ErrInterrupted)
				if taskstore.SaveRunning(ctx, store, task) {
					t.Fatal("stopped task started")
				}
			}
			got, err := store.Get(context.Background(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "queued" || got.Progress != 0 {
				t.Fatalf("invented progress: %#v", got)
			}
		})
	}
}
