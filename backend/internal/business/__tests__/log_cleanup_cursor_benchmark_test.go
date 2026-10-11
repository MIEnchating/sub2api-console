package business_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkLogCleanupEventsWithRetainedDates(b *testing.B) {
	for _, retained := range []int{0, 20000, 200000} {
		b.Run(fmt.Sprintf("retained=%d", retained), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				b.StopTimer()
				path := filepath.Join(b.TempDir(), "cleanup.db")
				store, err := business.Open(path)
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() { _ = store.Close() })
				db, err := sql.Open("sqlite", path)
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() { _ = db.Close() })
				if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?)
					INSERT INTO runtime_events(source_id,event_type,created_at,status,summary)
					SELECT x,'fixture',CASE WHEN x<=? THEN '0000-invalid' ELSE '2026-01-01T00:00:00Z' END,
					'succeeded','fixture' FROM n`, retained+20000, retained); err != nil {
					b.Fatal(err)
				}
				cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
				b.StartTimer()
				result, err := store.ClearLogRecords(b.Context(), &cutoff)
				b.StopTimer()
				if err != nil || result.Events != 20000 {
					b.Fatalf("cleanup=%+v error=%v", result, err)
				}
				var remaining int
				if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events`).Scan(&remaining); err != nil || remaining != retained {
					b.Fatalf("retained=%d error=%v", remaining, err)
				}
				if err := db.Close(); err != nil {
					b.Fatal(err)
				}
				if err := store.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
