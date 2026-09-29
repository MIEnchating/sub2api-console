package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestAccountGroupEndpointsEnforceAuthenticationInputAndManualProtection(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "groups.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(t.Context(), runtimepolicy.Full); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "account", "platform": "openai", "group_ids": []any{"7"}}}, []map[string]any{{"id": "7", "name": "original", "platform": "openai"}, {"id": "8", "name": "destination", "platform": "openai"}}, "test"); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.ConfigureTarget(t.Context(), "http://127.0.0.1:1", "isolated-only", 1); err != nil {
		t.Fatal(err)
	}
	service := accountops.New(private, store, nil)
	router := api.New(config.Config{AdminToken: "isolated-groups-token"}, private, store, api.Dependencies{AccountTasks: service})
	send := func(method, id, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/accounts/"+id+"/groups", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method+" requires authentication", func(t *testing.T) {
			if res := send(method, "41", `{}`, ""); res.Code != 401 {
				t.Fatalf("%d %s", res.Code, res.Body.String())
			}
		})
	}
	t.Run("preview returns stable group IDs", func(t *testing.T) {
		res := send(http.MethodGet, "41", "", "isolated-groups-token")
		var preview accountops.AccountGroupsPreview
		if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &preview) != nil || len(preview.CurrentGroupIDs) != 1 || preview.CurrentGroupIDs[0] != "7" || len(preview.Groups) != 2 {
			t.Fatalf("%d %s", res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), "isolated-only") {
			t.Fatal("credential exposed")
		}
	})
	for _, test := range []struct{ name, id, body string }{
		{"name is not stable ID", "account", `{}`},
		{"missing baseline", "41", `{"group_ids":["8"],"target_version":"v"}`},
		{"empty target", "41", `{"group_ids":[],"expected_group_ids":["7"],"target_version":"v"}`},
		{"invalid type", "41", `{"group_ids":[8],"expected_group_ids":["7"],"target_version":"v"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if res := send(http.MethodPut, test.id, test.body, "isolated-groups-token"); res.Code != 422 {
				t.Fatalf("%d %s", res.Code, res.Body.String())
			}
		})
	}
	if _, err := store.AssignManualPriority(t.Context(), "41", 1, "1", 1, true, "test"); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method+" protects manual account", func(t *testing.T) {
			res := send(method, "41", `{"group_ids":["8"],"expected_group_ids":["7"],"target_version":"v"}`, "isolated-groups-token")
			if res.Code != 409 || !strings.Contains(res.Body.String(), "手动控制") {
				t.Fatalf("%d %s", res.Code, res.Body.String())
			}
		})
	}
}
