package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type probeModelsOnboarding struct {
	api.OnboardingService
	requests []onboarding.Request
}

func (service *probeModelsOnboarding) Enqueue(_ context.Context, request onboarding.Request) (taskstore.Task, error) {
	service.requests = append(service.requests, request)
	return taskstore.Task{ID: "isolated-onboarding", Status: "queued"}, nil
}

func TestOnboardingAPIValidatesProbeModelsBeforeEnqueue(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		models any
		status int
		count  int
	}{
		{"selected", []string{" model-a ", "model-b", "model-a"}, 200, 2},
		{"empty fallback", []string{}, 200, 0},
		{"null rejected", nil, 422, 0},
		{"wrong type", "model-a", 422, 0},
		{"empty model", []string{" "}, 422, 0},
		{"long model", []string{strings.Repeat("x", 257)}, 422, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service := &probeModelsOnboarding{}
			router := api.New(config.Config{AdminToken: "isolated-token"}, nil, nil, api.Dependencies{Onboarding: service})
			body, err := json.Marshal(map[string]any{"host": "upstream.example", "upstream_type": "sub2api", "upstream_group_id": "7", "local_group_ids": []int{3}, "test_models": scenario.models})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/onboarding", strings.NewReader(string(body)))
			request.Header.Set("Authorization", "Bearer isolated-token")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if scenario.status == 200 && (len(service.requests) != 1 || len(service.requests[0].TestModels) != scenario.count) {
				t.Fatalf("probe models lost in API adaptation: %+v", service.requests)
			}
			if scenario.status != 200 && len(service.requests) != 0 {
				t.Fatal("invalid models reached background work")
			}
		})
	}
}
