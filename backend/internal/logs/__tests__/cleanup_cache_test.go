package logs_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/logs"
)

type cleanableLogStorage struct {
	*logStorage
	failure error
}

func (s *cleanableLogStorage) ClearLogs(context.Context, *time.Time) (int64, int64, error) {
	return 0, 0, nil
}

func (s *cleanableLogStorage) ClearLogRecords(context.Context, *time.Time) (business.LogCleanupResult, error) {
	count := int64(len(s.events))
	if s.failure != nil {
		s.events = s.events[1:]
		return business.LogCleanupResult{Events: 1}, s.failure
	}
	s.events = nil
	return business.LogCleanupResult{Events: count}, nil
}

func TestCompletedCleanupImmediatelyRemovesDeletedLogsFromFreshCache(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		failure   error
		remaining int
	}{
		{"success", nil, 0},
		{"partial failure", errors.New("fixture cleanup failed"), 1},
		{"partial cancellation", context.Canceled, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			storage := &cleanableLogStorage{logStorage: paginationStorage(2), failure: scenario.failure}
			service := logs.New(storage, storage)
			settings, err := configstore.Open(filepath.Join(t.TempDir(), "settings.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer settings.Close()
			maintenance := logs.NewMaintenance(settings, storage, storage)
			maintenance.UseCacheInvalidator(service.Invalidate)
			query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
			before, err := service.Query(t.Context(), query)
			if err != nil || before.Total != 2 {
				t.Fatalf("before cleanup: total=%d error=%v", before.Total, err)
			}
			if _, err := maintenance.ClearExpired(t.Context(), 30); !errors.Is(err, scenario.failure) {
				t.Fatalf("cleanup error=%v want=%v", err, scenario.failure)
			}
			after, err := service.Query(t.Context(), query)
			if err != nil || after.Total != scenario.remaining {
				t.Fatalf("completed cleanup still displays deleted records: total=%d error=%v", after.Total, err)
			}
		})
	}
}

type delayedLogStorage struct {
	*logStorage
	started chan struct{}
	release chan struct{}
}

func (s *delayedLogStorage) Events(ctx context.Context, _ *int) ([]business.RunEvent, error) {
	if s.reads.Add(1) != 1 {
		return nil, nil
	}
	close(s.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return s.events, nil
	}
}

func TestReadStartedBeforeInvalidationCannotRestoreDeletedLogs(t *testing.T) {
	storage := &delayedLogStorage{logStorage: paginationStorage(1), started: make(chan struct{}), release: make(chan struct{})}
	service := logs.New(storage, storage)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	query := logs.Query{Kind: "event", State: "all", Page: 1, PageSize: 20}
	type response struct {
		page logs.Page
		err  error
	}
	done := make(chan response, 1)
	go func() {
		page, err := service.Query(ctx, query)
		done <- response{page, err}
	}()
	select {
	case <-storage.started:
	case <-ctx.Done():
		t.Fatal("log read did not start")
	}
	service.Invalidate()
	close(storage.release)
	select {
	case result := <-done:
		if result.err != nil || result.page.Total != 0 {
			t.Fatalf("read returned records deleted during loading: total=%d error=%v", result.page.Total, result.err)
		}
	case <-ctx.Done():
		t.Fatal("log read did not finish after invalidation")
	}
	page, err := service.Query(ctx, query)
	if err != nil || page.Total != 0 {
		t.Fatalf("late read restored deleted records in cache: total=%d error=%v", page.Total, err)
	}
}
