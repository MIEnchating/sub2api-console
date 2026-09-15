package api_test

import (
	"context"
	"encoding/json"
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

func TestWorkbenchLocalSetupRequiresExplicitChoiceAndCreatesAuthenticatedConsoleWithoutTarget(t *testing.T) {
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	biz, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer biz.Close()
	router := api.New(config.Config{}, private, biz)
	initialize := func(input map[string]any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "http://localhost/api/setup/initialize", strings.NewReader(string(raw)))
		req.RemoteAddr = "127.0.0.1:43210"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	legacy := initialize(map[string]any{"username": "local-operator", "password": "private-local-password"})
	if legacy.Code != http.StatusConflict {
		t.Fatalf("default initialization bypassed target requirement: %d %s", legacy.Code, legacy.Body.String())
	}
	ambiguous := initialize(map[string]any{"username": "local-operator", "password": "private-local-password", "local_export_only": true, "admin_base_url": "https://target.example", "admin_key": "private-target-key"})
	if ambiguous.Code != http.StatusUnprocessableEntity {
		t.Fatalf("local setup ignored supplied management configuration: %d", ambiguous.Code)
	}
	response := initialize(map[string]any{"username": "local-operator", "password": "private-local-password", "local_export_only": true})
	if response.Code != http.StatusOK {
		t.Fatalf("local initialization = %d %s", response.Code, response.Body.String())
	}
	var status struct {
		Initialized      bool `json:"initialized"`
		TargetConfigured bool `json:"target_configured"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || !status.Initialized || status.TargetConfigured {
		t.Fatalf("local setup status = %+v, %v", status, err)
	}
	if _, err := private.TargetSettings(context.Background()); err == nil {
		t.Fatal("local initialization created management settings")
	}
	if cookies := response.Result().Cookies(); len(cookies) != 1 || cookies[0].Name != "sub2api_console_session" || !cookies[0].HttpOnly || cookies[0].Value == "" {
		t.Fatal("local setup did not establish the normal protected Console session")
	}
	if strings.Contains(response.Body.String(), "private-local-password") {
		t.Fatal("local setup exposed Console password")
	}
}
