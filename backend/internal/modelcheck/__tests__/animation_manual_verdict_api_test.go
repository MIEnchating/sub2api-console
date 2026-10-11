package modelcheck_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestAnimationManualVerdictAPIAuthenticatesAndValidatesStableResult(t *testing.T) {
	f := manualVerdictFixture(t)
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: f.service})
	valid := `{"account_id":"1","request_id":"request-1","verdict":"not_passed"}`
	for _, tc := range []struct {
		name, id, body string
		auth           bool
		status         int
	}{
		{"anonymous", "completed", valid, false, 401},
		{"malformed", "completed", `{`, true, 422},
		{"invalid verdict", "completed", `{"account_id":"1","request_id":"request-1","verdict":"invalid"}`, true, 422},
		{"mismatched target", "completed", `{"account_id":"2","request_id":"request-1","verdict":"passed"}`, true, 422},
		{"active task", "running", valid, true, 409},
		{"missing task", "missing", valid, true, 404},
		{"save", "completed", valid, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.id != "missing" {
				task := manualVerdictTask(tc.id)
				if tc.id == "running" {
					task.Status = "running"
				}
				if err := f.tasks.Store.Save(context.Background(), task); err != nil {
					t.Fatal(err)
				}
			}
			request := httptest.NewRequest("PUT", "/api/model-checks/animations/"+tc.id+"/manual-verdict", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			if tc.auth {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if tc.status == 200 && (!strings.Contains(response.Body.String(), `"manual_verdict"`) || !strings.Contains(response.Body.String(), `"updated_by":"console"`)) {
				t.Fatal("saved annotation missing from response")
			}
		})
	}
}
