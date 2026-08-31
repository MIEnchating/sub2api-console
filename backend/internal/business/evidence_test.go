package business

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestEvidenceTargetsAndTrafficPersistence(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,upstream_type,routing_state,metadata_json,updated_at)
		VALUES('41','demo','newapi','healthy','{"account_type":"oauth","platform":"openai"}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','6')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,schedulable,role,routing_state,updated_at,payload_json)
		VALUES('41','codex',0,'fused',NULL,'now','{}')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	latency := "125"
	inserted, err := store.PersistTrafficSamples(ctx, []TrafficSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", LatencyP50: &latency, LatencyP95: &latency,
		LatencyP99: &latency, SampleCount: 1, Attempts: 1, ObservedAt: now, EvidenceKey: "request-1",
		Payload: map[string]any{"request_id": "request-1", "latency_unit": "ms"},
	}})
	if err != nil || inserted != 1 {
		t.Fatalf("inserted=%d err=%v", inserted, err)
	}
	inserted, err = store.PersistTrafficSamples(ctx, []TrafficSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: now,
		EvidenceKey: "request-1", Payload: map[string]any{},
	}})
	if err != nil || inserted != 0 {
		t.Fatalf("duplicate inserted=%d err=%v", inserted, err)
	}
	var usageCount, usageError int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(is_error),0) FROM usage_records
		WHERE account_id='41' AND request_id='request-1'`).Scan(&usageCount, &usageError); err != nil {
		t.Fatal(err)
	}
	if usageCount != 1 || usageError != 0 {
		t.Fatalf("traffic usage record count=%d error=%d", usageCount, usageError)
	}
	targets, err := store.EvidenceTargets(ctx, nil, nil)
	if err != nil || len(targets) != 1 || targets[0].TrafficAt == nil || targets[0].ProbeAt != nil ||
		targets[0].EffectiveState != "healthy" || targets[0].DecisionState == nil || *targets[0].DecisionState != "fused" ||
		targets[0].AccountType != "oauth" || targets[0].Platform != "openai" {
		t.Fatalf("targets=%#v err=%v", targets, err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at)
		VALUES('routing-decision-epoch','{}','2999-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	targets, err = store.EvidenceTargets(ctx, nil, nil)
	if err != nil || len(targets) != 1 || targets[0].DecisionState != nil || targets[0].EffectiveState != "healthy" {
		t.Fatalf("previous-mode decision leaked into evidence plan: targets=%#v err=%v", targets, err)
	}
	previous, err := store.PreviousRoutingDecisions(ctx, nil, nil)
	if err != nil || len(previous) != 0 {
		t.Fatalf("previous-mode decision leaked into routing engine: previous=%#v err=%v", previous, err)
	}
}

func TestEvidenceTargetsExcludeManualPriorityAccounts(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','manual','{}','now'),('42','automatic','{}','now');
		INSERT INTO account_groups(account_id,group_name) VALUES('41','codex'),('42','codex');
		INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('41',3,'now','now')`); err != nil {
		t.Fatal(err)
	}

	targets, err := store.EvidenceTargets(ctx, nil, nil)
	if err != nil || len(targets) != 1 || targets[0].AccountID != "42" {
		t.Fatalf("manual priority account entered evidence targets: targets=%#v err=%v", targets, err)
	}
}

func TestPersistTrafficSamplesRejectsNonFiniteJSON(t *testing.T) {
	store := openPolicyStore(t)
	_, err := store.PersistTrafficSamples(context.Background(), []TrafficSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
		EvidenceKey: "bad", Payload: map[string]any{"bad": math.Inf(1)},
	}})
	if err == nil {
		t.Fatal("non-finite payload must be rejected")
	}
}

