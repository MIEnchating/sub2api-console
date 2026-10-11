package taskstore_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func BenchmarkLogCleanupWithActiveHistory(b *testing.B) {
	benchmarkLogCleanupWithActiveHistory(b, nil, "2026-01-02T00:00:00.000000000Z", 0)
}

func BenchmarkLogCleanupWithRetention(b *testing.B) {
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	benchmarkLogCleanupWithActiveHistory(b, &cutoff, "2026-01-02T00:00:00.000000000Z", 0)
}

func BenchmarkLogCleanupWithLegacyRetention(b *testing.B) {
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	benchmarkLogCleanupWithActiveHistory(b, &cutoff, "2026-01-02T08:00:00.12+08:00", 0)
}

func BenchmarkLogCleanupWithRetainedLegacyDates(b *testing.B) {
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, retained := range []int{20000, 200000} {
		b.Run(fmt.Sprintf("retained=%d", retained), func(b *testing.B) {
			benchmarkLogCleanupWithActiveHistory(b, &cutoff, "2026-01-02T00:00:00.000000000Z", retained)
		})
	}
}

func benchmarkLogCleanupWithActiveHistory(b *testing.B, cutoff *time.Time, expiredStamp string, retainedLegacy int) {
	for _, withIndex := range []bool{false, true} {
		b.Run(fmt.Sprintf("cleanup-index=%t", withIndex), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				b.StopTimer()
				path := filepath.Join(b.TempDir(), "cleanup.db")
				store, err := taskstore.Open(path)
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() { _ = store.Close() })
				db, err := sql.Open("sqlite", path)
				if err != nil {
					store.Close()
					b.Fatal(err)
				}
				b.Cleanup(func() { _ = db.Close() })
				if !withIndex {
					if _, err := db.Exec(`DROP INDEX ix_tasks_log_cleanup`); err != nil {
						b.Fatal(err)
					}
				}
				// Older active tasks exercise the repeated scanning avoided by the partial index.
				if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000)
					INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
					SELECT printf('task-%08d',x),'fixture','fixture',CASE WHEN x<=5000 THEN 'running' ELSE 'succeeded' END,
					0,'fixture','{}','2026-01-01T00:00:00.000000000Z',
					CASE WHEN x<=5000 THEN '2026-01-01T00:00:00.000000000Z' ELSE ? END FROM n`, expiredStamp); err != nil {
					b.Fatal(err)
				}
				if retainedLegacy > 0 {
					if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?)
						INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
						SELECT printf('retained-%08d',x),'fixture','fixture','succeeded',100,'fixture','{}',
						'2026-01-01T00:00:00Z','0000-invalid' FROM n`, retainedLegacy); err != nil {
						b.Fatal(err)
					}
				}
				b.StartTimer()
				deleted, protected, err := store.ClearLogs(b.Context(), cutoff)
				b.StopTimer()
				if err != nil || deleted != 15000 || protected != 5000 {
					b.Fatalf("cleanup deleted=%d protected=%d err=%v", deleted, protected, err)
				}
				if retainedLegacy > 0 {
					var retained int
					if err := db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE updated_at='0000-invalid'`).Scan(&retained); err != nil || retained != retainedLegacy {
						b.Fatalf("retained legacy dates=%d err=%v", retained, err)
					}
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
