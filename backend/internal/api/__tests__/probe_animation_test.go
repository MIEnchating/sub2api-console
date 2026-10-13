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
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type animationProbeOnboarding struct {
	api.OnboardingService
	mode string
}

func (s *animationProbeOnboarding) EnqueueProbe(_ context.Context, action, host, group, model, mode string) (taskstore.Task, error) {
	s.mode = mode
	return taskstore.Task{ID: "animation-probe", Status: "queued", Result: map[string]any{"action": action, "host": host, "group_id": group, "model": model}}, nil
}
func TestAnimationProbeAPIRequiresAuthenticatedTaskAndValidMode(t *testing.T) {
	for _, scenario := range []struct {
		name, mode, path string
		auth             bool
		status           int
	}{
		{"animation task", "animation", "/api/onboarding/probe/tasks/probe", true, 200},
		{"candy task", "candy", "/api/onboarding/probe/tasks/probe", true, 200},
		{"anonymous candy", "candy", "/api/onboarding/probe/tasks/probe", false, 401},
		{"synchronous candy rejected", "candy", "/api/onboarding/probe", true, 422},
		{"invalid mode", "invalid", "/api/onboarding/probe/tasks/probe", true, 422},
		{"anonymous", "animation", "/api/onboarding/probe/tasks/probe", false, 401},
		{"synchronous animation rejected", "animation", "/api/onboarding/probe", true, 422},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service := &animationProbeOnboarding{}
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{Onboarding: service})
			raw, _ := json.Marshal(map[string]string{"host": "upstream.test", "group_id": "6", "model": "gpt-6-astra", "mode": scenario.mode})
			request := httptest.NewRequest(http.MethodPost, scenario.path, strings.NewReader(string(raw)))
			request.Header.Set("Content-Type", "application/json")
			if scenario.auth {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if scenario.status == 200 && (service.mode != scenario.mode || !strings.Contains(response.Body.String(), "gpt-6-astra")) {
				t.Fatal("animation mode or model lost")
			}
			if scenario.status != 200 && service.mode != "" {
				t.Fatal("invalid request created task")
			}
		})
	}
}
