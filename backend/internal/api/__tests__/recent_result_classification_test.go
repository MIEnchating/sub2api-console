package api_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestAccountRecentResultsExposeNullForNeutralErrorsAndPreserveScoredFailures(t *testing.T) {
	for _, endpoint := range []string{"/api/accounts", "/api/accounts/41"} {
		t.Run(endpoint, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "business.sqlite3")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.ExecContext(t.Context(), `INSERT INTO accounts(id,name,updated_at)
				VALUES('41','classification-fixture','2026-09-17T07:00:00Z')`); err != nil {
				t.Fatal(err)
			}
			for _, sample := range []struct {
				request, reason string
				status          int
			}{
				{"client-error", "HTTP 400 invalid request", 400},
				{"gateway-error", "HTTP 502 bad gateway", 502},
				{"fatal-error", "HTTP 401 invalid api key", 401},
			} {
				_, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
					AccountID: "41", GroupName: "classification", EvidenceKey: sample.request,
					Result: "failed", FailureReason: &sample.reason, ObservedAt: "2026-09-17T07:00:00Z",
					Payload: map[string]any{"status_code": sample.status},
				}})
				if err != nil {
					t.Fatal(err)
				}
			}
			private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			router := api.New(config.Config{AdminToken: "isolated-classification-token"}, private, store)
			request := httptest.NewRequest(http.MethodGet, endpoint, nil)
			request.Header.Set("Authorization", "Bearer isolated-classification-token")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			type accountResponse struct {
				RecentResults []struct {
					EventType string          `json:"event_type"`
					Score     json.RawMessage `json:"score"`
				} `json:"recent_results"`
			}
			var account accountResponse
			if endpoint == "/api/accounts" {
				var accounts []accountResponse
				if err := json.Unmarshal(response.Body.Bytes(), &accounts); err != nil {
					t.Fatal(err)
				}
				if len(accounts) != 1 {
					t.Fatalf("accounts=%d, want one isolated fixture", len(accounts))
				}
				account = accounts[0]
			} else if err := json.Unmarshal(response.Body.Bytes(), &account); err != nil {
				t.Fatal(err)
			}
			if len(account.RecentResults) != 3 {
				t.Fatalf("recent results=%d, want all three traffic outcomes", len(account.RecentResults))
			}
			expected := map[string]string{"client_error": "null", "gateway_error": "25", "credential_invalid": "0"}
			for _, result := range account.RecentResults {
				want, found := expected[result.EventType]
				if !found || string(result.Score) != want {
					t.Errorf("event=%q score=%s, want %s", result.EventType, result.Score, want)
				}
				delete(expected, result.EventType)
			}
			if len(expected) > 0 {
				t.Errorf("missing classified outcomes: %v", expected)
			}
		})
	}
}
