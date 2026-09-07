package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
)

func TestCrossGroupTrafficResamplingPreservesKnownLatency(t *testing.T) {
	for _, tc := range []struct {
		name         string
		oldToken     any
		newToken     any
		oldDuration  string
		newDuration  string
		wantToken    string
		wantDuration string
	}{
		{name: "missing first token retains known value", oldToken: "1250", oldDuration: "30000", newDuration: "30000", wantToken: "1250", wantDuration: "30000"},
		{name: "null first token retains known value", oldToken: "1250", newToken: nil, oldDuration: "30000", wantToken: "1250", wantDuration: "30000"},
		{name: "first token enrichment retains total duration", newToken: "1250", oldDuration: "30000", wantToken: "1250", wantDuration: "30000"},
		{name: "new total duration retains old first token", oldToken: "1250", newDuration: "30000", wantToken: "1250", wantDuration: "30000"},
		{name: "new valid latency takes precedence", oldToken: "1250", newToken: "1500", oldDuration: "30000", newDuration: "31000", wantToken: "1500", wantDuration: "31000"},
		{name: "new zero first token retains old positive value", oldToken: "1250", newToken: "0", oldDuration: "30000", newDuration: "30000", wantToken: "1250", wantDuration: "30000"},
		{name: "old zero first token is not copied", oldToken: "0", oldDuration: "30000", newDuration: "30000", wantDuration: "30000"},
		{name: "empty first token retains known value", oldToken: "1250", newToken: "", oldDuration: "30000", newDuration: "30000", wantToken: "1250", wantDuration: "30000"},
		{name: "invalid old first token is not copied", oldToken: "invalid", newDuration: "30000", wantDuration: "30000"},
		{name: "absent latency stays absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openPolicyStore(t)
			ctx := context.Background()
			old := groupLatencySample("codex", tc.oldToken, tc.oldDuration)
			current := groupLatencySample("pro", tc.newToken, tc.newDuration)
			if tc.name == "null first token retains known value" {
				current.Payload["first_token_ms"] = nil
			}
			if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{old}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{current}); err != nil {
				t.Fatal(err)
			}

			var count int
			if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM health_samples
				WHERE account_id='41' AND source='traffic' AND evidence_key='same-request'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("sample count=%d, want 1", count)
			}
			var group, raw string
			var p50, p95, p99 sql.NullString
			if err := store.db.QueryRowContext(ctx, `SELECT group_name,latency_p50,latency_p95,latency_p99,payload_json
				FROM health_samples WHERE account_id='41' AND source='traffic' AND evidence_key='same-request'`).Scan(&group, &p50, &p95, &p99, &raw); err != nil {
				t.Fatal(err)
			}
			if group != "pro" {
				t.Fatalf("retained group=%q, want pro", group)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				t.Fatal(err)
			}
			if tc.wantToken != "" && payload["first_token_ms"] != tc.wantToken {
				t.Errorf("first_token_ms=%v, want %s", payload["first_token_ms"], tc.wantToken)
			}
			if tc.wantToken == "" && payload["first_token_ms"] != nil {
				t.Errorf("unexpected first_token_ms=%v", payload["first_token_ms"])
			}
			if tc.wantDuration != "" {
				if payload["duration_ms"] != tc.wantDuration || payload["latency_metric"] != "request_duration" {
					t.Errorf("total duration lost: %s", raw)
				}
				for _, latency := range []sql.NullString{p50, p95, p99} {
					if !latency.Valid || latency.String != tc.wantDuration {
						t.Errorf("aggregate=%v, want total duration %s", latency, tc.wantDuration)
					}
				}
			} else if payload["duration_ms"] != nil || p95.Valid {
				t.Errorf("unexpected latency: p95=%v payload=%s", p95, raw)
			}
		})
	}
}

func TestCrossGroupTrafficResamplingPreservesLegacyFirstTokenForRouting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		metric string
		unit   string
		source string
		p95    string
		want   string
	}{
		{"first token milliseconds", "first_token", "ms", "operations.first_token_ms", "1250", "1250"},
		{"ttfb milliseconds", "ttfb", "ms", "operations.first_token_ms", "1250", "1250"},
		{"ttfb seconds", "ttfb", "s", "operations.first_token_ms", "1.25", "1250"},
		{"duration source is not first token", "ttfb", "ms", "operations.duration_ms", "30000", ""},
		{"zero is not first token", "first_token", "ms", "operations.first_token_ms", "0", ""},
		{"unknown unit is not first token", "first_token", "minutes", "operations.first_token_ms", "1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openPolicyStore(t)
			ctx := context.Background()
			old := groupLatencySample("codex", nil, "")
			old.LatencyP95 = &tc.p95
			old.Payload = map[string]any{"latency_metric": tc.metric, "latency_unit": tc.unit, "latency_source": tc.source}
			if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{old}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{groupLatencySample("pro", nil, "30000")}); err != nil {
				t.Fatal(err)
			}
			accountID := "41"
			samples, err := store.RoutingSamples(ctx, &accountID, nil, "traffic", 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(samples) != 1 {
				t.Fatalf("routing sample count=%d, want 1", len(samples))
			}
			if tc.want != "" && samples[0].Payload["first_token_ms"] != tc.want {
				t.Errorf("routing first_token_ms=%v, want %s", samples[0].Payload["first_token_ms"], tc.want)
			}
			if tc.want == "" && samples[0].Payload["first_token_ms"] != nil {
				t.Errorf("invalid legacy latency used as first token: %v", samples[0].Payload)
			}
			if samples[0].LatencyP95 == nil || *samples[0].LatencyP95 != "30000" || samples[0].Payload["latency_metric"] != "request_duration" {
				t.Errorf("routing total duration changed: %+v", samples[0])
			}
		})
	}
}

