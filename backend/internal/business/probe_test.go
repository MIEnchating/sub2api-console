package business

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProbeRepositoryReadsStableCandidatesAndPersistsSamplesAtomically(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "probe.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO accounts(id,name,upstream_type,metadata_json,updated_at) VALUES
		('41','alpha','sub2api','{"platform":"openai"}','now'),
		('42','broken','sub2api','{invalid','now');
		INSERT INTO account_groups(account_id,group_name,group_id,group_rate) VALUES
		('41','codex','7','0.1'),('42','pro','9','0.2')`); err != nil {
		t.Fatal(err)
	}
	candidates, err := store.ProbeCandidates(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].AccountID != "41" || candidates[0].GroupID == nil || *candidates[0].GroupID != "7" || candidates[0].Metadata["platform"] != "openai" {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
	if candidates[1].MetadataErr == nil {
		t.Fatal("malformed account metadata was silently accepted")
	}
	observed := time.Now().UTC().Format(time.RFC3339Nano)
	status := 200
	latency := "12.345"
	count, err := store.PersistProbeSamples(ctx, []ProbeSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", LatencyP50: &latency,
		LatencyP95: &latency, LatencyP99: &latency, SampleCount: 1, Attempts: 1,
		ObservedAt: observed, StatusCode: &status,
	}})
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	var result, payload string
	var sampleCount int
	if err := store.db.QueryRow(`SELECT result,sample_count,payload_json FROM health_samples WHERE account_id='41' AND group_name='codex'`).Scan(&result, &sampleCount, &payload); err != nil {
		t.Fatal(err)
	}
	if result != "通过" || sampleCount != 1 || payload != `{"actual_model":"","request_model":"","status_code":200}` {
		t.Fatalf("result=%q sampleCount=%d payload=%s", result, sampleCount, payload)
	}
	before := 0
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM health_samples`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	_, err = store.PersistProbeSamples(ctx, []ProbeSample{
		{AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{AccountID: "", GroupName: "pro", Result: "失败", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	if err == nil {
		t.Fatal("invalid batch unexpectedly committed")
	}
	after := 0
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM health_samples`).Scan(&after); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("invalid batch partially committed: before=%d after=%d", before, after)
	}
}

func TestProbeRepositoryPersistsModelRewriteEvidence(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "probe-rewrite.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','alpha','{}','now')`); err != nil {
		t.Fatal(err)
	}
	observed := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.PersistProbeSamples(ctx, []ProbeSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", SampleCount: 1, Attempts: 1,
		ObservedAt: observed, RequestModel: "requested-model", ActualModel: "mapped-model",
	}}); err != nil {
		t.Fatal(err)
	}
	var eventType, status, summary, payload string
	if err := store.db.QueryRow(`SELECT event_type,status,summary,payload_json FROM runtime_events WHERE event_type='probe_model_rewritten'`).Scan(&eventType, &status, &summary, &payload); err != nil {
		t.Fatal(err)
	}
	if status != "warning" || !strings.Contains(summary, "requested-model") || !strings.Contains(summary, "mapped-model") ||
		!strings.Contains(payload, `"account_id":"41"`) {
		t.Fatalf("event=%q status=%q summary=%q payload=%s", eventType, status, summary, payload)
	}
}

func TestProbeRepositoryPersistsFailedProbeEvent(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	observed := time.Now().UTC().Format(time.RFC3339Nano)
	reason := "上游返回 401"
	statusCode := 401
	if _, err := store.PersistProbeSamples(ctx, []ProbeSample{{
		AccountID: "41", GroupName: "codex", Result: "失败", Attempts: 1,
		ObservedAt: observed, FailureReason: &reason, StatusCode: &statusCode,
	}}); err != nil {
		t.Fatal(err)
	}
	var status, summary, payload string
	if err := store.db.QueryRow(`SELECT status,summary,payload_json FROM runtime_events WHERE event_type='probe.failed'`).Scan(&status, &summary, &payload); err != nil {
		t.Fatal(err)
	}
	if status != "warning" || !strings.Contains(summary, reason) || !strings.Contains(payload, `"group_name":"codex"`) {
		t.Fatalf("status=%q summary=%q payload=%s", status, summary, payload)
	}
}

func TestPersistProbeSamplesKeepsLatestTwoHundred(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	samples := make([]ProbeSample, 205)
	for index := range samples {
		samples[index] = ProbeSample{
			AccountID: "41", GroupName: "codex", Result: "通过", SampleCount: 1, Attempts: 1,
			ObservedAt: base.Add(time.Duration(index) * time.Second).Format(time.RFC3339Nano),
		}
	}
	if _, err := store.PersistProbeSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	var count int
	var oldest, newest string
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*),MIN(observed_at),MAX(observed_at)
		FROM health_samples WHERE account_id='41'`).Scan(&count, &oldest, &newest); err != nil {
		t.Fatal(err)
	}
	if count != retainedHealthSamplesPerAccount || oldest != samples[5].ObservedAt || newest != samples[204].ObservedAt {
		t.Fatalf("retained count=%d range=%s..%s", count, oldest, newest)
	}
}
