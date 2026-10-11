package management_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func queuedRateRecovery(t testing.TB) (*taskstore.Store, *management.Service, *rateCollectionTarget) {
	t.Helper()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "queued.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	target := &rateCollectionTarget{settings: configstore.TargetSettings{BaseURL: "http://127.0.0.1:1", AdminKey: "test-only", TimeoutSeconds: 1}}
	return store, management.New(target, nil, store), target
}

func saveQueuedRateTask(t testing.TB, store *taskstore.Store, target *rateCollectionTarget, id string, size int) taskstore.Task {
	t.Helper()
	task := taskstore.Task{ID: id, Skill: "test", Operation: "account-rate-sync", Status: "queued", Progress: 40, Message: "等待恢复", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"completed": 1, "detail": strings.Repeat("x", size)}}
	settings, err := target.TargetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := taskstore.WithRecovery(&task, map[string]any{"account_ids": []string{"11"}, "actor": "test", "target": targetguard.Fingerprint(settings)}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestQueuedRateRecoveryShutdownPreservesLatestStoredProgress(t *testing.T) {
	store, service, target := queuedRateRecovery(t)
	task := saveQueuedRateTask(t, store, target, "shutdown", 64<<10)
	pool := &recoveryRunner{}
	service.UseTaskRunner(pool)
	if err := service.ResumeTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	task.Progress = 70
	task.Result = map[string]any{"completed": 2, "detail": "latest completed result"}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(taskcontext.ErrInterrupted)
	pool.runs[0](ctx)
	got, err := store.RecoveryTask(context.Background(), task.ID)
	if err != nil || got.Progress != 70 || got.Result["completed"] != json.Number("2") || got.Result["detail"] != "latest completed result" {
		t.Fatalf("queued shutdown lost persisted progress: progress=%d completed=%v detail-bytes=%d, %v", got.Progress, got.Result["completed"], len(fmt.Sprint(got.Result["detail"])), err)
	}
}

func TestQueuedRateRecoveryDoesNotOverwriteDurableCancellation(t *testing.T) {
	store, service, target := queuedRateRecovery(t)
	task := saveQueuedRateTask(t, store, target, "cancel", 1024)
	pool := &recoveryRunner{}
	service.UseTaskRunner(pool)
	if err := service.ResumeTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	task.Result = map[string]any{"completed": 2}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if ok, err := store.CancelRecovery(context.Background(), task.ID); err != nil || !ok {
		t.Fatalf("cancel: %v %v", ok, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool.runs[0](ctx)
	got, err := store.Get(context.Background(), task.ID)
	if err != nil || got.Status != "cancelled" || got.Result["completed"] != json.Number("2") || got.Message != "任务已手动取消" {
		t.Fatalf("durable cancellation overwritten: status=%s completed=%v message=%s, %v", got.Status, got.Result["completed"], got.Message, err)
	}
}

func BenchmarkQueuedRateRecovery(b *testing.B) {
	store, service, target := queuedRateRecovery(b)
	saveQueuedRateTask(b, store, target, "queued-rate", 1<<20)
	var pool *recoveryRunner
	b.ReportAllocs()
	for b.Loop() {
		pool = &recoveryRunner{}
		service.UseTaskRunner(pool)
		task, err := store.RecoveryTask(context.Background(), "queued-rate")
		if err != nil {
			b.Fatal(err)
		}
		if err := service.ResumeTask(context.Background(), task); err != nil {
			b.Fatal(err)
		}
	}
	service.UseTaskRunner(nil)
	runtime.GC()
	var held, released runtime.MemStats
	runtime.ReadMemStats(&held)
	runtime.KeepAlive(pool)
	pool = nil
	runtime.GC()
	runtime.ReadMemStats(&released)
	runtime.KeepAlive(service)
	runtime.KeepAlive(store)
	b.ReportMetric(float64(int64(held.HeapAlloc)-int64(released.HeapAlloc)), "queue-heap-B")
}
