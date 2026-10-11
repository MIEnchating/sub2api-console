package api_test

import (
	"context"
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
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type automationAccounts struct{}

func (automationAccounts) Models(context.Context, string) ([]string, error) {
	return []string{"model-a"}, nil
}
func (automationAccounts) PreviewModels(context.Context, string) ([]string, error) {
	return []string{"model-a"}, nil
}
func (automationAccounts) AccountIDsByGroups(context.Context, []string) ([]string, error) {
	return []string{"10"}, nil
}

func TestAutomationAPIAllowsOptionalChannels(t *testing.T) {
	for _, tc := range []struct{ name, channels string }{
		{"omitted", ""}, {"empty", `,"channels":[]`}, {"null", `,"channels":null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer private.Close()
			tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer tasks.Close()
			runner := taskrunner.New(context.Background())
			defer func() {
				if err := runner.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			}()
			service := accountautomation.New(automationAccounts{}, nil, nil, tasks, runner, automationAccounts{})
			service.UseScheduleRepository(private)
			router := api.New(config.Config{AdminToken: "test-admin"}, private, nil, api.Dependencies{AccountModelAutomation: service})
			for _, method := range []string{http.MethodPost, http.MethodPut} {
				path := "/api/management/accounts/models/automation"
				if method == http.MethodPut {
					path += "/schedules"
				}
				fields := `,"dry_run":true`
				if method == http.MethodPut {
					fields = `,"name":"账号模型同步","interval_minutes":60`
				}
				body := `{"group_ids":["7"],"platform_id":"platform","price_enabled":true` + fields + tc.channels + `}`
				request := httptest.NewRequest(method, path, strings.NewReader(body))
				request.Header.Set("Authorization", "Bearer test-admin")
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("%s: %d %s", method, response.Code, response.Body.String())
				}
				if method == http.MethodPut {
					var schedule accountautomation.Schedule
					if err := json.Unmarshal(response.Body.Bytes(), &schedule); err != nil {
						t.Fatal(err)
					}
					if schedule.Request.Channels == nil || len(schedule.Request.Channels) != 0 || schedule.Request.PriceEnabled {
						t.Fatalf("invalid account-only schedule: %#v", schedule)
					}
				}
			}
		})
	}
}
