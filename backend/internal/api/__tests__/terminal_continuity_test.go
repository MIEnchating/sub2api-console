package api_test

import (
	"context"
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

type terminalService struct {
	api.ModelCheckService
	input *modelcheck.TerminalContinuityRequest
}

func (s *terminalService) EnqueueTerminalContinuity(_ context.Context, input modelcheck.TerminalContinuityRequest) (taskstore.Task, error) {
	s.input = &input
	return taskstore.Task{ID: "terminal-task", Status: "queued"}, nil
}
func (s *terminalService) TerminalContinuityHistory(context.Context) ([]taskstore.Task, error) {
	return []taskstore.Task{}, nil
}

func TestTerminalContinuityAPIIsAuthenticatedAndIndependent(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		auth               bool
		status             int
	}{
		{"anonymous submit", "POST", `{}`, false, 401},
		{"anonymous history", "GET", "", false, 401},
		{"invalid JSON", "POST", `{"targets":{}}`, true, 422},
		{"create", "POST", `{"targets":[{"account_id":"41","model":"gpt-6-astra"}],"timeout_seconds":120}`, true, 200},
		{"history", "GET", "", true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			service := &terminalService{}
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: service})
			request := httptest.NewRequest(tc.method, "/api/model-checks/terminal-continuity", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			if tc.auth {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if tc.status != 200 && service.input != nil {
				t.Fatal("invalid request reached service")
			}
			if tc.method == "POST" && tc.status == 200 && (service.input == nil || len(service.input.Targets) != 1 || service.input.Targets[0].AccountID != "41" || !strings.Contains(response.Body.String(), "terminal-task")) {
				t.Fatal("stable account binding or task response missing")
			}
		})
	}
}
