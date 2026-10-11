package business_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkRecoveryTrafficCanonicalHistory(b *testing.B) {
	store, db := recoveryTrafficBenchmarkStore(b)
	stamp := "2026-10-01T12:00:30.000000000Z"
	payload := `{"model":"fixture","text":"` + strings.Repeat("x", 4096) + `"}`
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000)
		INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
		SELECT '41',printf('recent-%d',(x-1)/2),printf('group-%d',x%2),'traffic',0,?,? FROM n`, stamp, payload); err != nil {
		b.Fatal(err)
	}
	since := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rows, err := store.RoutingRecoveryTraffic(b.Context(), "41", since, since.Add(time.Minute))
		if err != nil || len(rows) != 1001 {
			b.Fatalf("recovery interval=%d rows, error=%v", len(rows), err)
		}
	}
}

func BenchmarkRecoveryTrafficWriteBatch(b *testing.B) {
	_, db := recoveryTrafficBenchmarkStore(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := db.Exec(`BEGIN;
			WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
			INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
			SELECT '42',printf('write-%d',x),'codex','traffic',0,'2026-10-01T12:00:30.000000000Z','{}' FROM n WHERE true
			ON CONFLICT(request_id,account_id,group_name,observed_at) DO UPDATE SET is_error=excluded.is_error;
			COMMIT`); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRecoveryTrafficInsertBatch(b *testing.B) {
	for _, withIndex := range []bool{false, true} {
		b.Run(fmt.Sprintf("recovery-index=%t", withIndex), func(b *testing.B) {
			_, db := recoveryTrafficBenchmarkStore(b)
			if !withIndex {
				if _, err := db.Exec(`DROP INDEX ix_usage_records_recovery_time`); err != nil {
					b.Fatal(err)
				}
			}
			sequence := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				tx, err := db.BeginTx(b.Context(), nil)
				if err != nil {
					b.Fatal(err)
				}
				_, err = tx.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
					INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
					SELECT '42',printf('insert-%d-%d',?,x),'codex','traffic',0,'2026-10-01T12:00:30.000000000Z','{}' FROM n`, sequence)
				if err != nil {
					tx.Rollback()
					b.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					b.Fatal(err)
				}
				sequence++
			}
		})
	}
}

func BenchmarkRecoveryTrafficIndexBuild(b *testing.B) {
	_, db := recoveryTrafficBenchmarkStore(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		if _, err := db.Exec(`DROP INDEX ix_usage_records_recovery_time`); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if _, err := db.Exec(`CREATE INDEX ix_usage_records_recovery_time ON usage_records(
			account_id,console_recovery_time_v1(observed_at) DESC,id DESC
		) WHERE LOWER(REPLACE(source,'_','-'))='traffic'`); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
	}
	var bytes int64
	if err := db.QueryRow(`SELECT COALESCE(SUM(pgsize),0) FROM dbstat WHERE name='ix_usage_records_recovery_time'`).Scan(&bytes); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(bytes), "index-B")
}

func recoveryTrafficBenchmarkStore(b *testing.B) (*business.Store, *sql.DB) {
	b.Helper()
	path := filepath.Join(b.TempDir(), "recovery.db")
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
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<100000)
		INSERT INTO usage_records(account_id,request_id,group_name,source,is_error,observed_at,payload_json)
		SELECT '41',printf('old-%d',x),'codex','traffic',0,'2026-09-01T12:00:00.000000000Z','{}' FROM n`); err != nil {
		b.Fatal(err)
	}
	return store, db
}
