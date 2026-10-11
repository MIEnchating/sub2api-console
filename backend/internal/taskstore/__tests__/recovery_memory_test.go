package taskstore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestPendingRecoveryKeepsOnlyReferencesAndPreservesStoredDetails(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	task := taskstore.Task{ID: "recover-large", Skill: "test", Operation: "test-recovery", Status: "running", Progress: 40, Message: "处理中", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"completed": 1, "detail": strings.Repeat("x", 256<<10)}}
	if err := taskstore.WithRecovery(&task, map[string]any{"account_ids": []string{"41"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingRecovery(ctx)
	if err != nil || len(pending) != 1 || pending[0].ID != task.ID {
		t.Fatalf("pending: %#v %v", pending, err)
	}
	raw, err := json.Marshal(pending)
	if err != nil || len(raw) > 1024 || strings.Contains(string(raw), "account_ids") || strings.Contains(string(raw), "detail") {
		t.Fatalf("waiting queue retained execution payload: %d bytes, %v", len(raw), err)
	}
	stored, err := store.Get(ctx, task.ID)
	if err != nil || stored.Status != "queued" || stored.Progress != 40 || stored.Result["detail"] != task.Result["detail"] || stored.Result["completed"] != json.Number("1") {
		t.Fatalf("preparation lost persisted progress or details: status=%s, err=%v", stored.Status, err)
	}
}

func BenchmarkPendingRecovery(b *testing.B) {
	store, err := taskstore.Open(filepath.Join(b.TempDir(), "recovery.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for index := range 128 {
		task := taskstore.Task{ID: fmt.Sprintf("recover-%03d", index), Skill: "test", Operation: "test-recovery", Status: "running", Progress: 40, Message: "处理中", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"completed": 1, "detail": strings.Repeat("x", 64<<10)}}
		if err := taskstore.WithRecovery(&task, map[string]any{"account_ids": []string{"41"}}); err != nil {
			b.Fatal(err)
		}
		if err := store.Save(ctx, task); err != nil {
			b.Fatal(err)
		}
	}
	runtime.GC()
	var held, released runtime.MemStats
	var retained any
	b.ReportAllocs()
	for b.Loop() {
		pending, err := store.PendingRecovery(ctx)
		if err != nil || len(pending) != 128 {
			b.Fatalf("pending count=%d, err=%v", len(pending), err)
		}
		retained = pending
	}
	runtime.GC()
	runtime.ReadMemStats(&held)
	runtime.KeepAlive(retained)
	retained = nil
	runtime.GC()
	runtime.ReadMemStats(&released)
	runtime.KeepAlive(store)
	b.ReportMetric(float64(int64(held.HeapAlloc)-int64(released.HeapAlloc)), "queue-heap-B")
}
