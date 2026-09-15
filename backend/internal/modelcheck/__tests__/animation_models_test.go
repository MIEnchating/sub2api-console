package modelcheck_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestCustomAnimationModelsUsesSuppliedCredentialsAndNormalizesList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/proxy/v1/models" || r.Header.Get("Authorization") != "Bearer "+fixtureSecret {
			t.Error("unexpected model discovery request")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"beta"},{"id":"alpha"},{"id":" beta "}]}`))
	}))
	defer server.Close()
	service := new(modelcheck.Service)
	models, err := service.CustomAnimationModels(context.Background(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL + "/proxy/v1/", APIKey: fixtureSecret, Platform: "openai"})
	if err != nil || !reflect.DeepEqual(models, []string{"alpha", "beta"}) {
		t.Fatalf("models=%v err=%v", models, err)
	}
}

func TestCustomAnimationModelsReadsAnthropicPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("x-api-key") != fixtureSecret || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected Anthropic model discovery request")
		}
		switch r.URL.Query().Get("after_id") {
		case "":
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-a"}],"has_more":true,"last_id":"claude-a"}`))
		case "claude-a":
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-b"}],"has_more":false}`))
		default:
			t.Error("unexpected pagination cursor")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	models, err := new(modelcheck.Service).CustomAnimationModels(context.Background(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL, APIKey: fixtureSecret, Platform: "anthropic"})
	if err != nil || !reflect.DeepEqual(models, []string{"claude-a", "claude-b"}) {
		t.Fatalf("models=%v err=%v", models, err)
	}
}

func TestCustomAnimationModelsRejectsInvalidUpstreamPayloads(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"missing data", `{}`},
		{"null data", `{"data":null}`},
		{"invalid model", `{"data":[{"id":""}]}`},
		{"business failure", `{"data":[],"success":false}`},
		{"business error", `{"data":[],"error":{"message":"failure"}}`},
		{"trailing JSON", `{"data":[]} {}`},
		{"secret in model", fmt.Sprintf(`{"data":[{"id":%q}]}`, fixtureSecret)},
		{"missing cursor", `{"data":[{"id":"model"}],"has_more":true}`},
		{"repeated cursor", `{"data":[{"id":"model"}],"has_more":true,"last_id":"same"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(test.body)) }))
			defer server.Close()
			models, err := new(modelcheck.Service).CustomAnimationModels(context.Background(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL, APIKey: fixtureSecret, Platform: "openai"})
			if err == nil || len(models) != 0 || strings.Contains(err.Error(), fixtureSecret) {
				t.Fatalf("invalid response accepted or secret exposed: models=%v err=%v", models, err)
			}
		})
	}
}

func TestCustomAnimationModelsAcceptsEmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":[]}`)) }))
	defer server.Close()
	models, err := new(modelcheck.Service).CustomAnimationModels(context.Background(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL, APIKey: fixtureSecret, Platform: "openai"})
	if err != nil || models == nil || len(models) != 0 {
		t.Fatalf("empty list must be returned: %v %v", models, err)
	}
}

func TestCustomAnimationModelsDoesNotFollowRedirects(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("credentials must not be forwarded to a redirect")
	}))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := new(modelcheck.Service).CustomAnimationModels(context.Background(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL, APIKey: fixtureSecret, Platform: "openai"})
	if err == nil {
		t.Fatal("redirect must fail")
	}
}

func TestCustomAnimationModelsRedactsAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(w, `{"error":{"message":%q}}`, "invalid key: "+fixtureSecret)
	}))
	defer server.Close()
	_, err := new(modelcheck.Service).CustomAnimationModels(context.Background(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL, APIKey: fixtureSecret, Platform: "openai"})
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), fixtureSecret) {
		t.Fatalf("unsafe or missing error: %v", err)
	}
}

func TestCustomAnimationModelsRetriesTransientFailureOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"recovered-model"}]}`))
	}))
	defer server.Close()
	models, err := new(modelcheck.Service).CustomAnimationModels(context.Background(), modelcheck.AnimationCustomEndpoint{BaseURL: server.URL, APIKey: fixtureSecret, Platform: "openai"})
	if err != nil || !reflect.DeepEqual(models, []string{"recovered-model"}) || calls.Load() != 2 {
		t.Fatalf("recovery failed: %v %v", models, err)
	}
}

func TestCustomAnimationModelsRejectsInvalidConnectionBeforeRequest(t *testing.T) {
	for _, input := range []modelcheck.AnimationCustomEndpoint{
		{BaseURL: "https://user:pass@example.invalid", APIKey: fixtureSecret, Platform: "openai"},
		{BaseURL: "https://example.invalid", APIKey: "", Platform: "openai"},
		{BaseURL: "https://example.invalid", APIKey: "bad\r\nkey", Platform: "openai"},
		{BaseURL: "https://example.invalid", APIKey: fixtureSecret, Platform: "invalid"},
	} {
		if _, err := new(modelcheck.Service).CustomAnimationModels(context.Background(), input); err == nil {
			t.Fatal("invalid connection accepted")
		}
	}
}
