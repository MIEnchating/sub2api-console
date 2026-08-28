package upstreamdetect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetectNewAPIFromExplicitPublicMarkers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/status" {
			t.Fatalf("unexpected fallback request: %s", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"success":true,"data":{"system_name":"Example API","quota_per_unit":500000}}`))
	}))
	defer server.Close()

	result, err := New(nil).Detect(context.Background(), server.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if !result.TypeDetected || result.UpstreamType == nil || *result.UpstreamType != "newapi" || result.AuthMode == nil || *result.AuthMode != "newapi_admin_key" || result.Name == nil || *result.Name != "Example API" || result.Evidence == nil || *result.Evidence != "/api/status" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDetectSub2APIFallsBackAcrossPublicEndpointsWithoutRedirects(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		switch request.URL.Path {
		case "/api/status":
			http.Redirect(response, request, "/redirected", http.StatusFound)
		case "/api/v1/settings/public":
			_, _ = response.Write([]byte(`{"code":0,"data":{"site_name":"Sub2API","server_timezone":"UTC"}}`))
		default:
			t.Fatalf("redirect was followed or unexpected endpoint requested: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	result, err := New(nil).Detect(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if result.UpstreamType == nil || *result.UpstreamType != "sub2api" || result.Name == nil || *result.Name != "Sub2API" || strings.Join(paths, ",") != "/api/status,/api/v1/settings/public" {
		t.Fatalf("result=%#v paths=%#v", result, paths)
	}
}

func TestDetectDoesNotInferPlatformFromNameOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/status":
			_, _ = response.Write([]byte(`{"success":true,"data":{"name":"Unknown Console"}}`))
		case "/api/v1/settings/public":
			_, _ = response.Write([]byte(`{"name":"Fallback Name"}`))
		case "/setup/status":
			_, _ = response.Write([]byte(`{"code":1,"data":{"needs_setup":false}}`))
		}
	}))
	defer server.Close()

	result, err := New(nil).Detect(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if result.TypeDetected || result.UpstreamType != nil || !result.NameDetected || result.Name == nil || *result.Name != "Unknown Console" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDetectRejectsInvalidBaseURLBeforeNetwork(t *testing.T) {
	_, err := New(nil).Detect(context.Background(), "not-a-url")
	if err == nil || err.Error() != "上游地址必须是完整的 http 或 https 地址" {
		t.Fatalf("unexpected error: %v", err)
	}
}
