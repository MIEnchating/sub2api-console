package business_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func openTrafficRankingStore(t testing.TB) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "traffic-ranking.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func TestTrafficRankingCountsActivityWithinRequestedTimeBuckets(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		start    time.Time
		duration time.Duration
		offsets  []time.Duration
		want     int
	}{
		{
			name:  "hour crossing UTC clock boundary remains one requested hour",
			start: time.Date(2026, 9, 14, 9, 30, 0, 0, time.UTC), duration: time.Hour,
			offsets: []time.Duration{10 * time.Minute, 50 * time.Minute}, want: 1,
		},
		{
			name:  "inclusive window endpoint belongs to the final bucket",
			start: time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC), duration: time.Hour,
			offsets: []time.Duration{0, time.Hour}, want: 1,
		},
		{
			name:  "partial final day does not add an extra UTC calendar bucket",
			start: time.Date(2026, 9, 10, 23, 30, 0, 0, time.UTC), duration: 49 * time.Hour,
			offsets: []time.Duration{0, time.Hour, 25 * time.Hour, 49 * time.Hour}, want: 3,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store, db := openTrafficRankingStore(t)
			if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','活跃账号','{}','now')`); err != nil {
				t.Fatal(err)
			}
			for index, offset := range fixture.offsets {
				observedAt := fixture.start.Add(offset).Format("2006-01-02T15:04:05.000000000Z")
				if _, err := db.Exec(`INSERT INTO usage_records(request_id,account_id,observed_at,source,payload_json)
					VALUES(?,'41',?,'traffic','{}')`, index, observedAt); err != nil {
					t.Fatal(err)
				}
			}
			result, err := store.TrafficRanking(context.Background(), business.TrafficRankingQuery{
				StartAt: fixture.start, EndAt: fixture.start.Add(fixture.duration),
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Accounts) != 1 || result.Accounts[0].ActiveBuckets != fixture.want || result.Accounts[0].TotalBuckets != fixture.want {
				t.Fatalf("activity = %#v; want %d / %d buckets", result.Accounts, fixture.want, fixture.want)
			}
		})
	}
}
