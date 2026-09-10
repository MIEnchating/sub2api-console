package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestKumaRoutesRequireConsoleAuthentication(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	for _, tc := range []struct{ method, path string }{{"GET", "/api/uptime-kuma/config"}, {"PUT", "/api/uptime-kuma/config"}, {"DELETE", "/api/uptime-kuma/config"}, {"GET", "/api/uptime-kuma/monitors"}, {"POST", "/api/uptime-kuma/monitors"}, {"POST", "/api/uptime-kuma/monitors/19"}} {
		r := request(t, router, tc.method, tc.path, nil, "")
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: %d", tc.method, tc.path, r.Code)
		}
	}
}
func TestKumaConfigurationReturnsOnlyPublicSummaryAndVersionedDisconnect(t *testing.T) {
	router, store := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	if err := store.SaveUptimeKuma(context.Background(), configstore.UptimeKumaConfig{BaseURL: "https://test.invalid", APIKey: "private-key", Username: "admin", Password: "private-password", Token: "private-token"}); err != nil {
		t.Fatal(err)
	}
	r := authenticatedRequest(t, router, "GET", "/api/uptime-kuma/config", nil)
	if r.Code != 200 || strings.Contains(r.Body.String(), "private-") || !strings.Contains(r.Body.String(), `"management_configured":true`) {
		t.Fatalf("unsafe config response: %d %s", r.Code, r.Body.String())
	}
	r = authenticatedRequest(t, router, "DELETE", "/api/uptime-kuma/config", map[string]any{"revision": 0})
	if r.Code != 409 {
		t.Fatalf("stale disconnect: %d", r.Code)
	}
	r = authenticatedRequest(t, router, "DELETE", "/api/uptime-kuma/config", map[string]any{"revision": 1})
	if r.Code != 200 {
		t.Fatalf("disconnect: %d %s", r.Code, r.Body.String())
	}
	stored, err := store.UptimeKuma(context.Background())
	if err != nil || stored.Token != "" || stored.APIKey != "" {
		t.Fatal("disconnect retained secrets")
	}
}
func TestKumaResourceAndPushRoutesRequireAuthentication(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	for _, path := range []string{"/api/uptime-kuma/monitors/19/push-url", "/api/uptime-kuma/resources/notifications", "/api/uptime-kuma/resources/maintenance", "/api/uptime-kuma/resources/status-pages", "/api/uptime-kuma/resources/notifications/7"} {
		if r := request(t, router, "GET", path, nil, ""); r.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d", path, r.Code)
		}
	}
	for _, kind := range []string{"notifications", "maintenance", "status-pages"} {
		if r := request(t, router, "POST", "/api/uptime-kuma/resources/"+kind+"/0", nil, ""); r.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d", kind, r.Code)
		}
	}
}
func TestKumaMonitorIDIsValidatedBeforeCreatingTask(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	for _, id := range []string{"0", "-1", "name", "9223372036854775808"} {
		r := authenticatedRequest(t, router, "POST", "/api/uptime-kuma/monitors/"+id, map[string]any{"action": "delete"})
		if r.Code != 422 {
			t.Fatalf("ID %q: %d", id, r.Code)
		}
	}
}
