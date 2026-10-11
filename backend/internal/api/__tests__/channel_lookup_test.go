package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestChannelLookupReadsOnlySelectedIDsAndPreservesUnavailableIdentity(t *testing.T) {
	var calls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		id := strings.TrimPrefix(r.URL.Path, "/api/channel/")
		if r.Method != http.MethodGet || (id != "1201" && id != "99") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if id == "99" {
			id = "98"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
			"id": id, "name": "跨页渠道", "type": 1, "status": 1, "models": "model-a", "group": "default", "key": "private-key",
		}})
	}))
	defer remote.Close()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.SaveNewAPIPlatform(t.Context(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, UserID: "1", AdminKey: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	manager := newapimanagement.New(store, nil, remote.Client(), nil, nil)
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil, api.Dependencies{NewAPIManagement: manager})
	path := "/api/newapi/platforms/primary/channels/lookup?id=1201&id=99"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous lookup = %d", response.Code)
	}
	for _, query := range []string{"", "?id=0", "?id=01", "?id=../42", "?id=42&id=42", "?id=" + strings.Repeat("9", 30), "?" + strings.Repeat("id=42&", 51)} {
		req := httptest.NewRequest(http.MethodGet, "/api/newapi/platforms/primary/channels/lookup"+query, nil)
		req.Header.Set("Authorization", "Bearer test-admin")
		response = httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid lookup %q = %d %s", query, response.Code, response.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input reached upstream")
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer test-admin")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var result struct {
		Items    []newapimanagement.Channel `json:"items"`
		Failures []struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		} `json:"failures"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || len(result.Items) != 1 || result.Items[0].ID != "1201" || len(result.Failures) != 1 || result.Failures[0].ID != "99" {
		t.Fatalf("lookup result = %d %s", response.Code, response.Body.String())
	}
	if result.Failures[0].Code != "channel_identity_changed" || strings.Contains(response.Body.String(), "private-key") || calls.Load() != 2 {
		t.Fatalf("unsafe or unbounded lookup: %s, calls=%d", response.Body.String(), calls.Load())
	}
}
