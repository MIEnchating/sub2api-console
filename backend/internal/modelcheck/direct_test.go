package modelcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestDirectOpenAIResponsesUsesAccountBaseURLAndBearerKey(t *testing.T) {
	const secret = "sk-direct-test-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/proxy/v1/responses" {
			t.Errorf("path=%q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("authorization=%q", request.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["model"] != "gpt-5.6-sol" || body["input"] != "probe" {
			t.Errorf("body=%#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"model":"gpt-5.6-sol","output":[{"content":[{"type":"output_text","text":"answer"}]}]}`))
	}))
	defer server.Close()

	sender := directBundleSender{
		client: server.Client(),
		credential: directCredential{
			BaseURL: server.URL + "/proxy/v1?ignored=true", Secret: secret, Platform: "openai",
		},
	}
	text, responseModel, err := sender.Send(context.Background(), "41", "gpt-5.6-sol", "probe", 5)
	if err != nil {
		t.Fatal(err)
	}
	if text != "answer" || responseModel != "gpt-5.6-sol" {
		t.Fatalf("text=%q responseModel=%q", text, responseModel)
	}
}

func TestDirectOpenAIFallsBackToChatCompletionsWhenResponsesIsUnsupported(t *testing.T) {
	paths := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/responses":
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"message":"route not found"}}`))
		case "/v1/chat/completions":
			_, _ = writer.Write([]byte(`{"model":"gpt-5.6-sol","choices":[{"message":{"content":"fallback answer"}}]}`))
		default:
			writer.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	sender := directBundleSender{
		client:     server.Client(),
		credential: directCredential{BaseURL: server.URL, Secret: "key", Platform: "openai"},
	}
	text, responseModel, err := sender.Send(context.Background(), "41", "gpt-5.6-sol", "probe", 5)
	if err != nil {
		t.Fatal(err)
	}
	if text != "fallback answer" || responseModel != "gpt-5.6-sol" {
		t.Fatalf("text=%q responseModel=%q", text, responseModel)
	}
	want := []string{"/v1/responses", "/v1/chat/completions"}
	if len(paths) != len(want) || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("paths=%#v want=%#v", paths, want)
	}
}

func TestDirectRequestErrorRedactsAccountKey(t *testing.T) {
	const secret = "sk-must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"error":{"message":"rejected key ` + secret + `"}}`))
	}))
	defer server.Close()

	sender := directBundleSender{
		client:     server.Client(),
		credential: directCredential{BaseURL: server.URL, Secret: secret, Platform: "openai"},
	}
	_, _, err := sender.Send(context.Background(), "41", "gpt-5.6-sol", "probe", 5)
	if err == nil {
		t.Fatal("expected request error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("secret leaked in error: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("error=%q", err.Error())
	}
}

func TestDirectRequestRejectsTrailingJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"model":"gpt-5.6-sol","output_text":"answer"}{"extra":true}`))
	}))
	defer server.Close()

	sender := directBundleSender{
		client:     server.Client(),
		credential: directCredential{BaseURL: server.URL, Secret: "key", Platform: "openai"},
	}
	_, _, err := sender.Send(context.Background(), "41", "gpt-5.6-sol", "probe", 5)
	if err == nil || err.Error() != "上游直连接口响应包含尾随数据" {
		t.Fatalf("trailing JSON was accepted: %v", err)
	}
}

func TestDirectRequestErrorSummarizesCloudflareHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`<!DOCTYPE html><html><title>Attention Required! | Cloudflare</title><body>Sorry, you have been blocked</body></html>`))
	}))
	defer server.Close()
	sender := directBundleSender{
		client:     server.Client(),
		credential: directCredential{BaseURL: server.URL, Secret: "key", Platform: "openai"},
	}
	_, _, err := sender.Send(context.Background(), "16", "gpt-5.6-sol", "probe", 5)
	if err == nil || !strings.Contains(err.Error(), "Cloudflare/WAF") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "<!doctype") {
		t.Fatalf("HTML leaked into error: %q", err.Error())
	}
}

func TestDirectRequestRetriesAuthHostAfterCloudflareBlock(t *testing.T) {
	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		primaryCalls++
		writer.Header().Set("Content-Type", "text/html")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`<!DOCTYPE html><html><title>Cloudflare</title><body>Sorry, you have been blocked</body></html>`))
	}))
	defer primary.Close()
	fallbackCalls := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fallbackCalls++
		if request.URL.Path != "/v1/responses" || request.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("unexpected fallback request path=%q auth=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"model":"gpt-5.6-sol","output_text":"fallback answer"}`))
	}))
	defer fallback.Close()

	sender := directBundleSender{
		client: primary.Client(),
		credential: directCredential{
			BaseURL: primary.URL, FallbackBaseURL: fallback.URL, Secret: "key", Platform: "openai",
		},
	}
	text, model, err := sender.Send(context.Background(), "16", "gpt-5.6-sol", "probe", 5)
	if err != nil {
		t.Fatal(err)
	}
	if text != "fallback answer" || model != "gpt-5.6-sol" || primaryCalls != 1 || fallbackCalls != 1 {
		t.Fatalf("text=%q model=%q primary=%d fallback=%d", text, model, primaryCalls, fallbackCalls)
	}
}

func TestDirectFallbackBaseURLUsesPrimarySchemeAndAuthHost(t *testing.T) {
	if got := directFallbackBaseURL("https://cdn.example/v1", "origin.example:8443"); got != "https://origin.example:8443" {
		t.Fatalf("fallback=%q", got)
	}
	if got := directFallbackBaseURL("http://192.0.2.1:8080/v1", "http://192.0.2.1:8080"); got != "" {
		t.Fatalf("same-host fallback=%q", got)
	}
}

func TestDirectAccountSelectionRequiresOneUsableBinding(t *testing.T) {
	baseURL := "https://api.example/v1"
	platform := "openai"
	group7, group8 := "7", "8"
	row := businessAccountStatus("41", "account")

	missing := directAccountSelection(row, &business.AccountDetail{
		AccountStatus: business.AccountStatus{BaseURL: &baseURL, Platform: &platform},
	})
	if missing.CredentialError != "账号没有可用于直连检测的有效 Key 绑定" {
		t.Fatalf("missing error=%q", missing.CredentialError)
	}

	ambiguous := directAccountSelection(row, &business.AccountDetail{
		AccountStatus: business.AccountStatus{BaseURL: &baseURL, Platform: &platform},
		Bindings: []business.AccountBinding{
			{UpstreamHost: "one.example", UpstreamKeyID: "11", UpstreamGroupID: &group7},
			{UpstreamHost: "two.example", UpstreamKeyID: "12", UpstreamGroupID: &group8},
		},
	})
	if ambiguous.CredentialError != "账号存在多个不同 Key 绑定，无法确定直连凭据" {
		t.Fatalf("ambiguous error=%q", ambiguous.CredentialError)
	}

	sourceHost := "auth.example"
	valid := directAccountSelection(row, &business.AccountDetail{
		AccountStatus: business.AccountStatus{BaseURL: &baseURL, Platform: &platform},
		Bindings: []business.AccountBinding{
			{UpstreamHost: "alias.example", SourceAuthHost: &sourceHost, UpstreamKeyID: "11", UpstreamGroupID: &group7},
			{UpstreamHost: "alias.example", SourceAuthHost: &sourceHost, UpstreamKeyID: "11", UpstreamGroupID: &group7},
		},
	})
	if valid.CredentialError != "" || valid.AuthHost != sourceHost || valid.UpstreamKeyID != "11" || valid.BaseURL != baseURL {
		t.Fatalf("valid selection=%#v", valid)
	}
}

func businessAccountStatus(id, name string) business.AccountStatus {
	return business.AccountStatus{ID: id, Name: name}
}
