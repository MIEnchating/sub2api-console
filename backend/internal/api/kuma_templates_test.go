package api

import (
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"strings"
	"testing"
)

func TestKumaTemplateRoutesRequireAuthentication(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	for _, tc := range []struct{ method, path string }{{"GET", "/api/uptime-kuma/templates"}, {"POST", "/api/uptime-kuma/templates"}, {"PUT", "/api/uptime-kuma/templates/id"}, {"DELETE", "/api/uptime-kuma/templates/id"}} {
		if r := request(t, router, tc.method, tc.path, nil, ""); r.Code != 401 {
			t.Fatalf("%s %s: %d", tc.method, tc.path, r.Code)
		}
	}
}
func TestKumaTemplateAPIStoresSecretsWithoutReturningThemAndRemovesUnusedFeatures(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	r := authenticatedRequest(t, router, "POST", "/api/uptime-kuma/templates", map[string]any{"name": "API 请求", "method": "POST", "headers": `{"X-Key":"private-key"}`, "body": "private-body", "auth_method": "bearer", "auth_password": "private-token"})
	if r.Code != 200 || strings.Contains(r.Body.String(), "private-") {
		t.Fatalf("unsafe create: %d", r.Code)
	}
	var item struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	r = authenticatedRequest(t, router, "GET", "/api/uptime-kuma/templates", nil)
	if r.Code != 200 || strings.Contains(r.Body.String(), "private-") || !strings.Contains(r.Body.String(), `"auth_configured":true`) {
		t.Fatal("unsafe template list")
	}
	r = authenticatedRequest(t, router, "DELETE", "/api/uptime-kuma/templates/"+item.ID, map[string]any{"revision": item.Revision})
	if r.Code != 200 {
		t.Fatalf("delete: %d", r.Code)
	}
	for _, kind := range []string{"notifications", "maintenance"} {
		r = authenticatedRequest(t, router, "GET", "/api/uptime-kuma/resources/"+kind, nil)
		if r.Code != 404 {
			t.Fatal("removed feature remains active")
		}
	}
}

func TestKumaTemplateDetailReturnsEditableRequestButNotAuthCredentials(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	created := authenticatedRequest(t, router, "POST", "/api/uptime-kuma/templates", map[string]any{"name": "请求回显", "method": "POST", "headers": `{"X-Key":"editable-header"}`, "body": `{"messages":[{"role":"user","content":"ping"}]}`, "auth_method": "basic", "auth_username": "private-user", "auth_password": "private-password"})
	if created.Code != 200 {
		t.Fatalf("create: %d", created.Code)
	}
	var summary map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	path := "/api/uptime-kuma/templates/" + summary["id"].(string)
	if r := request(t, router, "GET", path, nil, ""); r.Code != 401 {
		t.Fatalf("unauth detail: %d", r.Code)
	}
	r := authenticatedRequest(t, router, "GET", path, nil)
	if r.Code != 200 {
		t.Fatalf("detail: %d", r.Code)
	}
	var detail map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["headers"] != `{"X-Key":"editable-header"}` || detail["body"] != `{"messages":[{"role":"user","content":"ping"}]}` || detail["revision"] != summary["revision"] {
		t.Fatal("request content or revision missing")
	}
	if strings.Contains(r.Body.String(), "private-") || detail["auth_password"] != nil || detail["auth_username"] != nil {
		t.Fatal("detail exposed auth credentials")
	}
	if r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("detail must not be cached")
	}
	if r := authenticatedRequest(t, router, "GET", "/api/uptime-kuma/templates/invalid", nil); r.Code != 422 {
		t.Fatalf("invalid ID: %d", r.Code)
	}
	if r := authenticatedRequest(t, router, "GET", "/api/uptime-kuma/templates/"+strings.Repeat("f", 48), nil); r.Code != 409 {
		t.Fatalf("missing template: %d", r.Code)
	}
}

func TestKumaTemplatePresetRequiresAuthAndReturnsEditableContent(t *testing.T) {
	router, _ := testRouter(t, config.Config{AdminToken: "test-token"}, fakeBusiness{})
	path := "/api/uptime-kuma/template-preset"
	input := map[string]any{"request_profile": "openai-responses", "model": "selected-model"}
	if r := request(t, router, "POST", path, input, ""); r.Code != 401 {
		t.Fatalf("auth: %d", r.Code)
	}
	r := authenticatedRequest(t, router, "POST", path, input)
	if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("preset: %d", r.Code)
	}
	var result struct {
		Body     string `json:"body"`
		Encoding string `json:"body_encoding"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Body, "selected-model") || !strings.Contains(result.Body, "max_output_tokens") || result.Encoding != "json" {
		t.Fatal("missing editable preset")
	}
}
