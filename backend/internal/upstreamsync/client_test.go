package upstreamsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestReaderNormalizesNewAPINamedGroupsKeysAndBalanceWithoutFloat64(t *testing.T) {
	var tokenPages int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/status" && request.Header.Get("Authorization") != "Bearer token" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"auto":{"ratio":1},"pro":{"desc":"stable","ratio":0.2}}}`))
		case "/api/token/":
			tokenPages++
			if request.URL.Query().Get("page") == "1" {
				_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":17,"name":"key","group":"pro","rate":0.2}],"total":1}}`))
			} else {
				_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[]}}`))
			}
		case "/api/user/self":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota":1000000,"used_quota":999999999}}`))
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"system_name":"Example","quota_per_unit":500000}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token := "token"
	record := configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token", AccessToken: &token, Headers: map[string]string{}, Cookies: map[string]string{}}
	reader := NewReader(server.Client())
	catalog, err := reader.ReadCatalog(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Groups) != 1 || catalog.Groups[0].GroupID != "pro" || catalog.Groups[0].RawRate == nil || *catalog.Groups[0].RawRate != "0.2" {
		t.Fatalf("groups=%#v", catalog.Groups)
	}
	if len(catalog.Keys) != 1 || catalog.Keys[0].KeyID != "17" || catalog.Keys[0].UpstreamGroup == nil || *catalog.Keys[0].UpstreamGroup != "pro" {
		t.Fatalf("keys=%#v", catalog.Keys)
	}
	if tokenPages != 1 {
		t.Fatalf("declared total did not stop pagination: pages=%d", tokenPages)
	}
	balance, err := reader.ReadBalance(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if balance.RawBalance == nil || *balance.RawBalance != "2" || balance.QuotaPerUnit == nil || *balance.QuotaPerUnit != "500000" || balance.BalanceUnit == nil || *balance.BalanceUnit != "usd" {
		t.Fatalf("balance=%#v", balance)
	}
}

func TestReadSiteNameUsesOnlyPublicConfiguration(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Path != "/api/status" || request.Header.Get("Authorization") != "" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"data":{"system_name":"New API"}}`))
	}))
	defer server.Close()
	name, err := NewReader(server.Client()).ReadSiteName(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || name == nil || *name != "New API" {
		t.Fatalf("requests=%d name=%v", requests, name)
	}
}

func TestReaderUsesLegacyNewAPISessionCookieAndRequiredUserHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, cookieErr := request.Cookie("session")
		if cookieErr != nil || cookie.Value != "legacy-session" || request.Header.Get("New-Api-User") != "24" || request.Header.Get("Authorization") != "" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"pro":{"ratio":0.2}}}`))
		case "/api/token/":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[],"total":0}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	userID := "24"
	record := configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_manual_login", UserID: &userID,
		Headers: map[string]string{}, Cookies: map[string]string{"session": "legacy-session"},
	}
	catalog, err := NewReader(server.Client()).ReadCatalog(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Groups) != 1 || catalog.Groups[0].GroupID != "pro" {
		t.Fatalf("catalog=%#v", catalog)
	}
}

func TestReaderFallsBackOn404AndStopsRepeatedLegacyKeyPage(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/groups/available", "/api/v1/keys":
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":"missing"}`))
		case "/api/v1/admin/groups":
			_, _ = writer.Write([]byte(`{"code":0,"data":[{"id":7,"name":"pro","rate":"0.3"}]}`))
		case "/api/token/":
			pages++
			_, _ = writer.Write([]byte(`{"code":0,"items":[{"id":1,"name":"legacy","group_id":7}]}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token, refresh := "token", "refresh"
	record := configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token, RefreshToken: &refresh, Headers: map[string]string{}, Cookies: map[string]string{}}
	catalog, err := NewReader(server.Client()).ReadCatalog(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Groups) != 1 || len(catalog.Keys) != 1 || pages != 2 {
		t.Fatalf("catalog=%#v pages=%d", catalog, pages)
	}
}

func TestReaderExplainsMissingKeyListEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/self/groups":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"pro":{"ratio":1}}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token := "token"
	_, err := NewReader(server.Client()).ReadCatalog(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err == nil || err.Error() != "未找到上游 Key 列表接口，请检查平台类型或上游版本（HTTP 404，/api/v1/keys）" {
		t.Fatalf("err=%v", err)
	}
}

func TestReaderClassifiesAuthenticationFailureWithoutReturningResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"token":"super-secret-value"}`))
	}))
	defer server.Close()
	token := "expired"
	_, err := NewReader(server.Client()).ReadCatalog(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if !IsAuthenticationError(err) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("err=%v", err)
	}
}

func TestReaderRejectsExplicitBusinessFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": false, "data": map[string]any{"pro": map[string]any{"ratio": 1}}})
	}))
	defer server.Close()
	token := "token"
	_, err := NewReader(server.Client()).ReadCatalog(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	})
	if err == nil || err.Error() != "上游业务读取失败" {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateNewAPIKeyVerifiesInventoryAndRevealsSecret(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authorization") != "Bearer admin" || request.Header.Get("New-Api-User") != "24" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/token/":
			items := `[]`
			if created {
				items = `[{"id":91,"name":"pro-key","group":"pro","status":1}]`
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":` + items + `,"total":` + map[bool]string{false: "0", true: "1"}[created] + `}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/token/":
			created = true
			_, _ = writer.Write([]byte(`{"success":true,"data":{"id":91}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/token/91/key":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"key":"sk-once"}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	admin, userID := "admin", "24"
	record := configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_admin_key", AdminKey: &admin, UserID: &userID,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}
	key, err := NewReader(server.Client()).CreateKey(context.Background(), record, "pro-key", "pro")
	if err != nil {
		t.Fatal(err)
	}
	if key.KeyID != "91" || key.GroupID != "pro" || key.Name != "pro-key" || key.Secret != "sk-once" {
		t.Fatalf("key=%#v", key)
	}
}

func TestCreateSub2APIKeyUsesNumericGroupIDAndVerifiesGroup(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/keys":
			if created {
				_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":17,"name":"codex-key","group_id":6,"status":"active","key":"sk-hidden"}],"total":1}}`))
			} else {
				_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[],"total":0}}`))
			}
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/keys":
			var body struct {
				GroupID int64 `json:"group_id"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			if body.GroupID != 6 {
				http.Error(writer, "group_id must be the numeric stable ID", http.StatusBadRequest)
				return
			}
			created = true
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":17,"key":"sk-once"}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/user/keys":
			t.Error("must not call the nonexistent legacy user key endpoint")
			writer.WriteHeader(http.StatusNotFound)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token := "token"
	record := configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}
	key, err := NewReader(server.Client()).CreateKey(context.Background(), record, "codex-key", "6")
	if err != nil {
		t.Fatal(err)
	}
	if key.KeyID != "17" || key.GroupID != "6" || key.Secret != "sk-once" {
		t.Fatalf("key=%#v", key)
	}
}

func TestCreateSub2APIKeyKeepsCanonicalEndpointError(t *testing.T) {
	legacyCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/keys":
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"code":404,"message":"upstream group no longer exists"}`))
		case "/api/v1/user/keys":
			legacyCalls++
			writer.WriteHeader(http.StatusNotFound)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token := "token"
	_, err := NewReader(server.Client()).CreateKeyWithVerification(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, "codex-key", "6", false)
	if err == nil || !strings.Contains(err.Error(), "/api/v1/keys") || !strings.Contains(err.Error(), "upstream group no longer exists") {
		t.Fatalf("err=%v", err)
	}
	if legacyCalls != 0 {
		t.Fatalf("nonexistent legacy endpoint calls=%d", legacyCalls)
	}
}

func TestCreateKeySkipsInventoryReadbackWhenVerificationDisabled(t *testing.T) {
	var gets, posts int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			gets++
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		posts++
		_, _ = writer.Write([]byte(`{"code":0,"data":{"id":17,"name":"codex-key","group_id":6,"key":"sk-once"}}`))
	}))
	defer server.Close()
	token := "token"
	key, err := NewReader(server.Client()).CreateKeyWithVerification(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, "codex-key", "6", false)
	if err != nil {
		t.Fatal(err)
	}
	if key.KeyID != "17" || key.Secret != "sk-once" || gets != 0 || posts != 1 {
		t.Fatalf("key=%#v gets=%d posts=%d", key, gets, posts)
	}
}

func TestCreateKeyRecoversMissingResponseIDFromInventoryWhenVerificationDisabled(t *testing.T) {
	var gets, posts int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodPost:
			posts++
			_, _ = writer.Write([]byte(`{"code":0,"data":{"key":"sk-once"}}`))
		case http.MethodGet:
			gets++
			_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[{"id":17,"name":"codex-key","group_id":6,"status":"active"}],"total":1}}`))
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	token := "token"
	key, err := NewReader(server.Client()).CreateKeyWithVerification(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, "codex-key", "6", false)
	if err != nil {
		t.Fatal(err)
	}
	if key.KeyID != "17" || key.Name != "codex-key" || key.GroupID != "6" || key.Secret != "sk-once" || gets != 1 || posts != 1 {
		t.Fatalf("key=%#v gets=%d posts=%d", key, gets, posts)
	}
}

func TestCreateKeyRejectsAmbiguousInventoryFallbackWhenResponseIDMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			_, _ = writer.Write([]byte(`{"code":0,"data":{"key":"sk-once"}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"code":0,"data":{"items":[
			{"id":17,"name":"codex-key","group_id":6},
			{"id":18,"name":"codex-key","group_id":6}
		],"total":2}}`))
	}))
	defer server.Close()
	token := "token"
	_, err := NewReader(server.Client()).CreateKeyWithVerification(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, "codex-key", "6", false)
	if err == nil || !strings.Contains(err.Error(), "目录补读无法唯一定位新 Key") {
		t.Fatalf("err=%v", err)
	}
}

func TestDeleteKeyUsesPlatformEndpointAndAuthentication(t *testing.T) {
	var deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authorization") != "Bearer token" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		if request.Method != http.MethodDelete {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		deletedPath = request.URL.Path
		_, _ = writer.Write([]byte(`{"code":0,"data":{"deleted":true}}`))
	}))
	defer server.Close()
	token := "token"
	err := NewReader(server.Client()).DeleteKey(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, "17")
	if err != nil {
		t.Fatal(err)
	}
	if deletedPath != "/api/v1/keys/17" {
		t.Fatalf("deleted path=%q", deletedPath)
	}
}

func TestFallbackCachesVerifiedPathPerHost(t *testing.T) {
	var primary, fallback int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/primary":
			primary++
			writer.WriteHeader(http.StatusNotFound)
		case "/fallback":
			fallback++
			_, _ = writer.Write([]byte(`{"success":true,"data":{}}`))
		}
	}))
	defer server.Close()
	reader := NewReader(server.Client())
	record := configstore.AuthRecord{BaseURL: server.URL, Headers: map[string]string{}, Cookies: map[string]string{}}
	for range 2 {
		if _, err := reader.getFallback(context.Background(), record, []string{"/primary", "/fallback"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if primary != 1 || fallback != 2 {
		t.Fatalf("primary=%d fallback=%d", primary, fallback)
	}
}
