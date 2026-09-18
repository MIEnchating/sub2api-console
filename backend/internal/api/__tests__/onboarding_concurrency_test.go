package api_test

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

type concurrencyPreviewService struct{ api.OnboardingService }

func (*concurrencyPreviewService) PreviewConcurrency(_ context.Context, requests []onboarding.Request) ([]onboarding.ConcurrencyAllocation, error) {
	value := int64(7)
	return []onboarding.ConcurrencyAllocation{{Concurrency: &value}}, nil
}

func TestConcurrencyPreviewRequiresAuthenticationAndValidatesInput(t *testing.T) {
	for _, test := range []struct {
		name, token, body string
		status            int
	}{
		{"authorized", "isolated-token", `{"items":[{"host":"upstream.example","upstream_type":"sub2api","upstream_group_id":"7","local_group_ids":[3]}]}`, 200},
		{"unauthenticated", "", `{"items":[]}`, 401},
		{"empty batch", "isolated-token", `{"items":[]}`, 422},
		{"invalid manual value", "isolated-token", `{"items":[{"host":"upstream.example","upstream_type":"sub2api","upstream_group_id":"7","local_group_ids":[3],"concurrency":0}]}`, 422},
		{"waiting confirmation", "isolated-token", `{"items":[{"host":"upstream.example","upstream_type":"sub2api","upstream_group_id":"7","local_group_ids":[3],"concurrency":1,"schedulable":false,"waiting_for_capacity":true}]}`, 200},
		{"active waiting rejected", "isolated-token", `{"items":[{"host":"upstream.example","upstream_type":"sub2api","upstream_group_id":"7","local_group_ids":[3],"concurrency":1,"schedulable":true,"waiting_for_capacity":true}]}`, 422},
		{"invalid waiting type", "isolated-token", `{"items":[{"host":"upstream.example","upstream_type":"sub2api","upstream_group_id":"7","local_group_ids":[3],"concurrency":1,"waiting_for_capacity":"true"}]}`, 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{Onboarding: &concurrencyPreviewService{}})
			request := httptest.NewRequest(http.MethodPost, "/api/onboarding/concurrency-preview", strings.NewReader(test.body))
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.status == 200 {
				var result struct {
					Items []onboarding.ConcurrencyAllocation `json:"items"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if response.Header().Get("Cache-Control") != "no-store" || len(result.Items) != 1 || result.Items[0].Concurrency == nil || *result.Items[0].Concurrency != 7 {
					t.Fatalf("invalid preview: %+v", result)
				}
			}
		})
	}
}
