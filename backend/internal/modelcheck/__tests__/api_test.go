package modelcheck_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAnimationAPIRequiresAuthenticationAndReturnsStoredResults(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"custom-model"}]}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
	})
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	store, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router := api.New(config.Config{AdminToken: "isolated-console-token"}, private, store, api.Dependencies{ModelChecks: f.service, Tasks: f.tasks.Store})
	call := func(method, path, body string, authenticated bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.Header.Set("Authorization", "Bearer isolated-console-token")
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	for _, path := range []string{"/api/model-checks/animations", "/api/model-checks/animation-schedules"} {
		if res := call(http.MethodGet, path, "", false); res.Code != 401 {
			t.Fatalf("unauthenticated %s: %d", path, res.Code)
		}
	}
	payload := `{"targets":[{"account_id":"1","model":"test-model"}],"timeout_seconds":5}`
	if res := call(http.MethodPost, "/api/model-checks/animations", payload, false); res.Code != 401 {
		t.Fatalf("unauthenticated write=%d", res.Code)
	}
	res := call(http.MethodPost, "/api/model-checks/animations", payload, true)
	if res.Code != 200 {
		t.Fatalf("create: %d %s", res.Code, res.Body)
	}
	var task taskstore.Task
	if json.Unmarshal(res.Body.Bytes(), &task) != nil {
		t.Fatal("invalid task JSON")
	}
	_ = finished(t, f)
	detail := call(http.MethodGet, "/api/tasks/"+task.ID, "", true)
	if detail.Code != 200 || !strings.Contains(detail.Body.String(), "animateTransform") || strings.Contains(detail.Body.String(), fixtureSecret) {
		t.Fatalf("detail: %d %s", detail.Code, detail.Body)
	}
	history := call(http.MethodGet, "/api/model-checks/animations", "", true)
	if history.Code != 200 || history.Header().Get("Cache-Control") != "no-store" || strings.Contains(history.Body.String(), "animateTransform") {
		t.Fatalf("history: %d %s", history.Code, history.Body)
	}
	invalid := call(http.MethodPut, "/api/model-checks/animation-schedules/2", `{"account_id":"1","enabled":true}`, true)
	if invalid.Code != 422 {
		t.Fatalf("mismatched route ID accepted: %d", invalid.Code)
	}
	t.Run("custom endpoint authenticates and omits credentials from task responses", func(t *testing.T) {
		payload := customRequest(t, *f.catalog.rows[0].BaseURL, "openai", fixtureSecret, "custom-model")
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if res := call(http.MethodPost, "/api/model-checks/animations", string(raw), false); res.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated custom request accepted: %d", res.Code)
		}
		res := call(http.MethodPost, "/api/model-checks/animations", string(raw), true)
		if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" || strings.Contains(res.Body.String(), fixtureSecret) {
			t.Fatalf("custom request: %d %s", res.Code, res.Body)
		}
		completed := finished(t, f)
		detail := call(http.MethodGet, "/api/tasks/"+completed.ID, "", true)
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "custom-") || strings.Contains(detail.Body.String(), fixtureSecret) {
			t.Fatalf("custom detail: %d %s", detail.Code, detail.Body)
		}
	})
	t.Run("custom model list authenticates and never caches credentials", func(t *testing.T) {
		payload := customRequest(t, *f.catalog.rows[0].BaseURL, "openai", fixtureSecret, "")
		raw, err := json.Marshal(payload.Custom)
		if err != nil {
			t.Fatal(err)
		}
		if res := call(http.MethodPost, "/api/model-checks/animations/models", string(raw), false); res.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated list accepted: %d", res.Code)
		}
		res := call(http.MethodPost, "/api/model-checks/animations/models", string(raw), true)
		if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" || strings.Contains(res.Body.String(), fixtureSecret) || !strings.Contains(res.Body.String(), `"models":["custom-model"]`) {
			t.Fatalf("custom model list: %d %s", res.Code, res.Body)
		}
	})
}
