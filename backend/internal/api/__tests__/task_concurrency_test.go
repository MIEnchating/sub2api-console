package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/tasksettings"
)

func TestTaskConcurrencyRoutesAuthenticateValidateAndApply(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	g := taskrunner.NewQueued(context.Background(), 2, 100)
	defer g.Cancel()
	s, err := tasksettings.New(context.Background(), store, map[string]*taskrunner.Group{"account": g})
	if err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil, api.Dependencies{TaskSettings: s})
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/api/config/task-concurrency", nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatal(response.Code)
		}
	}
	invoke := func(method, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "/api/config/task-concurrency", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer test-admin")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	response := invoke(http.MethodGet, "")
	var state tasksettings.State
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &state) != nil || len(state.Pools) != 1 {
		t.Fatal(response.Body.String())
	}
	input := tasksettings.Input{Limits: map[string]int{"account": 5}, QueueCapacity: 30, Version: state.Version}
	body, _ := json.Marshal(input)
	response = invoke(http.MethodPut, string(body))
	if response.Code != 200 || g.Snapshot().Limit != 5 {
		t.Fatalf("save %d %s", response.Code, response.Body.String())
	}
	response = invoke(http.MethodPut, string(body))
	if response.Code != http.StatusConflict {
		t.Fatal(response.Code)
	}
	input.Version = s.Snapshot().Version
	input.Limits["account"] = -1
	body, _ = json.Marshal(input)
	response = invoke(http.MethodPut, string(body))
	if response.Code != http.StatusUnprocessableEntity || g.Snapshot().Limit != 5 {
		t.Fatalf("validation %d", response.Code)
	}
}
