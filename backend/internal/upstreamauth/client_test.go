package upstreamauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type recordingTransport struct {
	requests  []*http.Request
	responses []string
	cookies   [][]string
	statuses  []int
}

func (t *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.requests = append(t.requests, request)
	response := t.responses[0]
	t.responses = t.responses[1:]
	status := http.StatusOK
	if len(t.statuses) > 0 {
		status = t.statuses[0]
		t.statuses = t.statuses[1:]
	}
	headers := make(http.Header)
	if len(t.cookies) > 0 {
		for _, cookie := range t.cookies[0] {
			headers.Add("Set-Cookie", cookie)
		}
		t.cookies = t.cookies[1:]
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(response)), Header: headers, Request: request}, nil
}

func TestValidateRecordAcceptsCustomHeadersInsteadOfModeCredentials(t *testing.T) {
	tests := []configstore.AuthRecord{
		{UpstreamType: "sub2api", AuthMode: "sub2api_user_token"},
		{UpstreamType: "newapi", AuthMode: "newapi_admin_key"},
		{UpstreamType: "newapi", AuthMode: "newapi_user_token"},
	}
	for _, record := range tests {
		record.Headers = map[string]string{"X-API-Key": "header-secret"}
		if err := ValidateRecord(record); err != nil {
			t.Fatalf("mode %s rejected header authentication: %v", record.AuthMode, err)
		}
	}
}

func TestValidateRecordStillRejectsMissingCredentialsAndHeaders(t *testing.T) {
	err := ValidateRecord(configstore.AuthRecord{UpstreamType: "sub2api", AuthMode: "sub2api_user_token"})
	if err == nil || !strings.Contains(err.Error(), "Token 和 Refresh Token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyUsesHeaderAuthenticationWithoutSynthesizingCredentials(t *testing.T) {
	transport := &recordingTransport{responses: []string{`{"success":true,"data":{"id":7}}`}}
	client := New(&http.Client{Transport: transport})
	err := client.Verify(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_admin_key",
		Headers: map[string]string{"X-API-Key": "header-secret"}, Cookies: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := transport.requests[0]
	if request.Header.Get("X-API-Key") != "header-secret" || request.Header.Get("Authorization") != "" || request.Header.Get("New-Api-User") != "" {
		t.Fatalf("unexpected request headers: %#v", request.Header)
	}
}

func TestVerifyDoesNotTreatPresentHeadersAsSuccessfulAuthentication(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"message":"unauthorized"}`},
		statuses:  []int{http.StatusUnauthorized},
	}
	client := New(&http.Client{Transport: transport})
	err := client.Verify(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		Headers: map[string]string{"X-API-Key": "invalid"}, Cookies: map[string]string{},
	})
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusUnauthorized {
		t.Fatalf("header authentication was not verified upstream: %v", err)
	}
}

func TestAuthenticationHTTPErrorKeepsUsefulReasonAndRedactsSecrets(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"code":"CREDENTIAL_EXPIRED","message":"凭据包时间戳已过期","reason":"password=must-not-leak","access_token":"also-secret"}`},
		statuses:  []int{http.StatusBadRequest},
	}
	client := New(&http.Client{Transport: transport})
	token, refresh := "expired", "refresh"
	err := client.Verify(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &token, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err == nil || !strings.Contains(err.Error(), "凭据包时间戳已过期") || !strings.Contains(err.Error(), "CREDENTIAL_EXPIRED") {
		t.Fatalf("useful upstream reason was lost: %v", err)
	}
	if strings.Contains(err.Error(), "must-not-leak") || strings.Contains(err.Error(), "also-secret") {
		t.Fatalf("upstream error leaked a secret: %v", err)
	}
}

func TestAuthenticationHTTPErrorDoesNotEchoNonJSONBody(t *testing.T) {
	transport := &recordingTransport{responses: []string{"gateway dumped bearer super-secret-token"}, statuses: []int{http.StatusBadGateway}}
	client := New(&http.Client{Transport: transport})
	token, refresh := "expired", "refresh"
	err := client.Verify(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &token, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err == nil || !strings.Contains(err.Error(), "上游未返回错误详情") || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("non-JSON response body was exposed: %v", err)
	}
}

