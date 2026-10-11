package business

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestTrafficRankingUsesRealTrafficAndConfidenceAdjustedStability(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,upstream_host,upstream_type,metadata_json,updated_at) VALUES
		('41','稳定账号','alpha.example','sub2api','{"platform":"anthropic"}','now'),
		('42','低样本账号','beta.example','sub2api','{"platform":"openai"}','now'),
		('43','无流量账号','gamma.example','newapi','{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','codex'),('42','codex'),('43','other')`); err != nil {
		t.Fatal(err)
	}
	for index := range 10 {
		isError := 0
		var reason any
		if index == 9 {
			isError, reason = 1, "HTTP 502"
		}
		observedAt := end.Add(-time.Duration(index+1) * time.Hour).Format(time.RFC3339Nano)
		if _, err := store.db.ExecContext(ctx, `INSERT INTO usage_records(
			request_id,account_id,account_name,group_name,is_error,error_reason,first_token_ms,observed_at,source,payload_json
		) VALUES(?,'41','稳定账号','codex',?,?,?,?, 'traffic','{}')`, fmt.Sprintf("stable-%d", index), isError, reason, fmt.Sprintf("%d", 100+index*10), observedAt); err != nil {
			t.Fatal(err)
		}
	}
	for index := range 2 {
		observedAt := end.Add(-time.Duration(index+1) * time.Hour).Format(time.RFC3339Nano)
		if _, err := store.db.ExecContext(ctx, `INSERT INTO usage_records(
			request_id,account_id,account_name,group_name,is_error,first_token_ms,observed_at,source,payload_json
		) VALUES(?,'42','低样本账号','codex',0,'80',?,'traffic','{}')`, fmt.Sprintf("small-%d", index), observedAt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO usage_records(
		request_id,account_id,account_name,group_name,is_error,first_token_ms,observed_at,source,payload_json
	) VALUES
		('probe','42','低样本账号','codex',1,'70',?,'active-probe','{}'),
		('old','42','低样本账号','codex',1,'70',?,'traffic','{}')`,
		end.Add(-time.Hour).Format(time.RFC3339Nano), end.Add(-25*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{
		StartAt: end.Add(-24 * time.Hour), EndAt: end, SortBy: TrafficRankingSortStability,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 12 || result.AccountsWithTraffic != 2 || len(result.Accounts) != 3 {
		t.Fatalf("summary=%#v", result)
	}
	stable := result.Accounts[0]
	if stable.AccountID != "41" || stable.Requests != 10 || stable.Successful != 9 || stable.Failed != 1 || stable.Platform != "anthropic" {
		t.Fatalf("stable row=%#v", stable)
	}
	if stable.SuccessRate == nil || math.Abs(*stable.SuccessRate-90) > 0.01 || stable.StabilityScore == nil || *stable.StabilityScore <= 50 {
		t.Fatalf("stable rates=%#v", stable)
	}
	if result.Accounts[1].AccountID != "42" || result.Accounts[1].StabilityScore == nil || *stable.StabilityScore <= *result.Accounts[1].StabilityScore {
		t.Fatalf("confidence ranking=%#v", result.Accounts)
	}
	if result.Accounts[2].AccountID != "43" || result.Accounts[2].Requests != 0 || result.Accounts[2].SuccessRate != nil {
		t.Fatalf("zero traffic row=%#v", result.Accounts[2])
	}
	if stable.TrafficShare == nil || math.Abs(*stable.TrafficShare-83.33) > 0.01 || stable.ActiveBuckets != 10 || stable.TotalBuckets != 24 {
		t.Fatalf("traffic coverage=%#v", stable)
	}
}

func TestTrafficRankingFiltersCurrentAccountGroupAndSortsByLatency(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','slow','{}','now'),('42','fast','{}','now'),('43','other','{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','codex'),('42','codex'),('43','other');
		INSERT INTO usage_records(request_id,account_id,account_name,group_name,is_error,first_token_ms,observed_at,source,payload_json) VALUES
		('slow','41','slow','codex',0,'500',?,'traffic','{}'),
		('fast','42','fast','codex',0,'100',?,'traffic','{}'),
		('other','43','other','other',0,'50',?,'traffic','{}')`,
		end.Add(-time.Hour).Format(time.RFC3339Nano), end.Add(-time.Hour).Format(time.RFC3339Nano), end.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{
		StartAt: end.Add(-6 * time.Hour), EndAt: end, GroupName: "codex", SortBy: TrafficRankingSortLatency,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 2 || result.Accounts[0].AccountID != "42" || result.Accounts[1].AccountID != "41" {
		t.Fatalf("filtered latency ranking=%#v", result.Accounts)
	}
}

func TestTrafficRankingSelectedGroupIgnoresInvalidMetadataOutsideScope(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','selected','{"platform":"openai"}','now'),('42','outside','not-json','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','selected'),('42','other');
		INSERT INTO usage_records(request_id,account_id,group_name,is_error,observed_at,source,payload_json)
		VALUES('selected-request','41','selected',0,?,'traffic','{}')`, end.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{
		StartAt: end.Add(-6 * time.Hour), EndAt: end, GroupName: "selected",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 1 || len(result.Accounts) != 1 || result.Accounts[0].AccountID != "41" {
		t.Fatalf("selected group ranking = %#v", result)
	}
}

func TestTrafficRankingRejectsInvalidMetadataInSelectedScope(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','selected','not-json','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','selected')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TrafficRanking(ctx, TrafficRankingQuery{
		StartAt: end.Add(-6 * time.Hour), EndAt: end, GroupName: "selected",
	}); err == nil || !strings.Contains(err.Error(), "元数据记录损坏") {
		t.Fatalf("invalid selected metadata was accepted: %v", err)
	}
}

func TestTrafficRankingPrefersRequestDurationOverFirstToken(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','account','{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','codex');
		INSERT INTO usage_records(request_id,account_id,account_name,group_name,is_error,first_token_ms,observed_at,source,payload_json)
		VALUES('request','41','account','codex',0,'1250',?,'traffic','{"duration_ms":"6800","duration_unit":"ms"}')`,
		end.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{
		StartAt: end.Add(-6 * time.Hour), EndAt: end, SortBy: TrafficRankingSortLatency,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 1 || result.Accounts[0].AverageLatency == nil || *result.Accounts[0].AverageLatency != 6800 ||
		result.Accounts[0].P95Latency == nil || *result.Accounts[0].P95Latency != 6800 {
		t.Fatalf("ranking=%#v", result.Accounts)
	}
}

func TestTrafficRankingCountsSharedRequestOnlyOnceAcrossGroups(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','shared','{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','codex'),('41','pro');
		INSERT INTO usage_records(request_id,account_id,account_name,group_name,is_error,observed_at,source,payload_json) VALUES
		('same-request','41','shared','codex',0,?,'traffic','{}'),
		('same-request','41','shared','pro',0,?,'traffic','{}')`,
		end.Add(-time.Hour).Format(time.RFC3339Nano), end.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{
		StartAt: end.Add(-6 * time.Hour), EndAt: end, SortBy: TrafficRankingSortTraffic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 1 || len(result.Accounts) != 1 || result.Accounts[0].Requests != 1 {
		t.Fatalf("deduplicated ranking=%#v", result)
	}
}

func TestTrafficRankingAggregatesTokenUsageAndDistinguishesMissingFromZero(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','with usage','{}','now'),('42','without usage','{}','now');
		INSERT INTO usage_records(request_id,account_id,account_name,is_error,observed_at,source,payload_json) VALUES
		('one','41','with usage',0,?,'traffic','{"token_usage":{"input_tokens":0,"output_tokens":12,"cache_read_tokens":4,"cache_creation_tokens":1}}'),
		('two','41','with usage',0,?,'traffic','{"extra":{"token_usage":{"prompt_tokens":3,"completion_tokens":0}}}'),
		('three','42','without usage',0,?,'traffic','{}')`,
		end.Add(-3*time.Hour).Format(time.RFC3339Nano), end.Add(-2*time.Hour).Format(time.RFC3339Nano), end.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{StartAt: end.Add(-6 * time.Hour), EndAt: end})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]TrafficRankingRow{}
	for _, row := range result.Accounts {
		rows[row.AccountID] = row
	}
	withUsage := rows["41"]
	if !withUsage.UsageAvailable || withUsage.InputTokens == nil || *withUsage.InputTokens != 3 || withUsage.OutputTokens == nil || *withUsage.OutputTokens != 12 || withUsage.CacheReadTokens == nil || *withUsage.CacheReadTokens != 4 || withUsage.CacheWriteTokens == nil || *withUsage.CacheWriteTokens != 1 {
		t.Fatalf("with usage=%#v", withUsage)
	}
	withoutUsage := rows["42"]
	if withoutUsage.UsageAvailable || withoutUsage.InputTokens != nil || withoutUsage.OutputTokens != nil || withoutUsage.CacheReadTokens != nil || withoutUsage.CacheWriteTokens != nil {
		t.Fatalf("without usage=%#v", withoutUsage)
	}
}

func TestTrafficRankingAverageIncludesRecordsBeyondLatencySampleLimit(t *testing.T) {
	t.Parallel()
	store := openPolicyStore(t)
	ctx := context.Background()
	end := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','sampled','{}','now')`); err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `WITH RECURSIVE requests(index_number) AS (
		SELECT 0 UNION ALL SELECT index_number + 1 FROM requests WHERE index_number < ?
	) INSERT INTO usage_records(
		request_id,account_id,account_name,is_error,first_token_ms,observed_at,source,payload_json
	) SELECT printf('request-%05d', index_number), '41', 'sampled', 0,
		CASE WHEN index_number = ? THEN '10001' ELSE '1' END, ?, 'traffic', '{}'
	FROM requests`, trafficRankingLatencySampleLimit, trafficRankingLatencySampleLimit,
		end.Add(-time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{
		StartAt: end.Add(-6 * time.Hour), EndAt: end, SortBy: TrafficRankingSortLatency,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 1 || result.Accounts[0].AverageLatency == nil || *result.Accounts[0].AverageLatency != 2 {
		t.Fatalf("ranking=%#v", result.Accounts)
	}
}

func TestTrafficRankingWindowQueryUsesBoundedWindowIndex(t *testing.T) {
	store := openPolicyStore(t)
	rows, err := store.db.Query("EXPLAIN QUERY PLAN "+trafficRankingWindowQuery,
		"2026-08-01T00:00:00.000000000Z", "2026-08-02T00:00:00.000000000Z")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "ix_usage_records_traffic_window") || !strings.Contains(plan, "observed_at>?") || !strings.Contains(plan, "observed_at<?") {
		t.Fatalf("traffic window index was not used:\n%s", plan)
	}
	if strings.Contains(plan, "USE TEMP B-TREE") {
		t.Fatalf("traffic window scan sorted records outside the index:\n%s", plan)
	}
}

func TestTrafficRankingLargeWindowDedupUsesBoundedWindowIndex(t *testing.T) {
	store := openPolicyStore(t)
	rows, err := store.db.Query("EXPLAIN QUERY PLAN "+trafficRankingLargeWindowCandidates+trafficRankingLargeWindowResults,
		"2026-08-01T00:00:00.000000000Z", "2026-09-01T00:00:00.000000000Z")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "ix_usage_records_traffic_window") {
		t.Fatalf("large traffic window scan did not use bounded index:\n%s", plan)
	}
}

func TestTrafficRankingRejectsInvalidRangeAndSort(t *testing.T) {
	store := openPolicyStore(t)
	now := time.Now().UTC()
	for name, query := range map[string]TrafficRankingQuery{
		"reversed": {StartAt: now, EndAt: now.Add(-time.Hour), SortBy: TrafficRankingSortTraffic},
		"too long": {StartAt: now.Add(-31 * 24 * time.Hour), EndAt: now, SortBy: TrafficRankingSortTraffic},
		"sort":     {StartAt: now.Add(-time.Hour), EndAt: now, SortBy: "unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := store.TrafficRanking(context.Background(), query); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
