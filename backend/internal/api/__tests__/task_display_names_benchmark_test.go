package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// Expose the original Business contract without the optional display projection.
// Both paths still use real API handlers, authentication and temporary SQLite.
type legacyTaskNameBusiness struct{ api.Business }

func BenchmarkTaskDetailNames(b *testing.B) {
	rows := make([]map[string]any, 1000)
	for i := range rows {
		rows[i] = map[string]any{
			"id": json.Number(strconv.Itoa(i + 1)), "name": "账号 " + strconv.Itoa(i+1),
			"temp_unschedulable_reason": strings.Repeat("x", 4096),
		}
	}
	accounts, private, tasks := taskNameStores(b, rows)
	task := taskstore.Task{ID: "legacy", Skill: "console", Operation: "active-probe", Status: "succeeded", Progress: 100, Message: "完成",
		CreatedAt: "2026-10-10T00:00:00Z", UpdatedAt: "2026-10-10T00:00:00Z",
		Result: map[string]any{"results": []any{map[string]any{"account_id": "41", "result": "通过"}}},
	}
	if err := tasks.Save(context.Background(), task); err != nil {
		b.Fatal(err)
	}
	for _, scenario := range []struct {
		name     string
		business api.Business
		paths    []string
	}{
		{"legacy_full_directory", legacyTaskNameBusiness{accounts}, []string{"/api/tasks/legacy", "/api/accounts"}},
		{"scoped_display_names", accounts, []string{"/api/tasks/legacy"}},
	} {
		router := api.New(config.Config{AdminToken: "isolated-token"}, private, scenario.business, api.Dependencies{Tasks: tasks})
		b.Run(scenario.name, func(b *testing.B) {
			b.ReportAllocs()
			responseBytes := 0
			for b.Loop() {
				responseBytes = 0
				for _, path := range scenario.paths {
					request := httptest.NewRequest(http.MethodGet, path, nil)
					request.Header.Set("Authorization", "Bearer isolated-token")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != http.StatusOK {
						b.Fatalf("%s status %d", path, response.Code)
					}
					responseBytes += response.Body.Len()
				}
			}
			b.ReportMetric(float64(responseBytes), "response-B/op")
		})
	}
}
