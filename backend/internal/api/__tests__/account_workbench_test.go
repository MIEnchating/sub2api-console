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
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func workbenchRouter(t *testing.T, localOnly ...bool) (http.Handler, string) {
	t.Helper()
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if len(localOnly) > 0 && localOnly[0] {
		err = private.InitializeLocalExport(context.Background(), "tester", "isolated-password")
	} else {
		err = private.Initialize(context.Background(), "tester", "isolated-password", "https://target.example", "test-admin-key")
	}
	if err != nil {
		t.Fatal(err)
	}
	biz, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = biz.Close() })
	if err := biz.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tasks.Close() })
	runner := taskrunner.NewBounded(context.Background(), 2)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = runner.Shutdown(ctx)
	})
	service := accountworkbench.New(private, tasks, biz, nil, runner)
	if err := service.UseExportDirectory(filepath.Join(t.TempDir(), "exports")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.CloseExports() })
	router := api.New(config.Config{}, private, biz, api.Dependencies{AccountWorkbench: service, Tasks: tasks, TaskCanceller: runner})
	token, err := private.CreateSession(context.Background(), "tester", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return router, token
}

func TestAccountWorkbenchDependencyMakesAuthenticatedTemplateRouteAvailable(t *testing.T) {
	router, token := workbenchRouter(t)
	req := httptest.NewRequest(http.MethodGet, "http://console.test/api/account-workbench/templates", nil)
	req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("wired route = %d %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "[]" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected template response: %s", response.Body.String())
	}
}

func TestAccountWorkbenchRoutesRejectRequestsWithoutLogin(t *testing.T) {
	router, _ := workbenchRouter(t)
	for _, route := range []struct{ method, path string }{
		{"GET", "templates"}, {"POST", "templates"}, {"PUT", "templates/id"}, {"DELETE", "templates/id"},
		{"PUT", "templates/id/preference"},
		{"POST", "template-from-account"}, {"POST", "preview"}, {"DELETE", "preview/id"}, {"POST", "import"},
		{"GET", "history"}, {"GET", "maintenance"}, {"PUT", "maintenance"}, {"POST", "maintenance/check"},
		{"POST", "history/delete"}, {"POST", "history/cancel"},
		{"POST", "history/query"}, {"GET", "history/active"},
		{"POST", "runs/preview"}, {"DELETE", "runs/preview/id"}, {"POST", "runs"}, {"GET", "runs/id"}, {"POST", "runs/id/preview"}, {"DELETE", "runs/id"},
		{"GET", "login-profiles"}, {"POST", "login-profiles"}, {"DELETE", "login-profiles/id"}, {"POST", "login-profiles/security-result"}, {"POST", "reauthorization/preview"},
		{"GET", "maintenance/authorization"}, {"POST", "maintenance/authorization"},
		{"POST", "oauth"}, {"GET", "oauth/id"}, {"POST", "oauth/id/input"},
		{"POST", "sms/options"},
		{"POST", "oauth/id/finish"}, {"DELETE", "oauth/id"}, {"POST", "oauth/id/preview"},
		{"POST", "retry-preview"}, {"POST", "exports/preview"}, {"DELETE", "exports/preview/id"},
		{"GET", "exports"}, {"POST", "exports"}, {"DELETE", "exports/id"},
		{"POST", "oauth-batches/preview"}, {"DELETE", "oauth-batches/preview/id"},
		{"POST", "oauth-batches"}, {"GET", "oauth-batches/id"}, {"DELETE", "oauth-batches/id"},
		{"POST", "oauth-batches/id/preview"},
		{"POST", "security"}, {"GET", "security/id"}, {"POST", "security/id/input"}, {"POST", "security/id/continue"}, {"DELETE", "security/id"},
		{"POST", "security-batches/preview"}, {"DELETE", "security-batches/preview/id"}, {"POST", "security-batches"}, {"GET", "security-batches/id"}, {"DELETE", "security-batches/id"},
	} {
		t.Run(route.method+"_"+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, "http://console.test/api/account-workbench/"+route.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://console.test")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated route = %d", response.Code)
			}
		})
	}
}

