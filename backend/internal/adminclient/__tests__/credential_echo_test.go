package adminclient_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func TestManagementErrorsRedactActualKeyBeforeDetailTruncation(t *testing.T) {
	for _, operation := range []string{"account", "preview"} {
		for _, scenario := range []struct {
			name   string
			status int
			field  string
			prefix string
			key    string
		}{
			{"HTTP JSON echo", http.StatusForbidden, "message", "rejected ", "private-management-value"},
			{"HTTP text truncation", http.StatusForbidden, "", strings.Repeat("x", 284), "private-management-value"},
			{"HTTP escaped JSON truncation", http.StatusForbidden, "detail", strings.Repeat("x", 284), "private-management-<quoted>"},
			{"business error truncation", http.StatusOK, "error", strings.Repeat("x", 284), "private-management-value"},
			{"HTTP unrecognized JSON field", http.StatusForbidden, "diagnostic", "rejected ", "private-management-<quoted>"},
		} {
			t.Run(operation+"/"+scenario.name, func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Header.Get("X-API-Key") != scenario.key {
						t.Error("management request must retain its actual authentication")
					}
					w.WriteHeader(scenario.status)
					detail := scenario.prefix + scenario.key
					if scenario.field == "" {
						_, _ = io.WriteString(w, detail)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, scenario.field: detail})
				}))
				defer server.Close()
				client, err := adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: scenario.key, Attempts: 1}, nil)
				if err != nil {
					t.Fatal(err)
				}
				if operation == "account" {
					_, err = client.Account(t.Context(), "1")
				} else {
					_, err = client.GenerateAccountPreview(t.Context(), "1", previewInput())
				}
				if err == nil {
					t.Fatal("management failure was accepted")
				}
				if strings.Contains(err.Error(), "private-management") || !strings.Contains(err.Error(), scenario.prefix+"[已隐藏]") {
					t.Fatalf("actual management key or its truncated prefix escaped redaction: %s", err)
				}
				if calls != 1 {
					t.Fatalf("failure unexpectedly replayed: %d calls", calls)
				}
				if operation == "account" && scenario.status != http.StatusOK {
					var httpError *adminclient.HTTPError
					if !errors.As(err, &httpError) || httpError.StatusCode != scenario.status {
						t.Fatalf("HTTP status classification was lost: %v", err)
					}
				}
			})
		}
	}
}
