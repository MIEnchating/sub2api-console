package business

import (
	"context"
	"testing"
	"time"
)

func TestTrafficRankingIncludesSubsecondRequestsAtWindowStart(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','account','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{{AccountID: "41", GroupName: "codex", EvidenceKey: "start", Result: "通过", ObservedAt: start.Add(100 * time.Millisecond).Format(time.RFC3339Nano)}}); err != nil {
		t.Fatal(err)
	}
	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{StartAt: start, EndAt: start.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 1 {
		t.Fatalf("window lost subsecond request: %#v", result)
	}
}

func TestTrafficRankingFiltersNanosecondBoundaryBeforeDeduplicatingRequest(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 1, 12, 0, 0, 100, time.UTC)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','account','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{
		{AccountID: "41", GroupName: "codex", EvidenceKey: "shared", Result: "失败", ObservedAt: start.Add(-time.Nanosecond).Format(time.RFC3339Nano)},
		{AccountID: "41", GroupName: "codex", EvidenceKey: "shared", Result: "通过", ObservedAt: start.Add(time.Nanosecond).Format(time.RFC3339Nano)},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := store.TrafficRanking(ctx, TrafficRankingQuery{StartAt: start, EndAt: start.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 1 || result.Accounts[0].Successful != 1 {
		t.Fatalf("boundary sample hid the in-window request: %#v", result)
	}
}
