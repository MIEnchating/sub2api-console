package business_test

import (
	"fmt"
	"testing"
)

func TestOverviewCapsActivityCountWithoutLosingLatestActivity(t *testing.T) {
	for _, count := range []int{0, 1, 99, 100, 101, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			store, db := auditRetentionStore(t)
			if _, err := db.Exec(`WITH RECURSIVE n(v) AS (SELECT 1 WHERE ?>0 UNION ALL SELECT v+1 FROM n WHERE v<?)
				INSERT INTO runtime_events(source_id,event_type,created_at,status,summary)
				SELECT -v,'fixture','2026-09-18T00:00:00Z','succeeded','fixture' FROM n`, count, count); err != nil {
				t.Fatal(err)
			}
			summary, err := store.OverviewSummary(t.Context())
			if err != nil || summary.Runs != min(count, 100) {
				t.Fatalf("count=%d summary=%+v err=%v", count, summary, err)
			}
			if count == 0 && summary.LastActivity != nil {
				t.Fatal("empty history has last activity")
			}
			if count > 0 && (summary.LastActivity == nil || *summary.LastActivity != "2026-09-18T00:00:00Z") {
				t.Fatal("latest activity missing")
			}
		})
	}
}
