package business_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// A thirty-day report must not sort the full upstream payload for each duplicate.
// Fixtures use a private temporary database and retain unrelated payload fields.
func BenchmarkTrafficRankingLargeWindowPayload(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("requests-%d", count), func(b *testing.B) {
			store, db := openTrafficRankingStore(b)
			if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at)
				VALUES('41','月度流量账号','{}','now')`); err != nil {
				b.Fatal(err)
			}
			payload := `{"input_tokens":10,"output_tokens":20,"duration_ms":100,"detail":"` + strings.Repeat("x", 4096) + `"}`
			if _, err := db.Exec(`WITH RECURSIVE sequence(value) AS
				(VALUES(1) UNION ALL SELECT value+1 FROM sequence WHERE value<?)
				INSERT INTO usage_records(request_id,account_id,group_name,is_error,first_token_ms,observed_at,source,payload_json)
				SELECT printf('request-%012d',value),'41',group_name,0,'50',
				'2026-09-14T12:00:00.000000000Z','traffic',?
				FROM sequence CROSS JOIN (SELECT 'primary' AS group_name UNION ALL SELECT 'secondary')`, count, payload); err != nil {
				b.Fatal(err)
			}
			end := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
			query := business.TrafficRankingQuery{StartAt: end.Add(-30 * 24 * time.Hour), EndAt: end}
			b.ReportAllocs()
			for b.Loop() {
				result, err := store.TrafficRanking(context.Background(), query)
				if err != nil || result.TotalRequests != count || len(result.Accounts) != 1 {
					b.Fatalf("monthly ranking = %#v, %v", result, err)
				}
				row := result.Accounts[0]
				if row.InputTokens == nil || *row.InputTokens != int64(count*10) || row.AverageLatency == nil || *row.AverageLatency != 100 {
					b.Fatal("monthly ranking lost first evidence payload")
				}
			}
		})
	}
}
