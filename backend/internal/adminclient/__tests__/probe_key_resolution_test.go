package adminclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func TestProbeResolvesHiddenKeyWithoutForwardingCredentialStatus(t *testing.T) {
	for _, masked := range []string{"", "sk-********"} {
		t.Run(masked, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer resolved-private" || r.Header.Get("X-API-Key") != "" {
					t.Error("wrong generation authentication")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
			}))
			defer upstream.Close()
			management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/104" {
					t.Errorf("unexpected management request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				creds := map[string]any{"base_url": upstream.URL}
				if masked != "" {
					creds["api_key"] = masked
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 104, "type": "apikey", "platform": "openai", "credentials": creds, "credentials_status": map[string]bool{"has_api_key": true}}})
			}))
			defer management.Close()
			client, err := adminclient.New(adminclient.Config{BaseURL: management.URL, AdminKey: "admin-private", Attempts: 1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := client.PrepareAccountProbe(context.Background(), "104", func(_ context.Context, id string, source adminclient.ProbeCredentialSource) (string, error) {
				if id != "104" || source.BindingBaseURL != upstream.URL || source.GenerationBaseURL != upstream.URL {
					t.Errorf("credential lookup scope mismatch id=%s source=%+v", id, source)
				}
				return "resolved-private", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			response, err := prepared.Open(context.Background(), "model", "hi")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if calls.Load() != 1 {
				t.Fatalf("generation calls=%d", calls.Load())
			}
		})
	}
}

func TestProbeDoesNotUseCachedKeyWhenManagementConfirmsKeyRemoved(t *testing.T) {
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 104, "type": "apikey", "platform": "openai", "credentials": map[string]any{"base_url": "https://unused.invalid"}, "credentials_status": map[string]bool{"has_api_key": false}}})
	}))
	defer management.Close()
	client, err := adminclient.New(adminclient.Config{BaseURL: management.URL, AdminKey: "test-admin", Attempts: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	_, err = client.PrepareAccountProbe(context.Background(), "104", func(context.Context, string, adminclient.ProbeCredentialSource) (string, error) {
		called = true
		return "stale-key", nil
	})
	var unavailable *adminclient.ProbeUnavailableError
	if !errors.As(err, &unavailable) || called {
		t.Fatalf("removed credential used: err=%v called=%v", err, called)
	}
}

func TestProbeResolverUnexpectedErrorIsSafeAndDoesNotBecomeHealthFailure(t *testing.T) {
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 104, "type": "apikey", "platform": "openai", "credentials": map[string]any{"base_url": "https://unused.invalid"}, "credentials_status": map[string]bool{"has_api_key": true}}})
	}))
	defer management.Close()
	client, err := adminclient.New(adminclient.Config{BaseURL: management.URL, AdminKey: "test-admin", Attempts: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.PrepareAccountProbe(context.Background(), "104", func(context.Context, string, adminclient.ProbeCredentialSource) (string, error) {
		return "", errors.New("private-credential from database")
	})
	var unavailable *adminclient.ProbeUnavailableError
	if !errors.As(err, &unavailable) || strings.Contains(err.Error(), "private-credential") {
		t.Fatalf("unsafe resolver error: %v", err)
	}
}