func TestPersistTrafficSamplesKeepsThirtyDaysOfUsageForRanking(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','demo','{}','now')`); err != nil {
		t.Fatal(err)
	}
	latest := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	samples := []TrafficSample{
		{AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: latest.Format(time.RFC3339Nano), EvidenceKey: "current", Payload: map[string]any{}},
		{AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: latest.Add(-31 * 24 * time.Hour).Format(time.RFC3339Nano), EvidenceKey: "expired", Payload: map[string]any{}},
	}
	if _, err := store.PersistTrafficSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_records WHERE account_id='41'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retained usage records=%d want=1", count)
	}
}

func TestPersistTrafficFetchesRecordsSuccessfulEmptyPull(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	if err := store.PersistTrafficFetches(ctx, []string{"41"}, now); err != nil {
		t.Fatal(err)
	}
	var updated string
	if err := store.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='evidence-traffic-fetch:41'`).Scan(&updated); err != nil {
		t.Fatal(err)
	}
	if updated != now.Format(time.RFC3339Nano) {
		t.Fatalf("last successful traffic pull=%q", updated)
	}
}

func TestPersistTrafficSamplesEnrichesExistingRequestWithFirstTokenLatency(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	base := TrafficSample{
		AccountID: "41", GroupName: "codex", Result: "通过", SampleCount: 1, Attempts: 1,
		ObservedAt: now, EvidenceKey: "request-1",
		Payload: map[string]any{"request_id": "request-1", "duration_ms": "6800", "duration_unit": "ms"},
	}
	inserted, err := store.PersistTrafficSamples(ctx, []TrafficSample{base})
	if err != nil || inserted != 1 {
		t.Fatalf("initial inserted=%d err=%v", inserted, err)
	}

	latency := "1250"
	enriched := base
	enriched.LatencyP50, enriched.LatencyP95, enriched.LatencyP99 = &latency, &latency, &latency
	enriched.Payload = map[string]any{
		"request_id": "request-1", "duration_ms": "6800", "duration_unit": "ms",
		"latency_metric": "first_token", "latency_unit": "ms",
	}
	inserted, err = store.PersistTrafficSamples(ctx, []TrafficSample{enriched})
	if err != nil || inserted != 1 {
		t.Fatalf("enriched=%d err=%v", inserted, err)
	}

	var p95, payload string
	if err := store.db.QueryRowContext(ctx, `SELECT latency_p95,payload_json FROM health_samples
		WHERE account_id='41' AND group_name='codex' AND source='traffic' AND evidence_key='request-1'`).Scan(&p95, &payload); err != nil {
		t.Fatal(err)
	}
	if p95 != latency || !strings.Contains(payload, `"latency_metric":"first_token"`) {
		t.Fatalf("p95=%q payload=%s", p95, payload)
	}
}

func TestPersistTrafficSamplesUpgradesFirstTokenAggregateToRequestDuration(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	firstToken, duration := "1250", "6800"
	legacy := TrafficSample{
		AccountID: "41", GroupName: "codex", Result: "通过", SampleCount: 1, Attempts: 1,
		ObservedAt: now, EvidenceKey: "request-upgrade",
		LatencyP50: &firstToken, LatencyP95: &firstToken, LatencyP99: &firstToken,
		Payload: map[string]any{
			"request_id": "request-upgrade", "duration_ms": duration, "duration_unit": "ms",
			"latency_metric": "first_token", "latency_source": "operations.first_token_ms", "latency_unit": "ms",
		},
	}
	if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{legacy}); err != nil {
		t.Fatal(err)
	}
	combined := legacy
	combined.LatencyP50, combined.LatencyP95, combined.LatencyP99 = &duration, &duration, &duration
	combined.Payload = map[string]any{
		"request_id": "request-upgrade", "duration_ms": duration, "duration_unit": "ms",
		"first_token_ms": firstToken, "first_token_unit": "ms",
		"latency_metric": "request_duration", "latency_source": "operations.duration_ms", "latency_unit": "ms",
	}
	if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{combined}); err != nil {
		t.Fatal(err)
	}

	var p95, payload string
	if err := store.db.QueryRowContext(ctx, `SELECT latency_p95,payload_json FROM health_samples
		WHERE account_id='41' AND group_name='codex' AND source='traffic' AND evidence_key='request-upgrade'`).Scan(&p95, &payload); err != nil {
		t.Fatal(err)
	}
	if p95 != duration || !strings.Contains(payload, `"latency_metric":"request_duration"`) || !strings.Contains(payload, `"first_token_ms":"1250"`) {
		t.Fatalf("p95=%q payload=%s", p95, payload)
	}
	var storedFirstToken string
	if err := store.db.QueryRowContext(ctx, `SELECT first_token_ms FROM usage_records
		WHERE account_id='41' AND group_name='codex' AND request_id='request-upgrade'`).Scan(&storedFirstToken); err != nil {
		t.Fatal(err)
	}
	if storedFirstToken != firstToken {
		t.Fatalf("usage first token=%q", storedFirstToken)
	}
}

func TestPersistTrafficSamplesKeepsLatestTwoHundredPerAccount(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	samples := make([]TrafficSample, 0, 209)
	for index := range 205 {
		samples = append(samples, TrafficSample{
			AccountID: "41", GroupName: "codex", Result: "通过",
			ObservedAt:  base.Add(time.Duration(index) * time.Second).Format(time.RFC3339Nano),
			EvidenceKey: fmt.Sprintf("request-%03d", index), Payload: map[string]any{},
		})
	}
	for index := range 4 {
		samples = append(samples, TrafficSample{
			AccountID: "42", GroupName: "pro", Result: "通过",
			ObservedAt:  base.Add(time.Duration(index) * time.Second).Format(time.RFC3339Nano),
			EvidenceKey: fmt.Sprintf("other-%03d", index), Payload: map[string]any{},
		})
	}
	if _, err := store.PersistTrafficSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}

	var count41, count42 int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM health_samples WHERE account_id='41'`).Scan(&count41); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM health_samples WHERE account_id='42'`).Scan(&count42); err != nil {
		t.Fatal(err)
	}
	if count41 != retainedHealthSamplesPerAccount || count42 != 4 {
		t.Fatalf("retained samples account 41=%d account 42=%d", count41, count42)
	}
	var oldest, newest string
	if err := store.db.QueryRowContext(ctx, `SELECT MIN(evidence_key),MAX(evidence_key) FROM health_samples WHERE account_id='41'`).Scan(&oldest, &newest); err != nil {
		t.Fatal(err)
	}
	if oldest != "request-005" || newest != "request-204" {
		t.Fatalf("retained range=%s..%s", oldest, newest)
	}
}

func TestPersistTrafficSamplesDeduplicatesEvidenceAcrossGroups(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	inserted, err := store.PersistTrafficSamples(ctx, []TrafficSample{
		{AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: base.Format(time.RFC3339Nano), EvidenceKey: "same-request", Payload: map[string]any{}},
		{AccountID: "41", GroupName: "pro", Result: "通过", ObservedAt: base.Add(time.Second).Format(time.RFC3339Nano), EvidenceKey: "same-request", Payload: map[string]any{}},
	})
	if err != nil || inserted != 2 {
		t.Fatalf("inserted=%d err=%v", inserted, err)
	}
	var count int
	var groupName string
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*),MAX(group_name) FROM health_samples
		WHERE account_id='41' AND source='traffic' AND evidence_key='same-request'`).Scan(&count, &groupName); err != nil {
		t.Fatal(err)
	}
	if count != 1 || groupName != "pro" {
		t.Fatalf("duplicate evidence retained count=%d group=%q", count, groupName)
	}
}

func TestPersistTrafficSamplesRollsBackWholeBatch(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := store.PersistTrafficSamples(ctx, []TrafficSample{
		{AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: now, EvidenceKey: "valid", Payload: map[string]any{}},
		{AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: now, EvidenceKey: "invalid", Payload: map[string]any{"value": math.Inf(1)}},
	})
	if err == nil {
		t.Fatal("invalid batch unexpectedly committed")
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM health_samples`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid batch partially committed: count=%d", count)
	}
}
