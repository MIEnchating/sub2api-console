package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
)

func TestOnboardingAPIValidatesModelMappingBeforeEnqueue(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mapping  any
		existing bool
		status   int
	}{
		{"exact map", map[string]string{" alias ": " upstream "}, false, 200},
		{"empty map", map[string]string{}, false, 200},
		{"null", nil, false, 422},
		{"array", []string{"model"}, false, 422},
		{"non string target", map[string]any{"alias": 3}, false, 422},
		{"blank target", map[string]string{"alias": " "}, false, 422},
		{"wildcard", map[string]string{"*": "upstream"}, false, 422},
		{"existing account", map[string]string{"alias": "upstream"}, true, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &probeModelsOnboarding{}
			router := api.New(config.Config{AdminToken: "isolated-token"}, nil, nil, api.Dependencies{Onboarding: service})
			payload := map[string]any{"host": "upstream.test", "upstream_type": "sub2api", "upstream_group_id": "7", "local_group_ids": []int{3}, "model_mapping": tc.mapping}
			if tc.existing {
				payload["account_ids"] = []int{41}
			}
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/onboarding", strings.NewReader(string(body)))
			request.Header.Set("Authorization", "Bearer isolated-token")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if tc.status == 200 {
				if len(service.requests) != 1 {
					t.Fatal("request not enqueued")
				}
				if tc.name == "exact map" && !reflect.DeepEqual(service.requests[0].ModelMapping, map[string]string{"alias": "upstream"}) {
					t.Fatalf("mapping lost: %+v", service.requests[0])
				}
			} else if len(service.requests) != 0 {
				t.Fatal("invalid mapping reached background task")
			}
		})
	}
}
