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
	if transport.requests[0].Header.Get("Authorization") != "" || transport.requests[0].Header.Get("New-Api-User") != "" {
		t.Fatalf("login reused stale authentication headers: %#v", transport.requests[0].Header)
	}
}

func TestSub2APILoginRequiresExplicitConsentForLatestLoginAgreement(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"code":"LOGIN_AGREEMENT_REQUIRED","message":"please read and accept the latest login agreement before signing in"}`},
		statuses:  []int{http.StatusForbidden},
	}
	client := New(&http.Client{Transport: transport})
	username, password := "user@example.com", "secret"

	_, err := client.Login(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}})

	var interaction *InteractionError
	if !errors.As(err, &interaction) || interaction.Code != "login_agreement_required" {
		t.Fatalf("agreement response was not classified: %v", err)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("login retried without explicit consent: %d requests", len(transport.requests))
	}
}

func TestSub2APILoginDoesNotRetryUnrelatedForbiddenResponseWithAgreementConsent(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"code":"ACCOUNT_DISABLED","message":"account is disabled"}`},
		statuses:  []int{http.StatusForbidden},
	}
	client := New(&http.Client{Transport: transport})
	username, password := "user@example.com", "secret"

	_, err := client.LoginWithOptions(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}}, LoginOptions{
		AcceptLoginAgreement: true,
	})

	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusForbidden {
		t.Fatalf("unrelated forbidden response was reclassified: %v", err)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("unrelated forbidden response triggered agreement retry: %d requests", len(transport.requests))
	}
}

func TestSub2APILoginRetriesWithLatestAgreementRevisionAfterExplicitConsent(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{
			`{"code":"LOGIN_AGREEMENT_REQUIRED","message":"please read and accept the latest login agreement before signing in"}`,
			`{"code":0,"message":"success","data":{"login_agreement_enabled":true,"login_agreement_revision":"a90464c54fba46d4"}}`,
			`{"code":0,"data":{"access_token":"new-token","refresh_token":"new-refresh"}}`,
			`{"code":0,"data":{"id":9}}`,
		},
		statuses: []int{http.StatusForbidden, http.StatusOK, http.StatusOK, http.StatusOK},
	}
	client := New(&http.Client{Transport: transport})
	username, password := "user@example.com", "secret"

	record, err := client.LoginWithOptions(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}}, LoginOptions{
		AcceptLoginAgreement: true,
	})

	if err != nil {
		t.Fatal(err)
	}
	if record.AccessToken == nil || *record.AccessToken != "new-token" {
		t.Fatalf("unexpected login result: %#v", record)
	}
	wantPaths := []string{"/api/v1/auth/login", "/api/v1/settings/public", "/api/v1/auth/login", "/api/v1/user/profile"}
	if len(transport.requests) != len(wantPaths) {
		t.Fatalf("requests=%d want=%d", len(transport.requests), len(wantPaths))
	}
	for index, path := range wantPaths {
		if transport.requests[index].URL.Path != path {
			t.Fatalf("request %d path=%q want=%q", index, transport.requests[index].URL.Path, path)
		}
	}
	body, err := io.ReadAll(transport.requests[2].Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"email":"user@example.com","login_agreement_revision":"a90464c54fba46d4","password":"secret"}` {
		t.Fatalf("unexpected agreement login body: %s", body)
	}
}

func TestSub2APILoginAcceptsRequiredAdminComplianceBeforeRetryingVerification(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{
			`{"code":0,"data":{"access_token":"new-token","refresh_token":"new-refresh"}}`,
			`{"message":"please read"}`,
			`{"code":0,"data":{"required":true,"version":"v2026.06.10","ack_phrase_zh":"我已阅读并同意当前协议"}}`,
			`{"code":0,"data":{"required":false,"version":"v2026.06.10"}}`,
			`{"code":0,"data":{"id":9}}`,
		},
		statuses: []int{http.StatusOK, http.StatusForbidden, http.StatusOK, http.StatusOK, http.StatusOK},
	}
	client := New(&http.Client{Transport: transport})
	username, password := "user@example.com", "secret"

	_, err := client.Login(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}})

	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodGet, "/api/v1/user/profile"},
		{http.MethodGet, "/api/v1/admin/compliance"},
		{http.MethodPost, "/api/v1/admin/compliance/accept"},
		{http.MethodGet, "/api/v1/user/profile"},
	}
	if len(transport.requests) != len(want) {
		t.Fatalf("requests=%d want=%d", len(transport.requests), len(want))
	}
	for index, expected := range want {
		request := transport.requests[index]
		if request.Method != expected.method || request.URL.Path != expected.path {
			t.Fatalf("request %d=%s %s", index, request.Method, request.URL.Path)
		}
		if index > 0 && request.Header.Get("Authorization") != "Bearer new-token" {
			t.Fatalf("request %d did not use recovered token: %#v", index, request.Header)
		}
	}
	body, err := io.ReadAll(transport.requests[3].Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"language":"zh","phrase":"我已阅读并同意当前协议"}` {
		t.Fatalf("unexpected compliance acceptance body: %s", body)
	}
}

