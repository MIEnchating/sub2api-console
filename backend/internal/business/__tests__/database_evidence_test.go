package business_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractTrafficEnrichmentPreservesDurationAndExactUsage(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			defer db.ExecContext(t.Context(), `DELETE FROM health_samples WHERE account_id='901'`)
			defer db.ExecContext(t.Context(), `DELETE FROM usage_records WHERE account_id='901'`)
			defer db.ExecContext(t.Context(), `DELETE FROM account_stability_samples WHERE account_id='901'`)
			duration, first := "30000", "1250"
			sample := business.TrafficSample{AccountID: "901", GroupName: "evidence", EvidenceKey: "request-901", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), LatencyP95: &duration, Payload: map[string]any{"latency_metric": "request_duration", "detail": map[string]any{"keep": true, "remove": "old"}}}
			if count, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{sample}); err != nil || count != 1 {
				t.Fatalf("first observation: %d %v", count, err)
			}
			if count, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{sample}); err != nil || count != 0 {
				t.Fatalf("duplicate observation: %d %v", count, err)
			}
			sample.LatencyP95 = &first
			sample.Payload = map[string]any{"latency_metric": "first_token", "first_token_ms": first, "token_usage": map[string]any{"input_tokens": json.Number("9007199254740993"), "output_tokens": 1}}
			if count, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{sample}); err != nil || count != 1 {
				t.Fatalf("enrichment: %d %v", count, err)
			}
			var latency, raw string
			if err := db.QueryRowContext(t.Context(), `SELECT latency_p95,payload_json FROM health_samples WHERE account_id='901'`).Scan(&latency, &raw); err != nil {
				t.Fatal(err)
			}
			var got struct {
				Metric string `json:"latency_metric"`
				First  string `json:"first_token_ms"`
				Usage  struct {
					Input json.Number `json:"input_tokens"`
				} `json:"token_usage"`
			}
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatal(err)
			}
			if latency != duration || got.Metric != "request_duration" || got.First != first || got.Usage.Input.String() != "9007199254740993" {
				t.Fatalf("enrichment corrupted evidence: %s %s", latency, raw)
			}
			if count, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{sample}); err != nil || count != 0 {
				t.Fatalf("enrichment replay: %d %v", count, err)
			}
		})
	}
}
