package api_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type detectionService struct {
	api.ModelCheckService
	called  bool
	input   modelcheck.DetectionTask
	id      string
	version int
}

func (s *detectionService) DetectionTasks() []modelcheck.DetectionTaskView {
	s.called = true
	return []modelcheck.DetectionTaskView{}
}
func (s *detectionService) SaveDetectionTask(_ context.Context, value modelcheck.DetectionTask, _ string) ([]modelcheck.DetectionTaskView, error) {
	s.called = true
	s.input = value
	if value.Version != 2 {
		return nil, errors.New("检测任务已变化，请刷新后重新编辑")
	}
	return []modelcheck.DetectionTaskView{{DetectionTask: value}}, nil
}
func (s *detectionService) DeleteDetectionTask(_ context.Context, id string, version int, _ string) ([]modelcheck.DetectionTaskView, error) {
	s.called = true
	s.id = id
	s.version = version
	return []modelcheck.DetectionTaskView{}, nil
}
func (s *detectionService) RunDetectionTask(_ context.Context, id string, version int) (taskstore.Task, error) {
	s.called = true
	s.id = id
	s.version = version
	return taskstore.Task{ID: "run-1", Status: "queued"}, nil
}

func TestDetectionTaskAPIAuthenticationAndVersionContract(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		auth                     bool
		status                   int
	}{
		{"anonymous list", "GET", "", "", false, 401},
		{"anonymous save", "PUT", "", `{}`, false, 401},
		{"anonymous delete", "DELETE", "/plan-1", `{"version":2}`, false, 401},
		{"anonymous run", "POST", "/plan-1/run", `{"version":2}`, false, 401},
		{"list", "GET", "", "", true, 200},
		{"invalid body", "PUT", "", `{"group_ids":{}}`, true, 422},
		{"stale version", "PUT", "", `{"version":1}`, true, 422},
		{"save", "PUT", "", `{"id":"plan-1","version":2,"group_ids":["7","8"],"terminal_rounds":5,"daily_times":["09:00","20:00"]}`, true, 200},
		{"delete", "DELETE", "/plan-1", `{"version":2}`, true, 200},
		{"run", "POST", "/plan-1/run", `{"version":2}`, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			service := &detectionService{}
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: service})
			request := httptest.NewRequest(tc.method, "/api/model-checks/detection-tasks"+tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			if tc.auth {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if !tc.auth && service.called {
				t.Fatal("anonymous request reached service")
			}
			if tc.auth && response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("configuration can be cached")
			}
			if tc.name == "save" && (len(service.input.GroupIDs) != 2 || service.input.TerminalRounds != 5 || len(service.input.DailyTimes) != 2) {
				t.Fatal("configuration fields lost")
			}
			if (tc.name == "run" || tc.name == "delete") && (service.id != "plan-1" || service.version != 2) {
				t.Fatal("stable ID or version lost")
			}
			if tc.name == "run" && !strings.Contains(response.Body.String(), "run-1") {
				t.Fatal("missing task ID")
			}
			if tc.name == "stale version" && !strings.Contains(response.Body.String(), "刷新") {
				t.Fatal("failure reason lost")
			}
		})
	}
}