func TestVerifyNewAPIAdminKeyUsesCanonicalHeaders(t *testing.T) {
	transport := &recordingTransport{responses: []string{`{"success":true,"data":{"id":7}}`}}
	client := New(&http.Client{Transport: transport})
	adminKey, userID := "admin", "7"
	err := client.Verify(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_admin_key",
		AdminKey: &adminKey, UserID: &userID, Headers: map[string]string{"X-Site": "custom"}, Cookies: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := transport.requests[0]
	if request.URL.Path != "/api/user/self" || request.Header.Get("Authorization") != "Bearer admin" || request.Header.Get("New-Api-User") != "7" || request.Header.Get("X-Site") != "custom" {
		t.Fatalf("unexpected request: %#v", request.Header)
	}
}

func TestSub2APILoginUsesEmailAndVerifiesReturnedToken(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"code":0,"data":{"access_token":"new-token","refresh_token":"new-refresh"}}`, `{"code":0,"data":{"id":9}}`},
		cookies:   [][]string{{"sub2api_refresh_token=cookie-refresh; Path=/"}, nil},
	}
	client := New(&http.Client{Transport: transport})
	username, password := "user@example.com", "secret"
	record, err := client.Login(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{"X-Login": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if record.AccessToken == nil || *record.AccessToken != "new-token" || record.RefreshToken == nil || *record.RefreshToken != "cookie-refresh" {
		t.Fatalf("unexpected login result: %#v", record)
	}
	if len(transport.requests) != 2 || transport.requests[0].URL.Path != "/api/v1/auth/login" || transport.requests[1].Header.Get("Authorization") != "Bearer new-token" {
		t.Fatalf("unexpected requests: %#v", transport.requests)
	}
}

func TestNewAPILegacyLoginUsesSessionCookieAndUserIDWithoutTokens(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{
			`{"success":true,"data":{"id":24,"username":"legacy-user"}}`,
			`{"success":true,"data":{"id":24,"quota":1000}}`,
		},
		cookies: [][]string{{"session=legacy-session; Path=/; HttpOnly"}, nil},
	}
	client := New(&http.Client{Transport: transport})
	username, password, staleRefresh := "legacy-user", "secret", "stale-new-api-refresh"
	record, err := client.Login(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_manual_login",
		RefreshToken: &staleRefresh, Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if record.AccessToken != nil || record.RefreshToken != nil || record.UserID == nil || *record.UserID != "24" || record.Cookies["session"] != "legacy-session" {
		t.Fatalf("unexpected legacy login result: %#v", record)
	}
	verification := transport.requests[1]
	if verification.URL.Path != "/api/user/self" || verification.Header.Get("Authorization") != "" || verification.Header.Get("New-Api-User") != "24" {
		t.Fatalf("unexpected legacy verification headers: %#v", verification.Header)
	}
	if cookie, err := verification.Cookie("session"); err != nil || cookie.Value != "legacy-session" {
		t.Fatalf("legacy session cookie=%#v err=%v", cookie, err)
	}
}

func TestNewAPICurrentLoginExtractsNestedUserIDAndRefreshCookie(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{
			`{"success":true,"data":{"access_token":"access","user":{"id":24}}}`,
			`{"success":true,"data":{"id":24}}`,
		},
		cookies: [][]string{{"new_api_refresh=refresh; Path=/api/user/auth; HttpOnly"}, nil},
	}
	client := New(&http.Client{Transport: transport})
	username, password := "current-user", "secret"
	record, err := client.Login(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if record.AccessToken == nil || *record.AccessToken != "access" || record.RefreshToken == nil || *record.RefreshToken != "refresh" || record.UserID == nil || *record.UserID != "24" {
		t.Fatalf("unexpected current login result: %#v", record)
	}
}

func TestExplicitBusinessFailureIsRejected(t *testing.T) {
	transport := &recordingTransport{responses: []string{`{"code":false,"data":{"id":7}}`}}
	client := New(&http.Client{Transport: transport})
	token, refresh := "token", "refresh"
	err := client.Verify(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &token, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err == nil || err.Error() != "上游业务鉴权失败" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSub2APILoginClassifiesCaptchaFromHTTP200BusinessFailure(t *testing.T) {
	transport := &recordingTransport{responses: []string{`{"code":400,"message":"请输入图片验证码"}`}}
	client := New(&http.Client{Transport: transport})
	username, password := "user@example.com", "secret"
	_, err := client.Login(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}})
	var interaction *InteractionError
	if !errors.As(err, &interaction) || interaction.Code != "image_captcha_required" {
		t.Fatalf("captcha response was not classified: %v", err)
	}
}

func TestSub2APILoginClassifiesCredentialBrowserFlowAsImageCaptcha(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"code":403,"message":"browser credential flow is required","reason":"CREDENTIAL_BROWSER_FLOW_REQUIRED"}`},
		statuses:  []int{http.StatusForbidden},
	}
	client := New(&http.Client{Transport: transport})
	username, password := "user@example.com", "secret"
	_, err := client.Login(context.Background(), configstore.AuthRecord{
		Host: "www.example.test", BaseURL: "https://www.example.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}})
	var interaction *InteractionError
	if !errors.As(err, &interaction) || interaction.Code != "image_captcha_required" {
		t.Fatalf("credential browser flow was not routed to image captcha: %v", err)
	}
}

func TestRefreshUsesPlatformEndpointAndVerifiesRotatedToken(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"success":true,"data":{"access_token":"rotated"}}`, `{"success":true,"data":{"id":7}}`},
		cookies:   [][]string{{"new_api_refresh=rotated-refresh; Path=/"}, nil},
	}
	client := New(&http.Client{Transport: transport})
	access, refresh := "expired", "refresh-token"
	record, err := client.Refresh(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "newapi", AuthMode: "newapi_user_token",
		AccessToken: &access, RefreshToken: &refresh, Headers: map[string]string{"X-Site": "custom"}, Cookies: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.AccessToken == nil || *record.AccessToken != "rotated" || record.RefreshToken == nil || *record.RefreshToken != "rotated-refresh" {
		t.Fatalf("unexpected refresh result: %#v", record)
	}
	if len(transport.requests) != 2 || transport.requests[0].URL.Path != "/api/user/auth/refresh" || transport.requests[1].URL.Path != "/api/user/self" {
		t.Fatalf("unexpected refresh sequence: %#v", transport.requests)
	}
	if cookie, err := transport.requests[0].Cookie("new_api_refresh"); err != nil || cookie.Value != "refresh-token" {
		t.Fatalf("refresh cookie=%#v err=%v", cookie, err)
	}
	if transport.requests[0].Header.Get("Origin") != "https://api.example" {
		t.Fatalf("refresh request origin=%q", transport.requests[0].Header.Get("Origin"))
	}
	if transport.requests[1].Header.Get("Authorization") != "Bearer rotated" || transport.requests[1].Header.Get("X-Site") != "custom" {
		t.Fatalf("verification did not use staged credentials: %#v", transport.requests[1].Header)
	}
}

func TestRefreshRejectsResponseWithoutNewAccessTokenBeforeVerification(t *testing.T) {
	transport := &recordingTransport{responses: []string{`{"code":0,"data":{"refresh_token":"another"}}`}}
	client := New(&http.Client{Transport: transport})
	access, refresh := "expired", "refresh-token"
	_, err := client.Refresh(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &access, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err == nil || !strings.Contains(err.Error(), "缺少新的 access token") {
		t.Fatalf("err=%v", err)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("expired access token was unexpectedly verified: requests=%d", len(transport.requests))
	}
}
