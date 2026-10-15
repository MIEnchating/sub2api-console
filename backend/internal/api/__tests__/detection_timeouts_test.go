package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDetectionTimeoutAPIAuthenticatesValidatesAndPersists(t *testing.T) {
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	repo, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer tasks.Close()
	service, err := modelcheck.New(tasks, private, repo, qualityNoRemoteKeys{})
	if err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, repo, api.Dependencies{ModelChecks: service})
	for _, tc := range []struct {
		name, method, mode, body string
		auth                     bool
		status                   int
	}{
		{"anonymous read", "GET", "animation", "", false, 401},
		{"anonymous save", "PUT", "animation", `{"timeout_seconds":75,"version":0}`, false, 401},
		{"default", "GET", "animation", "", true, 200},
		{"invalid mode", "GET", "invalid", "", true, 422},
		{"invalid timeout", "PUT", "animation", `{"timeout_seconds":121,"version":0}`, true, 422},
		{"save animation", "PUT", "animation", `{"timeout_seconds":75,"version":0}`, true, 200},
		{"conflict", "PUT", "animation", `{"timeout_seconds":90,"version":0}`, true, 409},
		{"save precheck", "PUT", "precheck", `{"timeout_seconds":80,"version":0}`, true, 200},
		{"save review", "PUT", "review", `{"timeout_seconds":90,"version":0}`, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/model-checks/timeouts/"+tc.mode, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.auth {
				req.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("%d %s", response.Code, response.Body.String())
			}
			if tc.status == 200 {
				if response.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("cache allowed")
				}
				var value modelcheck.DetectionTimeout
				if err = json.Unmarshal(response.Body.Bytes(), &value); err != nil {
					t.Fatal(err)
				}
				if tc.name == "default" && value.TimeoutSeconds != 120 {
					t.Fatalf("default=%+v", value)
				}
			}
		})
	}
	reopened, err := modelcheck.New(tasks, private, repo, qualityNoRemoteKeys{})
	if err != nil {
		t.Fatal(err)
	}
	for mode, want := range map[string]int{"animation": 75, "precheck": 80, "review": 90} {
		value, err := reopened.DetectionTimeout(mode)
		if err != nil || value.TimeoutSeconds != want || value.Version != 1 {
			t.Fatalf("%s did not persist: %+v %v", mode, value, err)
		}
	}
}
