package business

import (
	"context"
	"testing"
	"time"
)

func TestTrafficRankingPartialUsageKeepsUnreportedFieldsUnknown(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES ('41','partial','{}','now');
		INSERT INTO usage_records(request_id,account_id,is_error,observed_at,source,payload_json)
		VALUES ('partial','41',0,?,'traffic','{"input_tokens":0,"cache_read_input_tokens":4}')`, end.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{StartAt: end.Add(-2 * time.Hour), EndAt: end})
	if err != nil {
		t.Fatal(err)
	}
	row := result.Accounts[0]
	if row.OutputTokens != nil || row.CacheWriteTokens != nil {
		t.Fatalf("unreported fields must remain unknown: %#v", row)
	}
	if !row.UsageAvailable || row.InputTokens == nil || *row.InputTokens != 0 || row.CacheReadTokens == nil || *row.CacheReadTokens != 4 {
		t.Fatalf("reported zero and cache must be retained: %#v", row)
	}
}
