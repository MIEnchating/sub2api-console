package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestAutomationDailyScheduleAPI(t *testing.T) {
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	service := accountautomation.New(nil, nil, nil, nil, nil)
	service.UseScheduleRepository(private)
	router := api.New(config.Config{AdminToken: "test-admin"}, private, nil, api.Dependencies{AccountModelAutomation: service})
	for _, tc := range []struct {
		name, fields string
		status       int
	}{
		{"daily without interval", `"schedule_type":"daily","daily_times":["09:00","18:30"],"timezone":"Asia/Shanghai"`, http.StatusOK},
		{"duplicate clock", `"schedule_type":"daily","daily_times":["09:00","09:00"],"timezone":"Asia/Shanghai"`, http.StatusUnprocessableEntity},
		{"invalid clock", `"schedule_type":"daily","daily_times":["24:00"],"timezone":"Asia/Shanghai"`, http.StatusUnprocessableEntity},
		{"legacy invalid interval", `"interval_minutes":0`, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, "/api/management/accounts/models/automation/schedules", strings.NewReader(`{"name":"每日任务","group_ids":["7"],"platform_id":"platform","enabled":true,`+tc.fields+`}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer test-admin")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("got %d: %s", response.Code, response.Body.String())
			}
			if tc.status == http.StatusOK {
				var saved accountautomation.Schedule
				if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
					t.Fatal(err)
				}
				if saved.ScheduleType != "daily" || len(saved.DailyTimes) != 2 || saved.Timezone != "Asia/Shanghai" || saved.NextRunAt == nil {
					t.Fatalf("lost timing: %#v", saved)
				}
			}
		})
	}
}
