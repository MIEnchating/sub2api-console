package evidence_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestCostTrafficUsesVerifiedRequestGroupInsteadOfPrimaryMembership(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		opsGroup, usageGroup any
		usageAccount         int
		want                 int
	}{
		{"explicit request group", 8, nil, 41, 1},
		{"group enriched from matching usage", nil, 8, 41, 1},
		{"unknown request group", nil, nil, 41, 0},
		{"other account usage cannot supply group", nil, 8, 42, 0},
		{"request belongs to profitable group", 7, nil, 41, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "traffic.db")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES('41','account','0.3','now');
   INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('旗舰','7','0.6','now'),('平价','8','0.3','now');
   INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','旗舰','7'),('41','平价','8')`); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var row map[string]any
				switch r.URL.Path {
				case "/api/v1/admin/ops/requests":
					row = map[string]any{"account_id": 41, "request_id": "request", "kind": "success", "created_at": now.Format(time.RFC3339Nano), "group_id": tc.opsGroup, "first_token_ms": 100, "input_tokens": 1, "output_tokens": 1}
				case "/api/v1/admin/usage":
					row = map[string]any{"id": 1, "account_id": tc.usageAccount, "request_id": "request", "group_id": tc.usageGroup}
				default:
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{row}, "total": 1}})
			}))
			t.Cleanup(server.Close)
			client, err := adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: "test", Attempts: 1, Timeout: time.Second}, nil)
			if err != nil {
				t.Fatal(err)
			}
			result, err := evidence.New(store, nil).Collect(t.Context(), map[string]any{"traffic": map[string]any{"lookback_minutes": 120}, "probe": map[string]any{"enabled": false}}, client, evidence.Options{FetchTraffic: true, Now: now})
			if err != nil || len(result.SourceErrors) > 0 {
				t.Fatalf("collect=%+v err=%v", result, err)
			}
			if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM alert_incidents WHERE event_type='account.cost_traffic' AND incident_key='console:cost-traffic:41:平价' AND status='firing'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != tc.want {
				t.Fatalf("alerts=%d want=%d", count, tc.want)
			}
		})
	}
}
