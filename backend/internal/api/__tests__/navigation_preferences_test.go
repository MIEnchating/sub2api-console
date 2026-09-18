package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestNavigationRoutesRequireAuthenticationAndRejectStaleVersion(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/api/preferences/navigation", strings.NewReader(`{"hidden_item_ids":[]}`)))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous request accepted: %d", response.Code)
		}
	}
	for index, body := range []string{`{"hidden_item_ids":["accounts"],"version":""}`, `{"hidden_item_ids":[],"version":""}`} {
		request := httptest.NewRequest(http.MethodPut, "/api/preferences/navigation", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer test-admin")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		expected := http.StatusOK
		if index == 1 {
			expected = http.StatusConflict
		}
		if response.Code != expected {
			t.Fatalf("save returned %d: %s", response.Code, response.Body.String())
		}
		if index == 1 {
			var failure struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Code == "" {
				t.Fatalf("missing error code: %s", response.Body.String())
			}
		}
	}
}

func TestNavigationRoutesRejectMalformedBodyWithoutSaving(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil)
	for _, body := range []string{`{"hidden_item_ids":[]} {}`, `{"hidden_item_ids":[],"unexpected":true}`, `null`} {
		request := httptest.NewRequest(http.MethodPut, "/api/preferences/navigation", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer test-admin")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("malformed body accepted: %d %s", response.Code, response.Body.String())
		}
	}
}