func TestCrossGroupTrafficResamplingMergesOutOfOrderBatchIntoNewestSample(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	newest := groupLatencySample("pro", nil, "")
	newest.ObservedAt = "2026-09-07T00:00:02Z"
	newest.Result = "失败"
	reason := "请求失败"
	newest.FailureReason = &reason
	newest.Payload["status_code"] = 500
	newest.Payload["remote_id"] = json.Number("9007199254740993")
	middle := groupLatencySample("codex", "1250", "")
	middle.ObservedAt = "2026-09-07T00:00:01Z"
	oldest := groupLatencySample("default", "1500", "30000")

	if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{newest, oldest, middle}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM health_samples`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("sample count=%d, want 1", count)
	}
	var group, result, failureReason, firstToken, duration, p95, source, remoteID string
	var status int
	if err := store.db.QueryRowContext(ctx, `SELECT group_name,result,failure_reason,
		json_extract(payload_json,'$.first_token_ms'),json_extract(payload_json,'$.duration_ms'),latency_p95,
		json_extract(payload_json,'$.first_token_source'),json_extract(payload_json,'$.status_code'),
		json_extract(payload_json,'$.remote_id') FROM health_samples`).Scan(
		&group, &result, &failureReason, &firstToken, &duration, &p95, &source, &status, &remoteID); err != nil {
		t.Fatal(err)
	}
	if group != "pro" || result != "失败" || failureReason != reason || status != 500 || remoteID != "9007199254740993" {
		t.Errorf("newest sample changed: group=%s result=%s reason=%s status=%d remoteID=%s", group, result, failureReason, status, remoteID)
	}
	if firstToken != "1250" || duration != "30000" || p95 != "30000" || source != "operations.first_token_ms" {
		t.Errorf("latency merge: first=%s duration=%s p95=%s source=%s", firstToken, duration, p95, source)
	}
}

func TestCrossGroupTrafficResamplingKeepsStableIdentityIsolated(t *testing.T) {
	for _, tc := range []struct {
		name      string
		accountID string
		source    string
		key       string
	}{
		{"different account", "42", "traffic", "same-request"},
		{"different source", "41", "active-probe", "same-request"},
		{"different request", "41", "traffic", "other-request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openPolicyStore(t)
			ctx := context.Background()
			if _, err := store.db.ExecContext(ctx, `INSERT INTO health_samples
				(account_id,group_name,source,evidence_key,observed_at,payload_json)
				VALUES(?,'codex',?,?,'2026-09-07T00:00:00.000000000Z','{"first_token_ms":"1250","duration_ms":"30000"}')`,
				tc.accountID, tc.source, tc.key); err != nil {
				t.Fatal(err)
			}
			if _, err := store.PersistTrafficSamples(ctx, []TrafficSample{groupLatencySample("pro", nil, "")}); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM health_samples`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("distinct stable identities collapsed: count=%d", count)
			}
			var firstToken, duration sql.NullString
			if err := store.db.QueryRowContext(ctx, `SELECT json_extract(payload_json,'$.first_token_ms'),json_extract(payload_json,'$.duration_ms')
				FROM health_samples WHERE account_id='41' AND source='traffic' AND evidence_key='same-request'`).Scan(&firstToken, &duration); err != nil {
				t.Fatal(err)
			}
			if firstToken.Valid || duration.Valid {
				t.Fatalf("latency crossed stable identity: first=%v duration=%v", firstToken, duration)
			}
		})
	}
}

func groupLatencySample(group string, firstToken any, duration string) TrafficSample {
	sample := TrafficSample{
		AccountID: "41", GroupName: group, Result: "通过", SampleCount: 1, Attempts: 1,
		ObservedAt: "2026-09-07T00:00:00Z", EvidenceKey: "same-request", Payload: map[string]any{},
	}
	if firstToken != nil {
		sample.Payload["first_token_ms"] = firstToken
		sample.Payload["first_token_unit"] = "ms"
		sample.Payload["first_token_source"] = "operations.first_token_ms"
		if value, ok := firstToken.(string); ok {
			sample.LatencyP50, sample.LatencyP95, sample.LatencyP99 = &value, &value, &value
			sample.Payload["latency_metric"] = "first_token"
		}
	}
	if duration != "" {
		sample.LatencyP50, sample.LatencyP95, sample.LatencyP99 = &duration, &duration, &duration
		sample.Payload["duration_ms"] = duration
		sample.Payload["duration_unit"] = "ms"
		sample.Payload["latency_metric"] = "request_duration"
		sample.Payload["latency_source"] = "operations.duration_ms"
		sample.Payload["latency_unit"] = "ms"
	}
	return sample
}
