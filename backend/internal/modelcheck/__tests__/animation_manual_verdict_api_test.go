package modelcheck_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRetiredAnimationVerdictRejectsWritesAndPreservesHistory(t *testing.T) {
	f := qualitySourceFixture(t)
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	task := qualitySourceTask("legacy")
	task.Result["animations"].([]any)[0].(map[string]any)["manual_verdict"] = map[string]any{"verdict": "passed"}
	if err = f.tasks.Store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	before, _ := f.tasks.Get(t.Context(), task.ID)
	router := api.New(config.Config{AdminToken: "isolated-token"}, private, nil, api.Dependencies{ModelChecks: f.service})
	for _, auth := range []bool{false, true} {
		req := httptest.NewRequest("PUT", "/api/model-checks/animations/legacy/manual-verdict", strings.NewReader(`{"account_id":"1","request_id":"request-1","verdict":"not_passed"}`))
		req.Header.Set("Content-Type", "application/json")
		if auth {
			req.Header.Set("Authorization", "Bearer isolated-token")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		want := 401
		if auth {
			want = 410
		}
		if response.Code != want {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if auth && !strings.Contains(response.Body.String(), "animation_manual_verdict_retired") {
			t.Fatal(response.Body.String())
		}
	}
	after, _ := f.tasks.Get(t.Context(), task.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("retired endpoint changed history")
	}
}
