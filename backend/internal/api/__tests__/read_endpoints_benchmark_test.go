package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
)

// Real authenticated handlers and isolated stores, including serialization.
// This measures local processing; remote service latency is tested separately.
func BenchmarkConsoleReadEndpoints(b *testing.B) {
	rows := make([]map[string]any, 1000)
	for i := range rows {
		rows[i] = map[string]any{"id": json.Number(strconv.Itoa(i + 1)), "name": "fixture account", "temp_unschedulable_reason": strings.Repeat("x", 4096), "group_ids": []any{strconv.Itoa(i%10 + 1)}}
	}
	accounts, private, tasks := taskNameStores(b, nil)
	groups := make([]map[string]any, 10)
	for i := range groups {
		groups[i] = map[string]any{"id": strconv.Itoa(i + 1), "name": "fixture group " + strconv.Itoa(i+1), "platform": "openai"}
	}
	if _, err := accounts.SyncManagementSnapshot(b.Context(), rows, groups, "benchmark"); err != nil {
		b.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, accounts, api.Dependencies{Tasks: tasks})
	for _, path := range []string{
		"/api/overview", "/api/config", "/api/policy", "/api/groups", "/api/upstreams", "/api/upstreams/usage",
		"/api/catalog/accounts", "/api/catalog/groups", "/api/catalog/group-memberships", "/api/catalog/upstreams",
		"/api/accounts", "/api/accounts/41", "/api/accounts/41/control-state", "/api/tasks", "/api/events", "/api/dictionaries",
	} {
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.Header.Set("Authorization", "Bearer isolated-token")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					b.Fatalf("%s returned %d", path, response.Code)
				}
				b.ReportMetric(float64(response.Body.Len()), "response-B/op")
			}
		})
	}
}