func TestAccountWorkbenchOAuthRoutesNeverCacheSessionState(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, route := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "oauth", "{}", http.StatusConflict},
		{"GET", "oauth/missing", "", http.StatusNotFound},
		{"POST", "oauth/missing/input", `{"kind":"key","key":"Tab"}`, http.StatusNotFound},
		{"POST", "oauth/missing/input", `{"kind":"evaluate","text":"document.cookie"}`, http.StatusUnprocessableEntity},
		{"POST", "oauth/missing/finish", "{}", http.StatusNotFound},
		{"POST", "oauth/missing/preview", "{}", http.StatusNotFound},
		{"DELETE", "oauth/missing", "", http.StatusNotFound},
		{"POST", "oauth-batches", `{"preview_id":"missing","confirmed":false}`, http.StatusConflict},
		{"GET", "oauth-batches/missing", "", http.StatusNotFound},
		{"POST", "oauth-batches/missing/preview", "{}", http.StatusNotFound},
		{"DELETE", "oauth-batches/missing", "", http.StatusNotFound},
		{"POST", "security", `{"account_id":"1","operation":"totp","confirmed":false}`, http.StatusConflict},
		{"GET", "security/missing", "", http.StatusNotFound},
		{"POST", "security/missing/input", `{"kind":"key","key":"Tab"}`, http.StatusNotFound},
		{"POST", "security/missing/input", `{"kind":"evaluate","text":"document.cookie"}`, http.StatusUnprocessableEntity},
		{"POST", "security/missing/continue", "{}", http.StatusNotFound},
		{"DELETE", "security/missing", "", http.StatusNotFound},
		{"POST", "security-batches", `{"preview_id":"missing","confirmed":false}`, http.StatusConflict},
		{"GET", "security-batches/missing", "", http.StatusNotFound},
		{"DELETE", "security-batches/missing", "", http.StatusNotFound},
	} {
		t.Run(route.method+"_"+route.path+"_"+route.body, func(t *testing.T) {
			req := httptest.NewRequest(route.method, "http://console.test/api/account-workbench/"+route.path, strings.NewReader(route.body))
			req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://console.test")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != route.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("route status=%d cache=%s", response.Code, response.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestAccountWorkbenchBatchPreviewReturnsOnlyCredentialFreeAccountScope(t *testing.T) {
	router, token := workbenchRouter(t)
	input := accountworkbench.OAuthBatchPreviewInput{Content: `[{"email":"operator@example.com","password":"PrivatePassword123!"}]`}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://console.test/api/account-workbench/oauth-batches/preview", strings.NewReader(string(raw)))
	req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://console.test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("preview response status=%d cache=%s", response.Code, response.Header().Get("Cache-Control"))
	}
	if strings.Contains(response.Body.String(), "PrivatePassword123!") {
		t.Fatal("preview exposed account password")
	}
	var preview accountworkbench.OAuthBatchPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.ID == "" || len(preview.Items) != 1 || preview.Items[0].Email != "operator@example.com" || !preview.Items[0].HasPassword || len(preview.Errors) != 0 {
		t.Fatalf("unexpected public account scope: %+v", preview)
	}
}

func TestAccountWorkbenchPreferenceRequiresExplicitStateAndRevision(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, body := range []string{`{"revision":1}`, `{"preferred":true}`, `{"revision":0,"preferred":false}`} {
		req := httptest.NewRequest(http.MethodPut, "http://console.test/api/account-workbench/templates/id/preference", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://console.test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusUnprocessableEntity || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("preference validation status=%d cache=%s", response.Code, response.Header().Get("Cache-Control"))
		}
	}
}

func TestAccountWorkbenchPrivateExportsListOnlyMetadata(t *testing.T) {
	router, token := workbenchRouter(t)
	req := httptest.NewRequest(http.MethodGet, "http://console.test/api/account-workbench/exports", nil)
	req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Body.String() != "[]" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("private artifact listing status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAccountWorkbenchExportCannotBeSubmittedWithoutConfirmation(t *testing.T) {
	router, token := workbenchRouter(t)
	req := httptest.NewRequest(http.MethodPost, "http://console.test/api/account-workbench/exports", strings.NewReader(`{"preview_id":"unconfirmed","confirmed":false}`))
	req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://console.test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusConflict {
		t.Fatalf("unconfirmed export status=%d", response.Code)
	}
}
