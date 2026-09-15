package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type modelOptionsOnboarding struct {
	api.OnboardingService
	calls int
}

func (service *modelOptionsOnboarding) EnqueueProbe(_ context.Context, action, host, group, model, _ string) (taskstore.Task, error) {
	service.calls++
	return taskstore.Task{ID: "isolated-options", Status: "queued", Result: map[string]any{"action": action, "host": host, "group_id": group, "model": model}}, nil
}

func TestModelOptionsAPIRequiresAuthenticationAndExactStableTarget(t *testing.T) {
	for _, scenario := range []struct {
		name, body    string
		authenticated bool
		status        int
	}{
		{"valid", `{"host":"options.test","group_id":"6"}`, true, 200},
		{"unauthenticated", `{"host":"options.test","group_id":"6"}`, false, 401},
		{"missing group", `{"host":"options.test"}`, true, 422},
		{"group name rejected", `{"host":"options.test","group_name":"OpenAI"}`, true, 422},
		{"extra generation model", `{"host":"options.test","group_id":"6","model":"gpt"}`, true, 422},
		{"invalid group type", `{"host":"options.test","group_id":6}`, true, 422},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service := &modelOptionsOnboarding{}
			private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{Onboarding: service})
			request := httptest.NewRequest(http.MethodPost, "/api/onboarding/probe/tasks/model-options", strings.NewReader(scenario.body))
			request.Header.Set("Content-Type", "application/json")
			if scenario.authenticated {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if scenario.status == 200 {
				if service.calls != 1 || !strings.Contains(response.Body.String(), `"action":"model-options"`) || !strings.Contains(response.Body.String(), `"group_id":"6"`) {
					t.Fatalf("adaptation=%s", response.Body.String())
				}
			} else if service.calls != 0 {
				t.Fatal("invalid request enqueued")
			}
		})
	}
}
