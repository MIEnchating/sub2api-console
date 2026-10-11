package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
)

func TestOnboardingNewAccountDefaultsToSchedulableUnlessExplicitlyDisabled(t *testing.T) {
	for _, scenario := range []struct {
		name, field string
		want        bool
	}{
		{"omitted", "", true},
		{"explicitly disabled", `,"schedulable":false`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service := &probeModelsOnboarding{}
			router := api.New(config.Config{AdminToken: "isolated-token"}, nil, nil, api.Dependencies{Onboarding: service})
			body := `{"host":"upstream.example","upstream_type":"sub2api","upstream_group_id":"7","local_group_ids":[3]` + scenario.field + `}`
			request := httptest.NewRequest(http.MethodPost, "/api/onboarding", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer isolated-token")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || len(service.requests) != 1 {
				t.Fatalf("status=%d body=%s requests=%d", response.Code, response.Body.String(), len(service.requests))
			}
			if got := service.requests[0].Schedulable; got != scenario.want {
				t.Fatalf("schedulable=%t, want %t", got, scenario.want)
			}
		})
	}
}
