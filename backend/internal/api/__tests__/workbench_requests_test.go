package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestWorkbenchRoutesBindPreviewToSessionAndRemoveLegacyTools(t *testing.T) {
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	ctx := context.Background()
	if err = private.Initialize(ctx, "operator", "isolated-password", "https://target.invalid", "test-key"); err != nil {
		t.Fatal(err)
	}
	first, err := private.CreateSession(ctx, "operator", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := private.CreateSession(ctx, "operator", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tasks.Close()
	runner := taskrunner.NewBounded(ctx, 1)
	defer runner.Shutdown(ctx)
	service := accountworkbench.New(private)
	service.UseExecution(tasks, runner, nil, nil, t.TempDir())
	router := api.New(config.Config{}, private, nil, api.Dependencies{AccountWorkbench: service})
	request := func(method, path, body, session string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://console.test"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://console.test")
		if session != "" {
			req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: session})
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if response := request(http.MethodGet, "/api/account-workbench/runs", "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated records status=%d", response.Code)
	}
	preview := request(http.MethodPost, "/api/account-workbench/preview", `{"content":"rt_private-input","action":"export"}`, first)
	if preview.Code != http.StatusOK || preview.Header().Get("Cache-Control") != "no-store" || strings.Contains(preview.Body.String(), "rt_private-input") {
		t.Fatalf("preview response invalid: %d", preview.Code)
	}
	var parsed accountworkbench.Preview
	if err = json.Unmarshal(preview.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(accountworkbench.RunConfirmation{ID: parsed.ID, Revision: parsed.Revision})
	if response := request(http.MethodPost, "/api/account-workbench/runs", string(body), second); response.Code != http.StatusConflict {
		t.Fatalf("foreign preview accepted: %d", response.Code)
	}
	for _, path := range []string{"/api/account-workbench/oauth", "/api/account-workbench/security", "/api/account-workbench/profiles", "/api/account-workbench/sms", "/api/account-workbench/checkpoints"} {
		if response := request(http.MethodPost, path, `{}`, first); response.Code != http.StatusNotFound {
			t.Fatalf("legacy route %s returned %d", path, response.Code)
		}
	}
	if response := request(http.MethodGet, "/api/account-workbench/maintenance", "", first); response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("maintenance status unavailable: %d", response.Code)
	}
}
