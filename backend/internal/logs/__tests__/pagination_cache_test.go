package logs_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/logs"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// Storage is the controlled boundary; pagination and cache use the real service.
type logStorage struct {
	events []business.RunEvent
	reads  atomic.Int64
	failed atomic.Bool
}

func (s *logStorage) RunRecords(context.Context, *int) ([]business.RunRecord, error) { return nil, nil }
func (s *logStorage) Events(context.Context, *int) ([]business.RunEvent, error) {
	s.reads.Add(1)
	if s.failed.Load() {
		return nil, errors.New("fixture storage unavailable")
	}
	return s.events, nil
}

func TestLogCacheRefreshFailureKeepsRecentDataButNotExpiredData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := paginationStorage(1)
		service := logs.New(storage, storage)
		query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
		if _, err := service.Query(t.Context(), query); err != nil {
			t.Fatal(err)
		}
		storage.failed.Store(true)
		time.Sleep(15 * time.Second)
		recent, err := service.Query(t.Context(), query)
		if err != nil || recent.Total != 1 {
			t.Fatalf("recent data lost: total=%d error=%v", recent.Total, err)
		}
		synctest.Wait()
		time.Sleep(6 * time.Minute)
		if _, err := service.Query(t.Context(), query); err == nil {
			t.Fatal("expired cache hid a storage failure")
		}
	})
}
func (s *logStorage) AuditEvents(context.Context, *int, bool) ([]business.AuditEvent, error) {
	return nil, nil
}
func (s *logStorage) ListLogSummaries(context.Context, *int) ([]taskstore.Task, error) {
	return nil, nil
}
func (s *logStorage) SearchLogs(context.Context, string, *int) ([]taskstore.Task, error) {
	return nil, nil
}

func paginationStorage(count int) *logStorage {
	s := &logStorage{}
	for i := 0; i < count; i++ {
		s.events = append(s.events, business.RunEvent{
			ID: int64(i + 1), EventType: "fixture", CreatedAt: "2026-10-09T00:00:00Z", Status: "succeeded",
			Summary: fmt.Sprintf("event %d", i+1), Payload: map[string]any{"detail": "retained event payload"},
		})
	}
	return s
}

func TestLogPaginationDoesNotRetainUnreturnedEntries(t *testing.T) {
	storage := paginationStorage(1000)
	service := logs.New(storage, storage)
	page, err := service.Query(t.Context(), logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 20 || page.Total != 1000 {
		t.Fatalf("wrong page: items=%d total=%d", len(page.Items), page.Total)
	}
	// Capacity is a memory ownership contract: a held page must not keep the
	// entire filtered backing array alive after its cache entry is evicted.
	if cap(page.Items) != len(page.Items) {
		t.Fatalf("20-row page retains backing storage for %d entries", cap(page.Items))
	}
}

func TestLogPaginationReusesFreshFilteredRecordsAcrossPageAndSizeChanges(t *testing.T) {
	storage := paginationStorage(1000)
	service := logs.New(storage, storage)
	query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
	first, err := service.Query(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	query.Page = 2
	second, err := service.Query(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	if first.Items[0].ID == second.Items[0].ID {
		t.Fatal("page change returned the first page")
	}
	query.PageSize = 50
	resized, err := service.Query(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(resized.Items) != 50 || resized.Page != 2 || resized.PageSize != 50 {
		t.Fatalf("resized page=%+v", resized)
	}
	if reads := storage.reads.Load(); reads != 1 {
		t.Fatalf("three pages reread the same fresh records %d times", reads)
	}
}

func TestLogFilterCacheEvictsOldFiltersAfterEightDistinctQueries(t *testing.T) {
	storage := paginationStorage(1)
	service := logs.New(storage, storage)
	query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
	for i := 0; i < 9; i++ {
		query.GroupID = fmt.Sprint(i)
		if _, err := service.Query(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}
	query.GroupID = "0"
	if _, err := service.Query(t.Context(), query); err != nil {
		t.Fatal(err)
	}
	if reads := storage.reads.Load(); reads != 10 {
		t.Fatalf("old filter was retained without a cache bound: reads=%d", reads)
	}
}

func TestLogCacheOlderThanMaximumAgeReadsCurrentDataBeforeResponding(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := paginationStorage(1)
		service := logs.New(storage, storage)
		query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
		if _, err := service.Query(t.Context(), query); err != nil {
			t.Fatal(err)
		}
		// synctest advances its virtual clock; no wall-clock wait or timing assertion.
		time.Sleep(6 * time.Minute)
		storage.events = nil
		page, err := service.Query(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 0 {
			t.Fatalf("expired cache returned deleted records: total=%d", page.Total)
		}
		synctest.Wait()
	})
}

func BenchmarkLogPaginationAcrossPages(b *testing.B) {
	storage := paginationStorage(5000)
	b.ReportAllocs()
	for b.Loop() {
		service := logs.New(storage, storage)
		query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
		for page := 1; page <= 5; page++ {
			query.Page = page
			if _, err := service.Query(b.Context(), query); err != nil {
				b.Fatal(err)
			}
		}
	}
}
