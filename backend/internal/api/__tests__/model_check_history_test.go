package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestModelCheckHistoryPaginatesOnlyDetectionSummaries(t *testing.T) {
	accounts, private, tasks := taskNameStores(t, nil)
	for index := 0; index < 22; index++ {
		skill := "sub2api-model-check"
		if index == 21 {
			skill = "sub2api-model-animation"
		}
		task := taskstore.Task{ID: fmt.Sprintf("history-%02d", index), Skill: skill, Operation: "account-model-behavior-check", Status: "succeeded", Progress: 100, Message: "检测完成", CreatedAt: "2026-10-15T00:00:00Z", UpdatedAt: "2026-10-15T00:00:00Z", Result: map[string]any{"tests": []any{"large-private-answer"}}}
		if err := tasks.Save(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, accounts, api.Dependencies{Tasks: tasks})
	for _, tc := range []struct {
		query string
		count int
		more  bool
	}{{"?page=1", 20, true}, {"?page=2", 1, false}, {"?page=3", 0, false}} {
		request := httptest.NewRequest(http.MethodGet, "/api/model-checks/history"+tc.query, nil)
		request.Header.Set("Authorization", "Bearer isolated-token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("status %d: %s", response.Code, response.Body.String())
		}
		var body struct {
			Items   []map[string]any `json:"items"`
			HasMore bool             `json:"has_more"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Items) != tc.count || body.HasMore != tc.more || strings.Contains(response.Body.String(), "large-private-answer") {
			t.Fatalf("invalid bounded history: %s", response.Body.String())
		}
		if tc.query == "?page=1" && body.Items[0]["id"] != "history-20" {
			t.Fatalf("unstable history order: %v", body.Items)
		}
	}
	for _, tc := range []struct {
		query  string
		auth   bool
		status int
	}{{"", false, 401}, {"?page=0", true, 422}, {"?page=no", true, 422}, {"?page=100001", true, 422}} {
		request := httptest.NewRequest(http.MethodGet, "/api/model-checks/history"+tc.query, nil)
		if tc.auth {
			request.Header.Set("Authorization", "Bearer isolated-token")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s: want %d got %d", tc.query, tc.status, response.Code)
		}
	}
}
