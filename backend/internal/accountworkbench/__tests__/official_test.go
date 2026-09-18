package accountworkbench_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOfficialRefreshUsesFixedEndpointAndKeepsRotation(t *testing.T) {
	service, _ := fixture(t, `{}`)
	requests := 0
	service.UseOfficialTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.String() != "https://auth.openai.com/oauth/token" || request.Method != "POST" || request.GetBody != nil {
			t.Fatal("unexpected token request contract")
		}
		raw, _ := io.ReadAll(request.Body)
		form, _ := url.ParseQuery(string(raw))
		if form.Get("refresh_token") != "rt_original" || form.Get("grant_type") != "refresh_token" || form.Get("client_id") == "" {
			t.Fatal("refresh parameters missing")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"rt_rotated","token_type":"Bearer","expires_in":3600}`))}, nil
	}))
	credentials, err := service.RefreshCredential(context.Background(), "rt_original", "")
	if err != nil {
		t.Fatal(err)
	}
	expires, err := time.Parse(time.RFC3339, credentials["expires_at"].(string))
	if err != nil || !expires.After(time.Now()) || credentials["refresh_token"] != "rt_rotated" || requests != 1 {
		t.Fatal("rotation not preserved")
	}
}
func TestOfficialRefreshNeverReplaysUncertainFailureOrEchoesCredentials(t *testing.T) {
	service, _ := fixture(t, `{}`)
	requests := 0
	service.UseOfficialTransport(transportFunc(func(*http.Request) (*http.Response, error) { requests++; return nil, errors.New("rt_private-secret") }))
	_, err := service.RefreshCredential(context.Background(), "rt_private-secret", "")
	if err == nil || strings.Contains(err.Error(), "rt_private-secret") || requests != 1 {
		t.Fatal("uncertain refresh replayed or credential exposed")
	}
}
func TestOfficialRefreshRejectsRedirectAndBusinessFailureWithoutReplay(t *testing.T) {
	for _, response := range []struct {
		status int
		body   string
	}{{302, `{}`}, {200, `{"error":"private-provider-detail"}`}, {200, `{"access_token":"access-only","token_type":"Bearer"}`}, {200, `{"access_token":"a","refresh_token":"r","token_type":"Bearer","expires_in":-1}`}} {
		service, _ := fixture(t, `{}`)
		requests := 0
		service.UseOfficialTransport(transportFunc(func(*http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: response.status, Header: http.Header{"Location": []string{"https://untrusted.invalid"}}, Body: io.NopCloser(strings.NewReader(response.body))}, nil
		}))
		_, err := service.RefreshCredential(context.Background(), "rt_test", "")
		if err == nil || requests != 1 || strings.Contains(err.Error(), "private-provider-detail") {
			t.Fatal("invalid response accepted or leaked")
		}
	}
}
