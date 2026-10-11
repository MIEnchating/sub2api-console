package logs_test

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/logs"
)

// Return newly allocated payloads as a database reader would, so the fixture
// does not retain strings after Service.Invalidate releases the cached records.
type payloadLogStorage struct {
	logStorage
	payloadBytes int
}

func (s *payloadLogStorage) Events(context.Context, *int) ([]business.RunEvent, error) {
	rows := make([]business.RunEvent, 5000)
	for index := range rows {
		rows[index] = business.RunEvent{ID: int64(index + 1), EventType: "fixture", CreatedAt: "2026-10-09T00:00:00Z", Status: "succeeded",
			Summary: "scope0 scope1 scope2 scope3 scope4 scope5 scope6 scope7", Payload: map[string]any{"detail": strings.Repeat("x", s.payloadBytes)}}
	}
	return rows, nil
}

func BenchmarkLogCachePayloadRetention(b *testing.B) {
	for _, size := range []int{256, 4096} {
		b.Run(fmt.Sprintf("payload-%d", size), func(b *testing.B) {
			storage := &payloadLogStorage{payloadBytes: size}
			var service *logs.Service
			b.ReportAllocs()
			for b.Loop() {
				service = logs.New(storage, storage)
				for filter := range 8 {
					page, err := service.Query(b.Context(), logs.Query{Kind: "event", State: "all", Search: fmt.Sprintf("scope%d", filter), Page: 1, PageSize: 20})
					if err != nil || page.Total != 5000 || len(page.Items) != 20 {
						b.Fatalf("cache load: total=%d items=%d err=%v", page.Total, len(page.Items), err)
					}
				}
			}
			var held, released runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&held)
			service.Invalidate()
			runtime.GC()
			runtime.ReadMemStats(&released)
			runtime.KeepAlive(service)
			b.ReportMetric(float64(int64(held.HeapAlloc)-int64(released.HeapAlloc)), "cache-heap-B")
		})
	}
}
