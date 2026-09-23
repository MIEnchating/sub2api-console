package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type animationModelReader struct {
	api.ModelCheckService
	fail      bool
	failure   error
	accountID string
}

func (s *animationModelReader) AccountAnimationModels(_ context.Context, id string) ([]string, error) {
	s.accountID = id
	if s.failure != nil {
		return nil, s.failure
	}
	if s.fail {
		return nil, errors.New("OAuth 模型目录读取失败，请检查授权、账号代理或管理连接后重试；也可手动输入模型 ID")
	}
	return []string{"live-model"}, nil
}

func TestAnimationModelAPIHidesUnexpectedInternalFailure(t *testing.T) {
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	service := &animationModelReader{failure: errors.New("storage unavailable at /private/internal.db")}
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: service})
	request := httptest.NewRequest(http.MethodGet, "/api/model-checks/animations/accounts/41/models", nil)
	request.Header.Set("Authorization", "Bearer isolated-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "/private/internal.db") || !strings.Contains(response.Body.String(), "上游服务请求失败") {
		t.Fatalf("internal discovery failure exposed: %d %s", response.Code, response.Body.String())
	}
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
			if scenario.fail && !strings.Contains(response.Body.String(), "OAuth 模型目录读取失败，请检查授权、账号代理或管理连接后重试；也可手动输入模型 ID") {
				t.Fatalf("model discovery failure lost its actionable detail: %s", response.Body.String())
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
