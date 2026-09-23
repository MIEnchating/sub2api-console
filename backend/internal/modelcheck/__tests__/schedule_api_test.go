package modelcheck_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestBatchScheduleAPIRequiresAuthenticationAndPersistsExactMode(t *testing.T) {
	f := setup(t, 2, "openai", func(http.ResponseWriter, *http.Request) { t.Error("saving must not generate") })
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: f.service})
	body := `{"schedules":[{"account_id":"1","mode":"precheck","enabled":true,"model":"test-model","schedule_type":"daily","daily_time":"09:30","timezone":"Asia/Shanghai","timeout_seconds":5}]}`
	for _, authenticated := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodPut, "/api/model-checks/animation-schedules", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if authenticated {
			request.Header.Set("Authorization", "Bearer isolated-token")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if !authenticated {
			if response.Code != 401 || len(f.service.AnimationSchedules()) != 0 {
				t.Fatalf("unauthorized write: %d", response.Code)
			}
			continue
		}
		var views []modelcheck.AnimationScheduleView
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &views) != nil || len(views) != 1 || views[0].Mode != "precheck" || views[0].DailyTime != "09:30" {
			t.Fatalf("wrong response: %d %s", response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("schedule response must not persist in browser cache")
		}
	}
}
