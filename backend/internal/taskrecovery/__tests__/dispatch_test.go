package taskrecovery_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func pendingTask(t *testing.T) (*taskstore.Store, []taskstore.Task) {
	t.Helper()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "pending", Skill: "test", Operation: "test-recovery", Status: "running", Message: "处理中", Result: map[string]any{"completed": 1}, CreatedAt: now, UpdatedAt: now}
	if err := taskstore.WithRecovery(&task, []string{"1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingRecovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return store, pending
}

func TestRecoveryWaitsForCapacityAndDispatchesOriginalTaskOnce(t *testing.T) {
	store, pending := pendingTask(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	calls := 0
	handler := func(ctx context.Context, task taskstore.Task) error {
		calls++
		if calls == 1 {
			return taskrunner.ErrCapacity
		}
		task.Status = "succeeded"
		task.Message = "已恢复"
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return store.Save(ctx, task)
	}
	taskrecovery.Run(ctx, store, pending, map[string]taskrecovery.Handler{"test-recovery": handler})
	got, err := store.Get(ctx, pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "succeeded" || calls != 2 {
		t.Fatalf("dispatch lost or repeated: %#v calls=%d", got, calls)
	}
}

func TestRecoveryDoesNotDispatchDurablyCancelledTask(t *testing.T) {
	store, pending := pendingTask(t)
	if ok, err := store.CancelRecovery(context.Background(), pending[0].ID); !ok || err != nil {
		t.Fatal(err)
	}
	taskrecovery.Run(context.Background(), store, pending, map[string]taskrecovery.Handler{"test-recovery": func(context.Context, taskstore.Task) error { t.Fatal("cancelled task dispatched"); return nil }})
}

func TestInvalidRecoveryShowsReasonAndKeepsCompletedDetails(t *testing.T) {
	store, pending := pendingTask(t)
	taskrecovery.Run(context.Background(), store, pending, map[string]taskrecovery.Handler{"test-recovery": func(context.Context, taskstore.Task) error { return errors.New("账号绑定已变化") }})
	got, err := store.Get(context.Background(), pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.Message != "任务恢复失败：账号绑定已变化" || got.Result["completed"] == nil {
		t.Fatalf("lost failure details: %#v", got)
	}
	remaining, err := store.PendingRecovery(context.Background())
	if err != nil || len(remaining) != 0 {
		t.Fatalf("invalid task retries: %v %v", remaining, err)
	}
}
