package business_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestLogCleanupWithReversedIDsContinuesInTimestampOrderAcrossBatches(t *testing.T) {
	store, db := auditRetentionStore(t)
	// IDs run backwards while dates increase, so candidate insertion order
	// differs from primary-key order. Equal timestamps straddle the batch edge.
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<4001)
		INSERT INTO runtime_events(source_id,event_type,created_at,status,summary)
		SELECT 5000-x,'fixture',CASE WHEN x<=1999 THEN '2026-01-01T00:00:00Z'
		WHEN x<=2002 THEN '2026-01-02T00:00:00Z' ELSE '2026-01-03T00:00:00Z' END,
		'succeeded','expired' FROM n`); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var reports []int64
	result, err := store.ClearLogRecordsWithProgress(t.Context(), &cutoff, func(progress business.LogCleanupResult) error {
		reports = append(reports, progress.Events)
		return nil
	})
	if err != nil || result.Events != 4001 || !reflect.DeepEqual(reports, []int64{2000, 4000, 4001}) {
		t.Fatalf("cleanup=%+v reports=%v error=%v", result, reports, err)
	}
	var remaining int
	if err := db.QueryRow("SELECT COUNT(*) FROM runtime_events").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("remaining=%d error=%v", remaining, err)
	}
}
