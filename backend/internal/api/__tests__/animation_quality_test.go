package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// No network operation is allowed in this integration path.
type qualityNoRemoteKeys struct{ modelcheck.KeyRevealer }

func TestQualityAPIAuthenticatesAndPersistsAnnotationsByStableRequest(t *testing.T) {
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
	now := "2026-10-14T00:00:00Z"
	task := taskstore.Task{ID: "quality-api", Message: "动画检测完成", Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "succeeded", CreatedAt: now, UpdatedAt: now, Result: map[string]any{"animations": []modelcheck.AnimationResult{{AccountID: "41", RequestID: "req-1", Status: "succeeded", HTML: "<html>preview</html>", Source: "<html>original</html>", Prompt: q.PelicanPrompt, CompletedAt: now}}}}
	if err = tasks.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, repo, api.Dependencies{ModelChecks: service})
	payload := `{"account_id":"41","request_id":"req-1","label":"normal","reason":"结构完整","version":0}`
	for _, tc := range []struct {
		name, method, path, body string
		auth                     bool
		status                   int
	}{
		{"anonymous settings", "GET", "/api/model-checks/animation-quality/settings", "", false, 401},
		{"anonymous settings write", "PUT", "/api/model-checks/animation-quality/settings", `{"enabled":false,"timeout_seconds":60,"version":0}`, false, 401},
		{"settings read", "GET", "/api/model-checks/animation-quality/settings", "", true, 200},
		{"invalid reviewer", "PUT", "/api/model-checks/animation-quality/settings", `{"enabled":true,"account_id":"41","model":"review","timeout_seconds":60,"version":0}`, true, 422},
		{"settings save", "PUT", "/api/model-checks/animation-quality/settings", `{"enabled":false,"timeout_seconds":60,"version":0}`, true, 200},
		{"settings stale", "PUT", "/api/model-checks/animation-quality/settings", `{"enabled":false,"timeout_seconds":60,"version":0}`, true, 409},
		{"anonymous read", "GET", "/api/model-checks/animations/quality-api/quality?account_id=41&request_id=req-1", "", false, 401},
		{"anonymous export", "GET", "/api/model-checks/animation-quality/samples", "", false, 401},
		{"anonymous write", "PUT", "/api/model-checks/animations/quality-api/quality", payload, false, 401},
		{"read", "GET", "/api/model-checks/animations/quality-api/quality?account_id=41&request_id=req-1", "", true, 200},
		{"wrong request", "GET", "/api/model-checks/animations/quality-api/quality?account_id=41&request_id=other", "", true, 422},
		{"missing task", "GET", "/api/model-checks/animations/missing/quality?account_id=41&request_id=req-1", "", true, 404},
		{"bad body", "PUT", "/api/model-checks/animations/quality-api/quality", `{`, true, 422},
		{"save", "PUT", "/api/model-checks/animations/quality-api/quality", payload, true, 200},
		{"stale version", "PUT", "/api/model-checks/animations/quality-api/quality", payload, true, 409},
		{"bad cursor", "GET", "/api/model-checks/animation-quality/samples?after=garbage", "", true, 422},
		{"bad limit", "GET", "/api/model-checks/animation-quality/samples?limit=101", "", true, 422},
		{"export", "GET", "/api/model-checks/animation-quality/samples", "", true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.auth {
				req.Header.Set("Authorization", "Bearer isolated-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("%d %s", response.Code, response.Body.String())
			}
			if tc.name == "save" {
				var saved modelcheck.AnimationQualityReview
				if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
					t.Fatal(err)
				}
				if saved.Annotation == nil || saved.Annotation.UpdatedBy != "console" || saved.Annotation.Version != 1 {
					t.Fatalf("%+v", saved)
				}
			}
			if tc.name == "export" {
				var page q.SamplePage
				if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				if len(page.Items) != 1 || page.Items[0].HTML != "<html>original</html>" {
					t.Fatalf("%+v", page)
				}
			}
		})
	}
}
