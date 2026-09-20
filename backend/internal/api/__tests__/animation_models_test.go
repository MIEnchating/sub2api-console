package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type animationModelReader struct {
	api.ModelCheckService
	fail      bool
	accountID string
}

func (s *animationModelReader) AccountAnimationModels(_ context.Context, id string) ([]string, error) {
	s.accountID = id
	if s.fail {
		return nil, errors.New("绑定 Key 已失效，请更新授权后重试")
	}
	return []string{"live-model"}, nil
}

func TestAnimationModelAPIRequiresAuthenticationAndStableID(t *testing.T) {
	for _, scenario := range []struct {
		name, id         string
		authorized, fail bool
		status           int
	}{
		{"authenticated account", "41", true, false, 200},
		{"anonymous", "41", false, false, 401},
		{"invalid ID", "name", true, false, 422},
		{"discovery failure", "41", true, true, 502},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service := &animationModelReader{fail: scenario.fail}
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: service})
			request := httptest.NewRequest(http.MethodGet, "/api/model-checks/animations/accounts/"+scenario.id+"/models", nil)
			if scenario.authorized {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if scenario.status == 200 && (service.accountID != "41" || response.Body.String() != `{"models":["live-model"]}` || response.Header().Get("Cache-Control") != "no-store") {
				t.Fatalf("model response lost its account or cache boundary: %s", response.Body.String())
			}
			if (scenario.status == 401 || scenario.status == 422) && service.accountID != "" {
				t.Fatal("invalid request reached discovery")
			}
		})
	}
}
