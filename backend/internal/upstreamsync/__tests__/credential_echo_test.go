package upstreamsync_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestUpstreamFailureDoesNotExposeCredentialEchoedAsOrdinaryText(t *testing.T) {
	const secret = "fixture-echo-private-value"
	for _, scenario := range []struct {
		name, mode, prefix string
		status             int
		escaped            bool
	}{
		{"access token business error", "access", "", 200, false},
		{"access token HTTP error", "access", "", 403, false},
		{"admin key", "admin", "", 200, false},
		{"session cookie", "cookie", "", 200, false},
		{"custom header", "header", "", 200, false},
		{"custom bearer header", "bearer", "", 200, false},
		{"escaped value", "access", "", 200, true},
		{"truncated error", "access", strings.Repeat("x", 275), 200, false},
		{"overlapping credentials", "overlap", "", 200, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := json.Marshal(map[string]any{
					"success": false, "code": "AUTH_TOKEN_EXPIRED", "message": "rejected " + scenario.prefix + secret,
				})
				if err != nil {
					t.Error(err)
					return
				}
				if scenario.escaped {
					body = []byte(strings.ReplaceAll(string(body), "-", `\u002d`))
				}
				w.WriteHeader(scenario.status)
				_, _ = w.Write(body)
			}))
			t.Cleanup(server.Close)
			record := configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "newapi"}
			value := secret
			switch scenario.mode {
			case "admin":
				record.AuthMode, record.AdminKey = "newapi_admin_key", &value
			case "cookie":
				record.AuthMode, record.Cookies = "newapi_session", map[string]string{"session": value}
			case "header":
				record.AuthMode, record.Headers = "custom_headers", map[string]string{"X-API-Key": value}
			case "bearer":
				record.AuthMode, record.Headers = "custom_headers", map[string]string{"Authorization": "Bearer " + value}
			case "overlap":
				prefix := "fixture-echo"
				record.AccessToken, record.RefreshToken = &prefix, &value
			default:
				record.AccessToken = &value
			}

			_, err := upstreamsync.NewReader(server.Client()).ListKeys(t.Context(), record)

			if err == nil || !strings.Contains(err.Error(), "rejected") {
				t.Fatal("failure must retain an actionable reason")
			}
			if strings.Contains(err.Error(), "fixture-echo") || strings.Contains(err.Error(), "private-value") {
				t.Fatal("upstream error exposes a credential or its truncated remainder")
			}
			if !upstreamsync.IsAuthenticationError(err) {
				t.Fatal("redaction changed authentication classification")
			}
			if scenario.status != http.StatusOK {
				var statusError *upstreamsync.StatusError
				if !errors.As(err, &statusError) || statusError.StatusCode != scenario.status {
					t.Fatal("redaction lost the HTTP error contract")
				}
			}
		})
	}
}
