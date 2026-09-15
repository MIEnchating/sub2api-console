package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestOAuthProxyRejectsPrivateDestinationBeforeSendingCredentials(t *testing.T) {
	service := oauthService(t, func(*http.Request) (*http.Response, error) {
		t.Error("invalid proxy reached the transport")
		return nil, errors.New("unexpected request")
	})
	_, err := service.CheckOAuthWithProxy(context.Background(), "41", "隔离账号", map[string]any{"access_token": oauthFixtureToken}, "fixture-sol", 5, "http://private-password@127.0.0.1:8080")
	if err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatal("private proxy accepted or exposed its credentials")
	}
}

func TestOAuthProxyKeepsOfficialEndpointAndOmitsProxyCredentialsFromResult(t *testing.T) {
	service := oauthService(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://chatgpt.com/backend-api/codex/responses" || r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy changed official request boundary")
		}
		return oauthResponse(http.StatusOK, "application/json", `{"status":"completed","output_text":"[\"7\"]"}`), nil
	})
	result, err := service.CheckOAuthWithProxy(context.Background(), "41", "隔离账号", map[string]any{"access_token": oauthFixtureToken}, "fixture-sol", 5, "http://operator:private-password@proxy.example.com:8080")
	if err != nil || result["transport"] != "oauth-proxy" || result["verdict"] != "SOL_CONSISTENT" {
		t.Fatalf("proxied profile result: %v %v", result, err)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "private-password") || strings.Contains(string(raw), "proxy.example.com") {
		t.Fatal("private proxy escaped into public result")
	}
}
