package business_test

import (
	"context"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"testing"
	"time"
)

func TestStabilityRetains30DaysBeyondHealthWindowAndDeduplicatesGroups(t *testing.T) {
	store, _ := monitoringProjectionStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	samples := []business.TrafficSample{}
	for i := 0; i < 205; i++ {
		samples = append(samples, business.TrafficSample{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: now.Add(-time.Duration(i+1) * time.Minute).Format(time.RFC3339Nano), EvidenceKey: fmt.Sprintf("request-%d", i), Payload: map[string]any{}})
	}
	duplicate := samples[0]
	duplicate.GroupName = "b"
	samples = append(samples, duplicate)
	samples = append(samples, business.TrafficSample{AccountID: "41", GroupName: "a", Result: "失败", ObservedAt: now.Add(-48 * time.Hour).Format(time.RFC3339Nano), EvidenceKey: "older-failure", Payload: map[string]any{}})
	if _, err := store.PersistTrafficSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistProbeSamples(ctx, []business.ProbeSample{{AccountID: "41", GroupName: "a", Result: "失败", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), RequestModel: "model", SampleCount: 1}}); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	stats := account.Stability
	if stats == nil || stats.Short.Samples != 206 || stats.Short.Passed != 205 || stats.Long.Samples != 207 || stats.Long.Failed != 2 {
		t.Fatalf("stats: %+v", stats)
	}
	if *stats.Short.Score != 99.5 || *stats.Long.Score != 99 {
		t.Fatalf("scores: %+v", stats)
	}
}

func TestStabilityUsageEnrichmentRevisesExistingEvidenceWithoutCountingAgain(t *testing.T) {
	store, _ := monitoringProjectionStore(t)
	ctx := context.Background()
	sample := business.TrafficSample{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), EvidenceKey: "enriched", Payload: map[string]any{}}
	if _, err := store.PersistTrafficSamples(ctx, []business.TrafficSample{sample}); err != nil {
		t.Fatal(err)
	}
	sample.Payload = map[string]any{"token_usage": map[string]any{"input_tokens": 0, "output_tokens": 0}}
	if _, err := store.PersistTrafficSamples(ctx, []business.TrafficSample{sample}); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Stability.Short.Samples != 1 || account.Stability.Short.Failed != 1 || *account.Stability.Short.Score != 0 {
		t.Fatalf("stats: %+v", account.Stability)
	}
	duplicate := sample
	duplicate.GroupName = "b"
	duplicate.Payload = map[string]any{}
	if _, err := store.PersistTrafficSamples(ctx, []business.TrafficSample{duplicate}); err != nil {
		t.Fatal(err)
	}
	account, err = store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Stability.Short.Failed != 1 {
		t.Fatal("another group without usage erased the confirmed empty response")
	}
	sample.Payload = map[string]any{"token_usage": map[string]any{"input_tokens": 10, "output_tokens": 20}}
	if _, err := store.PersistTrafficSamples(ctx, []business.TrafficSample{sample}); err != nil {
		t.Fatal(err)
	}
	account, err = store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Stability.Short.Samples != 1 || account.Stability.Short.Passed != 1 {
		t.Fatalf("stats: %+v", account.Stability)
	}
}

func TestStabilityExcludesFutureExpiredAndOtherAccounts(t *testing.T) {
	store, db := monitoringProjectionStore(t)
	now := time.Now().UTC()
	for i, row := range []struct {
		id string
		at time.Time
	}{{"41", now.Add(-31 * 24 * time.Hour)}, {"41", now.Add(time.Hour)}, {"42", now.Add(-time.Hour)}} {
		if _, err := db.Exec(`INSERT INTO account_stability_samples(account_id,source,evidence_key,observed_at,outcome) VALUES(?,'traffic',?,?,'passed')`, row.id, fmt.Sprint(i), row.at.Format("2006-01-02T15:04:05.000000000Z")); err != nil {
			t.Fatal(err)
		}
	}
	account, err := store.Account(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Stability.Long.Samples != 0 || account.Stability.Long.Score != nil {
		t.Fatalf("stats: %+v", account.Stability)
	}
}