func TestAdminComplianceRequiredRecognizesCurrentSub2APIContract(t *testing.T) {
	err := &HTTPError{
		StatusCode: http.StatusLocked,
		Detail:     "administrator compliance acknowledgement is required；ADMIN_COMPLIANCE_ACK_REQUIRED",
	}

	if !adminComplianceRequired(err) {
		t.Fatal("current Sub2API compliance response was not recognized")
	}
}

func TestSub2APILoginDoesNotAcceptComplianceForUnrelatedForbiddenResponse(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{
			`{"code":0,"data":{"access_token":"new-token","refresh_token":"new-refresh"}}`,
			`{"code":"ACCOUNT_DISABLED","message":"account disabled"}`,
		},
		statuses: []int{http.StatusOK, http.StatusForbidden},
	}
	client := New(&http.Client{Transport: transport})
	username, password := "user@example.com", "secret"

	_, err := client.Login(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}})

	if err == nil || !strings.Contains(err.Error(), "ACCOUNT_DISABLED") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(transport.requests) != 2 {
		t.Fatalf("unrelated 403 triggered compliance requests: %d", len(transport.requests))
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

func TestSub2APIRefreshUsesEmptyBodyAndRefreshCookie(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"code":0,"data":{"access_token":"rotated"}}`, `{"code":0,"data":{"id":7}}`},
	}
	client := New(&http.Client{Transport: transport})
	access, refresh := "expired", "refresh-token"
	_, err := client.Refresh(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &access, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := transport.requests[0]
	if request.URL.Path != "/api/v1/auth/refresh" || request.Body != nil {
		t.Fatalf("unexpected refresh request: path=%s body=%#v", request.URL.Path, request.Body)
	}
	if cookie, err := request.Cookie("sub2api_refresh_token"); err != nil || cookie.Value != "refresh-token" {
		t.Fatalf("refresh cookie=%#v err=%v", cookie, err)
	}
}

func TestSub2APIRefreshRetriesLegacyJSONBodyAfterExplicitEOF(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{
			`{"code":400,"message":"Invalid request: EOF"}`,
			`{"code":0,"data":{"access_token":"rotated"}}`,
			`{"code":0,"data":{"id":7}}`,
		},
		statuses: []int{http.StatusBadRequest, http.StatusOK, http.StatusOK},
	}
	client := New(&http.Client{Transport: transport})
	access, refresh := "expired", "refresh-token"
	_, err := client.Refresh(context.Background(), configstore.AuthRecord{
		Host: "legacy.example", BaseURL: "https://legacy.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &access, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(transport.requests) != 3 || transport.requests[0].Body != nil || transport.requests[1].Body == nil {
		t.Fatalf("unexpected compatibility sequence: %#v", transport.requests)
	}
	body, err := io.ReadAll(transport.requests[1].Body)
	if err != nil || string(body) != `{"refresh_token":"refresh-token"}` {
		t.Fatalf("legacy body=%q err=%v", body, err)
	}
	for _, request := range transport.requests[:2] {
		cookie, err := request.Cookie("sub2api_refresh_token")
		if err != nil || cookie.Value != "refresh-token" {
			t.Fatalf("refresh cookie=%#v err=%v", cookie, err)
		}
	}
}

func TestSub2APIRefreshDoesNotRetryLegacyBodyForCredentialRejection(t *testing.T) {
	transport := &recordingTransport{
		responses: []string{`{"code":400,"message":"refresh token invalid"}`},
		statuses:  []int{http.StatusBadRequest},
	}
	client := New(&http.Client{Transport: transport})
	access, refresh := "expired", "refresh-token"
	_, err := client.Refresh(context.Background(), configstore.AuthRecord{
		Host: "api.example", BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: &access, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err == nil || len(transport.requests) != 1 {
		t.Fatalf("err=%v requests=%d", err, len(transport.requests))
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
