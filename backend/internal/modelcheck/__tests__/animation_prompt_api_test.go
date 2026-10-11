package modelcheck_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnimationPromptAPIAuthenticatesValidatesAndReadsSavedValue(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected generation") })
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: f.service})
	for _, tc := range []struct {
		name, method, body string
		auth               bool
		status             int
	}{
		{"anonymous read", "GET", "", false, 401},
		{"anonymous write", "PUT", `{"prompt":"鲸鱼","version":0}`, false, 401},
		{"malformed", "PUT", `{`, true, 422},
		{"empty prompt", "PUT", `{"prompt":" ","version":0}`, true, 422},
		{"save", "PUT", `{"prompt":"鲸鱼","version":0}`, true, 200},
		{"stale", "PUT", `{"prompt":"旧值","version":0}`, true, 422},
		{"read", "GET", "", true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, "/api/model-checks/animation-prompt", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			if tc.auth {
				request.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if tc.status == 200 && (!strings.Contains(response.Body.String(), `"prompt":"鲸鱼"`) || !strings.Contains(response.Body.String(), `"version":1`) || response.Header().Get("Cache-Control") != "no-store") {
				t.Fatalf("unexpected saved response: %s", response.Body.String())
			}
		})
	}
}
