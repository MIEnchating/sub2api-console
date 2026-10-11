package accountautomation_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func BenchmarkQueuedAutomationRecovery(b *testing.B) {
	service, store, pool, _, task := queuedAutomation(b)
	id := task.ID
	task.Result["detail"] = strings.Repeat("x", 1<<20)
	if err := store.Save(context.Background(), task); err != nil {
		b.Fatal(err)
	}
	task.Result = nil
	b.ReportAllocs()
	for b.Loop() {
		current, err := store.RecoveryTask(context.Background(), id)
		if err != nil {
			b.Fatal(err)
		}
		if err := service.ResumeTask(context.Background(), current); err != nil {
			b.Fatal(err)
		}
	}
	runtime.GC()
	var held, released runtime.MemStats
	runtime.ReadMemStats(&held)
	pool.run = nil
	runtime.GC()
	runtime.ReadMemStats(&released)
	runtime.KeepAlive(pool)
	runtime.KeepAlive(service)
	runtime.KeepAlive(store)
	b.ReportMetric(float64(int64(held.HeapAlloc)-int64(released.HeapAlloc)), "queue-heap-B")
}
