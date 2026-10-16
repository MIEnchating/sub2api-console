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

type verificationOnboarding struct {
	api.OnboardingService
	model, mode string
}

func (service *verificationOnboarding) EnqueueProbe(_ context.Context, action, host, groupID, model, mode string) (taskstore.Task, error) {
	service.model, service.mode = model, mode
	return taskstore.Task{ID: "verification-task", Status: "queued"}, nil
}

func TestOnboardingVerificationRequiresAuthenticatedBackgroundTask(t *testing.T) {
	for _, scenario := range []struct {
		name, path, model, mode string
		authenticated           bool
		status                  int
	}{
		{"behavior task", "/api/onboarding/probe/tasks/probe", "selected-model", "fingerprint", true, 200},
		{"behavior synchronous rejected", "/api/onboarding/probe", "selected-model", "fingerprint", true, 422},
		{"background task", "/api/onboarding/probe/tasks/probe", "selected-model", "verification", true, 200},
		{"synchronous rejected", "/api/onboarding/probe", "selected-model", "verification", true, 422},
		{"missing model", "/api/onboarding/probe/tasks/probe", "", "verification", true, 422},
		{"invalid mode", "/api/onboarding/probe/tasks/probe", "selected-model", "unknown", true, 422},
		{"anonymous rejected", "/api/onboarding/probe/tasks/probe", "selected-model", "verification", false, 401},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service := &verificationOnboarding{}
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{Onboarding: service})
			request := httptest.NewRequest(http.MethodPost, scenario.path, strings.NewReader(`{"host":"upstream.test","group_id":"6","model":"`+scenario.model+`","mode":"`+scenario.mode+`"}`))
			request.Header.Set("Content-Type", "application/json")
			if scenario.authenticated {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if scenario.status == 200 && (service.model != scenario.model || service.mode != scenario.mode) {
				t.Fatalf("selection lost: %+v", service)
			}
			if scenario.status != 200 && service.mode != "" {
				t.Fatal("invalid request reached execution")
			}
		})
	}
}
