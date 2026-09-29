package upstreamsync_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestNewAPIBusinessAuthenticationFailureDoesNotBecomeMissingEndpoint(t *testing.T) {
	for _, operation := range []string{"balance", "keys"} {
		t.Run(operation, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if strings.HasPrefix(r.URL.Path, "/api/v1/") {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = w.Write([]byte(`{"success":false,"message":"Unauthorized, invalid access token"}`))
			}))
			t.Cleanup(server.Close)
			reader := upstreamsync.NewReader(server.Client())
			record := configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "newapi"}
			var err error
			expectedPath := "/api/user/self"
			if operation == "keys" {
				expectedPath = "/api/token/"
				_, err = reader.ListKeys(t.Context(), record)
			} else {
				_, err = reader.ReadBalance(t.Context(), record)
			}
			if !upstreamsync.IsAuthenticationError(fmt.Errorf("sync failed: %w", err)) {
				t.Fatalf("business authentication failure was lost: %v", err)
			}
			if !strings.Contains(err.Error(), "鉴权") || !strings.Contains(err.Error(), "鉴权恢复") {
				t.Fatalf("missing actionable authentication error: %v", err)
			}
			if !slices.Equal(paths, []string{expectedPath}) {
				t.Fatalf("authentication failure tried unrelated endpoints: %v", paths)
			}
		})
	}
}

func TestNewAPIBusinessAuthenticationClassificationUsesKnownResponses(t *testing.T) {
	for _, test := range []struct {
		name     string
		platform string
		payload  string
		wantAuth bool
	}{
		{"legacy Chinese", "newapi", `{"success":false,"message":"无权进行此操作，access token 无效"}`, true},
		{"legacy OneAPI", "oneapi", `{"success":false,"message":"Unauthorized, not logged in and no access token provided"}`, true},
		{"structured expired token", "newapi", `{"success":false,"code":"AUTH_TOKEN_EXPIRED","message":"session ended"}`, true},
		{"structured revoked session", "newapi", `{"success":false,"code":"AUTH_SESSION_REVOKED"}`, true},
		{"ordinary business failure", "newapi", `{"success":false,"message":"quota exhausted"}`, false},
		{"internal auth service failure", "newapi", `{"success":false,"code":"AUTH_INTERNAL_ERROR"}`, false},
		{"unrelated platform", "sub2api", `{"success":false,"message":"Unauthorized, invalid access token"}`, false},
		{"incidental token text", "newapi", `{"success":false,"message":"cannot update token metadata"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.payload))
			}))
			t.Cleanup(server.Close)
			_, err := upstreamsync.NewReader(server.Client()).ListKeys(t.Context(), configstore.AuthRecord{
				BaseURL: server.URL, UpstreamType: test.platform,
			})
			if err == nil || upstreamsync.IsAuthenticationError(err) != test.wantAuth {
				t.Fatalf("authentication classification: want=%v err=%v", test.wantAuth, err)
			}
		})
	}
}

func TestReadFailurePreservesOriginalResponseInsteadOfFallingBack(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		detail string
	}{
		{"business failure", 200, `{"success":false,"message":"quota exhausted"}`, "quota exhausted"},
		{"rate limit", 429, `{"message":"rate limited"}`, "HTTP 429"},
		{"server failure", 503, `{"message":"temporarily unavailable"}`, "HTTP 503"},
		{"invalid response", 200, `<html>maintenance</html>`, "JSON"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/api/user/self" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)
			_, err := upstreamsync.NewReader(server.Client()).ReadBalance(t.Context(), configstore.AuthRecord{
				BaseURL: server.URL, UpstreamType: "newapi",
			})
			if err == nil || !strings.Contains(err.Error(), test.detail) || upstreamsync.IsAuthenticationError(err) {
				t.Fatalf("original error was lost or misclassified: %v", err)
			}
			if !slices.Equal(paths, []string{"/api/user/self"}) {
				t.Fatalf("non-404 response triggered fallback: %v", paths)
			}
		})
	}
}

func TestBusinessFailureDetailsRedactSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false, "message": "quota exhausted; Authorization: Bearer sk-test-private-credential",
		})
	}))
	t.Cleanup(server.Close)
	_, err := upstreamsync.NewReader(server.Client()).ListKeys(t.Context(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi",
	})
	if err == nil || !strings.Contains(err.Error(), "quota exhausted") || strings.Contains(err.Error(), "sk-test-private-credential") {
		t.Fatalf("business failure detail was lost or exposed a secret: %v", err)
	}
}

func TestMissingPrimaryEndpointStillFallsBackToLegacyBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/profile" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"quota":1000000}}`))
	}))
	t.Cleanup(server.Close)
	result, err := upstreamsync.NewReader(server.Client()).ReadBalance(t.Context(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi",
	})
	if err != nil || result.RawBalance == nil || *result.RawBalance != "2" {
		t.Fatalf("404 compatibility failed: result=%+v err=%v", result, err)
	}
}

func TestHTTPAuthenticationFailureRetainsStatusClassification(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)
			_, err := upstreamsync.NewReader(server.Client()).ListKeys(t.Context(), configstore.AuthRecord{
				BaseURL: server.URL, UpstreamType: "newapi",
			})
			var statusErr *upstreamsync.StatusError
			if !upstreamsync.IsAuthenticationError(err) || !errors.As(err, &statusErr) || statusErr.StatusCode != status {
				t.Fatalf("HTTP authentication status was lost: %v", err)
			}
		})
	}
}

func TestCombinedReadErrorsRetainAuthenticationFailureAfterBusinessFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		message := "quota exhausted"
		if r.URL.Path == "/api/user/self" {
			message = "Unauthorized, invalid access token"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": message})
	}))
	t.Cleanup(server.Close)
	reader := upstreamsync.NewReader(server.Client())
	record := configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "newapi"}
	_, keyErr := reader.ListKeys(t.Context(), record)
	_, balanceErr := reader.ReadBalance(t.Context(), record)
	if !upstreamsync.IsAuthenticationError(errors.Join(keyErr, balanceErr)) {
		t.Fatalf("earlier business failure masked authentication failure: %v; %v", keyErr, balanceErr)
	}
}
