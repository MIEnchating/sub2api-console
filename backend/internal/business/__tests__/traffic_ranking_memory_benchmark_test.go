package business_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkTrafficRankingRequestVolume(b *testing.B) {
	for _, count := range []int{1000, 100000} {
		b.Run(fmt.Sprintf("requests-%d", count), func(b *testing.B) {
			store, db := openTrafficRankingStore(b)
			if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at)
				VALUES('41','流量账号','{}','now');`); err != nil {
				b.Fatal(err)
			}
			if _, err := db.Exec(`WITH RECURSIVE sequence(value) AS
				(VALUES(1) UNION ALL SELECT value+1 FROM sequence WHERE value<?)
				INSERT INTO usage_records(request_id,account_id,group_name,is_error,first_token_ms,observed_at,source,payload_json)
				SELECT printf('request-%012d',value),'41',group_name,0,'100',
				'2026-09-14T12:00:00.000000000Z','traffic','{"input_tokens":10,"output_tokens":20}'
				FROM sequence CROSS JOIN (SELECT 'primary' AS group_name UNION ALL SELECT 'secondary')`, count); err != nil {
				b.Fatal(err)
			}
			query := business.TrafficRankingQuery{
				StartAt: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
				EndAt:   time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
			}
			b.ReportAllocs()
			for b.Loop() {
				result, err := store.TrafficRanking(context.Background(), query)
				if err != nil || result.TotalRequests != count || len(result.Accounts) != 1 {
					b.Fatalf("ranking count = %d; want %d, err = %v", result.TotalRequests, count, err)
				}
				if row := result.Accounts[0]; row.InputTokens == nil || *row.InputTokens != int64(count*10) || row.AverageLatency == nil || *row.AverageLatency != 100 {
					b.Fatal("ranking lost deduplicated usage or latency")
				}
			}
		})
	}
}
