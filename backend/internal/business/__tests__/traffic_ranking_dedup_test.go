package business_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestTrafficRankingDuplicateRequestsKeepFirstEvidenceWithinWindow(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','账号甲','{}','now'),('42','账号乙','{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','selected'),('42','selected');
		INSERT INTO usage_records(request_id,account_id,group_name,is_error,first_token_ms,observed_at,source,payload_json) VALUES
		('shared','41','old',1,'900','2026-08-01T12:00:00.000000000Z','traffic','{"input_tokens":900}'),
		('shared','41','probe',1,'800','2026-09-14T11:00:00.000000000Z','active-probe','{}'),
		('shared','41','later',1,'700','2026-09-14T13:00:00.000000000Z','traffic','{"input_tokens":700}'),
		('shared','41','first',0,'100','2026-09-14T12:00:00.000000000Z','TRAFFIC','{"input_tokens":10}'),
		('shared','41','same-time',1,'600','2026-09-14T12:00:00.000000000Z','traffic','{"input_tokens":600}'),
		('shared','42','first',0,'200','2026-09-14T12:00:00.000000000Z','traffic','{"input_tokens":20}');`); err != nil {
		t.Fatal(err)
	}
	for _, days := range []int{1, 8, 30} {
		for _, group := range []string{"", "selected"} {
			end := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
			result, err := store.TrafficRanking(context.Background(), business.TrafficRankingQuery{
				StartAt: end.Add(-time.Duration(days) * 24 * time.Hour), EndAt: end, GroupName: group,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.TotalRequests != 2 || len(result.Accounts) != 2 {
				t.Fatalf("%d day / %q ranking = %#v; want two account-scoped requests", days, group, result)
			}
			for index, row := range result.Accounts {
				wantInput := int64((index + 1) * 10)
				if row.Successful != 1 || row.Failed != 0 || row.InputTokens == nil || *row.InputTokens != wantInput ||
					row.AverageLatency == nil || *row.AverageLatency != float64(wantInput*10) ||
					row.LatestAt == nil || *row.LatestAt != "2026-09-14T12:00:00Z" {
					t.Fatalf("%d day / %q first evidence = %#v", days, group, row)
				}
			}
		}
	}
}
