package api_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestAccountGroupLockRequiresAuthenticationStableIDAndBoolean(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "group-lock.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "account", "group_ids": []any{"7"}}}, []map[string]any{{"id": "7", "name": "group", "platform": "openai"}}, "test"); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	router := api.New(config.Config{AdminToken: "isolated-lock-token"}, private, store, api.Dependencies{})
	send := func(id, body, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "/api/accounts/"+id+"/group-lock", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if result := send("41", `{"groups_locked":true}`, ""); result.Code != 401 {
		t.Fatalf("unauthorized=%d", result.Code)
	}
	for _, sample := range []struct{ id, body string }{{"name", `{"groups_locked":true}`}, {"41", `{}`}, {"41", `{"groups_locked":"true"}`}, {"41", `{"groups_locked":true,"extra":1}`}} {
		if result := send(sample.id, sample.body, "isolated-lock-token"); result.Code != 422 {
			t.Fatalf("invalid %q %q: %d %s", sample.id, sample.body, result.Code, result.Body.String())
		}
	}
	if result := send("999", `{"groups_locked":true}`, "isolated-lock-token"); result.Code != 404 {
		t.Fatalf("missing=%d %s", result.Code, result.Body.String())
	}
	if result := send("41", `{"groups_locked":true}`, "isolated-lock-token"); result.Code != 200 || !strings.Contains(result.Body.String(), `"groups_locked":true`) {
		t.Fatalf("lock=%d %s", result.Code, result.Body.String())
	}
	if result := send("41", `{"groups_locked":false}`, "isolated-lock-token"); result.Code != 200 || !strings.Contains(result.Body.String(), `"groups_locked":false`) {
		t.Fatalf("unlock=%d %s", result.Code, result.Body.String())
	}
}

func TestAccountGroupLockDoesNotReadUnrelatedAccountHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group-lock.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "account", "group_ids": []any{"7"}}}, []map[string]any{{"id": "7", "name": "group", "platform": "openai"}}, "test"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`DROP TABLE account_stability_samples; DROP TABLE health_samples`); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	router := api.New(config.Config{AdminToken: "isolated-lock-token"}, private, store, api.Dependencies{})
	request := httptest.NewRequest(http.MethodPut, "/api/accounts/41/group-lock", strings.NewReader(`{"groups_locked":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer isolated-lock-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"groups_locked":true`) {
		t.Fatalf("account control failed with unrelated history unavailable: %d %s", response.Code, response.Body.String())
	}
}
