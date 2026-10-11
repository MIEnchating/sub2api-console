package logs_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/logs"
)

func TestLargeLogFiltersEvictLeastRecentlyUsedDataWithinByteBudget(t *testing.T) {
	storage := paginationStorage(1)
	storage.events[0].Payload = map[string]any{"detail": strings.Repeat("x", 24<<20), "group_ids": []string{"1", "2", "3"}}
	service := logs.New(storage, storage)
	query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
	for _, group := range []string{"1", "2", "1", "3", "1"} {
		query.GroupID = group
		page, err := service.Query(t.Context(), query)
		if err != nil || page.Total != 1 {
			t.Fatalf("large log data lost: total=%d err=%v", page.Total, err)
		}
	}
	if storage.reads.Load() != 3 {
		t.Fatalf("recently used large filter was not retained: reads=%d", storage.reads.Load())
	}
	query.GroupID = "2"
	if _, err := service.Query(t.Context(), query); err != nil {
		t.Fatal(err)
	}
	if storage.reads.Load() != 4 {
		t.Fatalf("three 24MiB payloads remained cached: reads=%d", storage.reads.Load())
	}
}

func TestOversizedLogResultIsReturnedWithoutRetainingItInCache(t *testing.T) {
	storage := paginationStorage(1)
	storage.events[0].Payload = map[string]any{"detail": strings.Repeat("x", 65<<20)}
	service := logs.New(storage, storage)
	query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
	for range 2 {
		page, err := service.Query(t.Context(), query)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("oversized page missing: items=%d err=%v", len(page.Items), err)
		}
		payload := page.Items[0].Details["payload"].(map[string]any)
		if len(payload["detail"].(string)) != 65<<20 {
			t.Fatal("oversized result was truncated")
		}
	}
	if storage.reads.Load() != 2 {
		t.Fatal("oversized result remained in the cache")
	}
}

type gatedLogStorage struct {
	logStorage
	release chan struct{}
	started chan struct{}
}

func (s *gatedLogStorage) Events(ctx context.Context, _ *int) ([]business.RunEvent, error) {
	s.reads.Add(1)
	if s.started != nil {
		select {
		case <-s.started:
		default:
			close(s.started)
		}
	}
	select {
	case <-s.release:
		return s.events, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestConcurrentReadersShareOversizedLoadWithoutCachingIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := &gatedLogStorage{logStorage: *paginationStorage(1), release: make(chan struct{}), started: make(chan struct{})}
		storage.events[0].Payload = map[string]any{"detail": strings.Repeat("x", 65<<20)}
		release := sync.OnceFunc(func() { close(storage.release) })
		defer release()
		service := logs.New(storage, storage)
		done := make(chan error, 2)
		go func() {
			page, err := service.Query(t.Context(), logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20})
			if err == nil && len(page.Items) != 1 {
				err = errors.New("shared result lost records")
			}
			done <- err
		}()
		<-storage.started
		go func() {
			page, err := service.Query(t.Context(), logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20})
			if err == nil && len(page.Items) != 1 {
				err = errors.New("shared result lost records")
			}
			done <- err
		}()
		synctest.Wait()
		release()
		for range 2 {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
		if storage.reads.Load() != 1 {
			t.Fatalf("concurrent oversized query reread storage: reads=%d", storage.reads.Load())
		}
	})
}

func TestDistinctColdLogLoadsAreBoundedAndWaitingCancellationSkipsStorage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := &gatedLogStorage{release: make(chan struct{})}
		release := sync.OnceFunc(func() { close(storage.release) })
		defer release()
		service := logs.New(storage, storage)
		for _, group := range []string{"1", "2"} {
			go func() {
				_, err := service.Query(t.Context(), logs.Query{Kind: "event", State: "all", GroupID: group, Page: 1, PageSize: 20})
				if err != nil {
					t.Error(err)
				}
			}()
		}
		synctest.Wait()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := service.Query(ctx, logs.Query{Kind: "event", State: "all", GroupID: "3", Page: 1, PageSize: 20})
			done <- err
		}()
		synctest.Wait()
		if storage.reads.Load() != 2 {
			t.Errorf("distinct queries loaded more than two snapshots: reads=%d", storage.reads.Load())
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting cancellation: %v", err)
		}
		if storage.reads.Load() != 2 {
			t.Error("cancelled waiter reached storage")
		}
		release()
		synctest.Wait()
		if _, err := service.Query(t.Context(), logs.Query{Kind: "event", State: "all", GroupID: "4", Page: 1, PageSize: 20}); err != nil || storage.reads.Load() != 3 {
			t.Fatalf("load slots did not become available: reads=%d err=%v", storage.reads.Load(), err)
		}
	})
}

type auditPayloadStorage struct {
	logStorage
	audits     []business.AuditEvent
	auditReads atomic.Int64
}

func (s *auditPayloadStorage) AuditEvents(context.Context, *int, bool) ([]business.AuditEvent, error) {
	s.auditReads.Add(1)
	return s.audits, nil
}

func TestLogCacheBudgetIncludesNestedPayloadsInsideTypedAuditRows(t *testing.T) {
	name := "测试账号"
	storage := &auditPayloadStorage{audits: []business.AuditEvent{{ID: 1, OperationID: "change-1", State: "succeeded", ObjectName: &name, CreatedAt: "2026-10-09T00:00:00Z",
		After: map[string]any{"values": []any{map[string]any{"detail": strings.Repeat("x", 24<<20), "exact": json.Number("9007199254740993")}}},
	}}}
	service := logs.New(storage, storage)
	queries := []logs.Query{
		{Kind: "change", State: "all", Page: 1, PageSize: 20},
		{Kind: "all", State: "all", Page: 1, PageSize: 20},
		{Kind: "change", State: "succeeded", Page: 1, PageSize: 20},
	}
	for _, query := range append(queries, queries[0]) {
		page, err := service.Query(t.Context(), query)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("audit page: items=%d err=%v", len(page.Items), err)
		}
		row := page.Items[0].Details["changes"].([]business.AuditEvent)[0]
		value := row.After.(map[string]any)["values"].([]any)[0].(map[string]any)
		if value["exact"] != json.Number("9007199254740993") {
			t.Fatal("cache accounting changed audit numeric precision")
		}
	}
	if storage.auditReads.Load() != 4 {
		t.Fatal("typed audit payloads were excluded from the cache budget")
	}
}

func TestDeepLogPayloadBypassesCacheWithoutChangingTheResult(t *testing.T) {
	storage := paginationStorage(1)
	payload := map[string]any{"detail": "preserved"}
	for range 70 {
		payload = map[string]any{"nested": payload}
	}
	storage.events[0].Payload = payload
	service := logs.New(storage, storage)
	for range 2 {
		page, err := service.Query(t.Context(), logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("deep result missing: %v", err)
		}
		current := page.Items[0].Details["payload"].(map[string]any)
		for range 70 {
			current = current["nested"].(map[string]any)
		}
		if current["detail"] != "preserved" {
			t.Fatal("deep payload changed")
		}
	}
	if storage.reads.Load() != 2 {
		t.Fatal("deep payload remained cached")
	}
}
