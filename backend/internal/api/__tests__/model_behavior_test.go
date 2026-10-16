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
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type behaviorCheckService struct {
	api.ModelCheckService
	request modelcheck.Request
}

func (s *behaviorCheckService) Enqueue(_ context.Context, r modelcheck.Request) (taskstore.Task, error) {
	s.request = r
	return taskstore.Task{ID: "behavior-task", Status: "queued"}, nil
}

func TestModelBehaviorAPIRequiresExplicitBooleanOption(t *testing.T) {
	for _, value := range []string{"true", "false", `"true"`} {
		t.Run(value, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			service := &behaviorCheckService{}
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: service})
			request := httptest.NewRequest(http.MethodPost, "/api/model-checks", strings.NewReader(`{"account_ids":["1"],"models":["model-a"],"behavior_fingerprint":`+value+`}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer isolated-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if value == `"true"` {
				if response.Code < 400 {
					t.Fatal("invalid option accepted")
				}
				return
			}
			if response.Code != 200 || service.request.BehaviorFingerprint != (value == "true") {
				t.Fatalf("behavior option lost: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestModelPublicReferenceAPIRequiresExplicitBooleanOption(t *testing.T) {
	for _, value := range []string{"true", "false", `"true"`} {
		t.Run(value, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			service := &behaviorCheckService{}
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: service})
			request := httptest.NewRequest(http.MethodPost, "/api/model-checks", strings.NewReader(`{"account_ids":["1"],"models":["model-a"],"public_reference":`+value+`}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer isolated-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if value == `"true"` {
				if response.Code < 400 {
					t.Fatal("invalid option accepted")
				}
				return
			}
			if response.Code != 200 || service.request.PublicReference != (value == "true") {
				t.Fatalf("public reference option lost: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
