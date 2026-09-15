package business_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestTrafficRankingSelectedGroupUsesCurrentMembershipAndDeduplicatesSharedRequests(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
        ('41','选中账号','{}','now'),('42','其他账号','{}','now');
        INSERT INTO account_groups(account_id,group_name) VALUES('41','selected'),('41','secondary'),('42','other');
        INSERT INTO usage_records(request_id,account_id,group_name,is_error,observed_at,source,payload_json) VALUES
        ('shared','41','previous-group',0,'2026-09-14T12:00:00.000000000Z','traffic','{}'),
        ('shared','41','secondary',0,'2026-09-14T12:00:00.000000000Z','traffic','{}'),
        ('other','42','selected',0,'2026-09-14T12:00:00.000000000Z','traffic','{}')`); err != nil {
		t.Fatal(err)
	}
	query := business.TrafficRankingQuery{
		StartAt: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		EndAt:   time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), GroupName: "selected",
	}
	result, err := store.TrafficRanking(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 1 || len(result.Accounts) != 1 || result.Accounts[0].AccountID != "41" {
		t.Fatalf("ranking did not preserve current membership and request identity: %#v", result)
	}
	query.GroupName = "missing"
	empty, err := store.TrafficRanking(context.Background(), query)
	if err != nil || empty.TotalRequests != 0 || len(empty.Accounts) != 0 {
		t.Fatalf("unknown group ranking = %#v, %v; want an empty result", empty, err)
	}
}

func BenchmarkTrafficRankingSelectedGroup(b *testing.B) {
	store, db := openTrafficRankingStore(b)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','选中账号','{}','now'),('42','其他账号','{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','selected'),('42','other');
		WITH RECURSIVE sequence(value) AS (VALUES(1) UNION ALL SELECT value+1 FROM sequence WHERE value<20000)
		INSERT INTO usage_records(request_id,account_id,is_error,first_token_ms,observed_at,source,payload_json)
		SELECT value,CASE WHEN value<=100 THEN '41' ELSE '42' END,0,'100',
		'2026-09-14T12:00:00.000000000Z','traffic','{}' FROM sequence`); err != nil {
		b.Fatal(err)
	}
	query := business.TrafficRankingQuery{
		StartAt: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		EndAt:   time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), GroupName: "selected",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result, err := store.TrafficRanking(context.Background(), query)
		if err != nil || result.TotalRequests != 100 {
			b.Fatalf("selected group ranking = %d requests, %v", result.TotalRequests, err)
		}
	}
}
