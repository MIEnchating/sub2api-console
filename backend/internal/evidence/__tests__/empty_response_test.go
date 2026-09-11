package evidence_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestUsageEnrichmentDetectsEmptyResponsesAndUpdatesSameEvidence(t *testing.T) {
	for _, withLatency := range []bool{false, true} {
		t.Run(map[bool]string{false: "without latency", true: "existing first token"}[withLatency], func(t *testing.T) {
			ctx, now := context.Background(), time.Now().UTC()
			path := filepath.Join(t.TempDir(), "evidence.sqlite3")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','test','{}','now'); INSERT INTO account_groups(account_id,group_name) VALUES('41','codex')`)
			_ = db.Close()
			if err != nil {
				t.Fatal(err)
			}
			var phase atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				rows := []map[string]any{}
				switch r.URL.Path {
				case "/api/v1/admin/ops/requests":
					row := map[string]any{"account_id": 41, "request_id": "empty", "kind": "success", "created_at": now.Format(time.RFC3339Nano)}
					if withLatency {
						row["first_token_ms"] = 1800
						row["duration_ms"] = 37000
					}
					rows = append(rows, row)
				case "/api/v1/admin/usage":
					rows = append(rows, map[string]any{"id": 9, "account_id": 42, "request_id": "empty", "input_tokens": 0, "output_tokens": 0})
					if phase.Load() == 1 || phase.Load() == 2 {
						output := 0
						if phase.Load() == 2 {
							output = 8
						}
						rows = append(rows, map[string]any{"id": 1, "account_id": 41, "request_id": "empty", "input_tokens": 0, "output_tokens": output})
					}
				default:
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": rows, "total": len(rows)}})
			}))
			t.Cleanup(server.Close)
			client, err := adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: "isolated-test", Attempts: 1, Timeout: time.Second}, nil)
			if err != nil {
				t.Fatal(err)
			}
			policy := map[string]any{"traffic": map[string]any{"lookback_minutes": 120}, "probe": map[string]any{"enabled": false}}
			service := evidence.New(store, nil)
			for index, want := range []routing.Event{routing.EventHealthy, "empty_response", routing.EventHealthy, routing.EventHealthy} {
				phase.Store(int32(index))
				result, err := service.Collect(ctx, policy, client, evidence.Options{FetchTraffic: true, Now: now.Add(time.Duration(index) * 10 * time.Minute)})
				if err != nil || len(result.SourceErrors) != 0 {
					t.Fatalf("collection=%+v err=%v", result, err)
				}
				rows, err := store.RoutingSamples(ctx, nil, nil, "traffic", 60)
				if err != nil || len(rows) != 1 {
					t.Fatalf("same request duplicated: rows=%v err=%v", rows, err)
				}
				row := rows[0]
				got, err := routing.ClassifySample(routing.Sample{Result: row.Result, Source: row.Source, Payload: row.Payload}, nil)
				if err != nil || got.Event != want {
					t.Fatalf("phase=%d classified=%+v payload=%v err=%v", index, got, row.Payload, err)
				}
			}
			_, err = store.PersistTrafficSamples(ctx, []business.TrafficSample{{AccountID: "41", GroupName: "other", Result: "通过", EvidenceKey: "empty", ObservedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{}}})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := store.RoutingSamples(ctx, nil, nil, "traffic", 60)
			if err != nil || len(rows) != 1 || rows[0].Payload["token_usage"] == nil {
				t.Fatalf("membership change lost usage: rows=%v err=%v", rows, err)
			}
		})
	}
}
