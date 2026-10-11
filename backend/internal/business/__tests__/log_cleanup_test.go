package business_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestLogCleanupCancellationKeepsCommittedBatchAndAllowsRetry(t *testing.T) {
	store, db := auditRetentionStore(t)
	if _, err := db.Exec("WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2001) INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) SELECT x,'fixture','2026-01-01T00:00:00Z','succeeded','expired' FROM n"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := store.ClearLogRecordsWithProgress(ctx, nil, func(progress business.LogCleanupResult) error {
		if progress.Events == 0 {
			return nil
		}
		if progress.Events != 2000 {
			t.Fatalf("unbounded batch: %d", progress.Events)
		}
		var remaining int
		if err := db.QueryRow("SELECT COUNT(*) FROM runtime_events").Scan(&remaining); err != nil {
			return err
		}
		if remaining != 1 {
			t.Fatalf("progress preceded commit: %d", remaining)
		}
		if _, err := db.Exec("INSERT INTO runtime_events(source_id,event_type,created_at,status,summary) VALUES(3000,'fixture','2026-01-01T00:00:00Z','succeeded','other writer')"); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || result.Events != 2000 {
		t.Fatalf("partial=%+v error=%v", result, err)
	}
	result, err = store.ClearLogRecords(t.Context(), nil)
	if err != nil || result.Events != 2 {
		t.Fatalf("retry=%+v error=%v", result, err)
	}
}
