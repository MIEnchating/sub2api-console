package newapimanagement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type keyManagerStub struct {
	created       upstreamsync.CreatedKey
	createErr     error
	createRecord  configstore.AuthRecord
	createName    string
	createGroupID string
	revealRecord  configstore.AuthRecord
	revealKeyID   string
	revealGroupID string
}

func (stub *keyManagerStub) CreateKeyWithVerification(_ context.Context, record configstore.AuthRecord, name, groupID string, _ bool) (upstreamsync.CreatedKey, error) {
	stub.createRecord, stub.createName, stub.createGroupID = record, name, groupID
	return stub.created, stub.createErr
}

func (stub *keyManagerStub) RevealKey(_ context.Context, record configstore.AuthRecord, keyID, groupID string) (upstreamsync.CreatedKey, error) {
	stub.revealRecord, stub.revealKeyID, stub.revealGroupID = record, keyID, groupID
	return stub.created, nil
}

type privateStub struct {
	platform      configstore.NewAPIPlatform
	target        configstore.TargetSettings
	vaultEntries  map[string]configstore.VaultEntry
	storedSecret  *configstore.UpstreamKeySecret
	saveSecretErr error
}

func (stub *privateStub) NewAPIPlatforms(context.Context) ([]configstore.NewAPIPlatformSummary, error) {
	return []configstore.NewAPIPlatformSummary{{ID: stub.platform.ID, Name: stub.platform.Name}}, nil
}

func (stub *privateStub) NewAPIPlatform(_ context.Context, id string) (*configstore.NewAPIPlatform, error) {
	if id != stub.platform.ID {
		return nil, nil
	}
	copy := stub.platform
	return &copy, nil
}

func (stub *privateStub) SaveNewAPIPlatform(_ context.Context, value configstore.NewAPIPlatform) (configstore.NewAPIPlatformSummary, error) {
	stub.platform = value
	return configstore.NewAPIPlatformSummary{ID: value.ID, Name: value.Name, BaseURL: value.BaseURL, UserID: value.UserID, AdminKeyConfigured: true}, nil
}

func (*privateStub) DeleteNewAPIPlatform(context.Context, string) (bool, error) { return true, nil }

func (stub *privateStub) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return stub.target, nil
}

func (stub *privateStub) VaultEntry(_ context.Context, entry string) (*configstore.VaultEntry, error) {
	value, found := stub.vaultEntries[entry]
	if !found {
		return nil, nil
	}
	copy := value
	return &copy, nil
}

func (stub *privateStub) UpstreamKeySecret(_ context.Context, host, keyID, groupID string) (*configstore.UpstreamKeySecret, error) {
	if stub.storedSecret == nil || stub.storedSecret.Host != configstore.CanonicalHost(host) ||
		stub.storedSecret.KeyID != keyID || stub.storedSecret.GroupID != groupID {
		return nil, nil
	}
	copy := *stub.storedSecret
	return &copy, nil
}

func (stub *privateStub) SaveUpstreamKeySecret(_ context.Context, value configstore.UpstreamKeySecret) error {
	if stub.saveSecretErr != nil {
		return stub.saveSecretErr
	}
	value.Host = configstore.CanonicalHost(value.Host)
	stub.storedSecret = &value
	return nil
}

type authenticatorStub struct {
	loginRecord     configstore.AuthRecord
	loginCredential configstore.VaultEntry
	result          configstore.AuthRecord
	err             error
}

func (stub *authenticatorStub) Login(_ context.Context, record configstore.AuthRecord, credential configstore.VaultEntry) (configstore.AuthRecord, error) {
	stub.loginRecord, stub.loginCredential = record, credential
	return stub.result, stub.err
}

type repositoryStub struct {
	groups   []business.NewAPILocalGroup
	bindings []business.NewAPIGroupBinding
}

func (stub *repositoryStub) NewAPILocalGroups(context.Context) ([]business.NewAPILocalGroup, error) {
	return stub.groups, nil
}

func (stub *repositoryStub) NewAPIGroupBindings(context.Context, string) ([]business.NewAPIGroupBinding, error) {
	return stub.bindings, nil
}

func (stub *repositoryStub) ReplaceNewAPIGroupBindings(_ context.Context, _ string, values []business.NewAPIGroupBinding) error {
	stub.bindings = values
	return nil
}

func (stub *repositoryStub) DeleteNewAPIGroupBindings(context.Context, string) error { return nil }

func TestRefreshReadsAuthenticatedOptionsAndComparesPublicPricing(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer admin-secret" || request.Header.Get("New-Api-User") != "7" {
			t.Errorf("authentication headers=%v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/option/":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []map[string]string{
				{"key": "GroupRatio", "value": `{"default":1,"vip":2}`},
				{"key": "ModelRatio", "value": `{"gpt-5":0.5,"gpt-5-mini":0.1}`},
				{"key": "CompletionRatio", "value": `{"gpt-5":8,"gpt-5-mini":4}`},
			}})
		case "/api/pricing":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []map[string]any{
				{"model_name": "gpt-5", "model_ratio": 0.625, "completion_ratio": 8},
				{"model_name": "claude-sonnet-4", "model_ratio": 1.5, "completion_ratio": 5},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	private := &privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", Name: "Production", BaseURL: server.URL, AdminKey: "admin-secret", UserID: "7",
	}}
	service := New(private, &repositoryStub{}, server.Client(), nil, nil)
	snapshot, err := service.Refresh(context.Background(), "platform-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Groups) != 2 || len(snapshot.Models) != 2 || len(snapshot.References) != 2 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	kinds := map[string]string{}
	for _, difference := range snapshot.Differences {
		kinds[difference.Model] = difference.Kind
	}
	if kinds["gpt-5"] != "ratio_mismatch" || kinds["gpt-5-mini"] != "only_in_newapi" || kinds["claude-sonnet-4"] != "missing_in_newapi" {
		t.Fatalf("differences=%#v", snapshot.Differences)
	}
}

func TestSaveBindingsSynchronizesOnlyEnabledGroupRatios(t *testing.T) {
	var written map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]string{"GroupRatio": `{"vip":1}`}})
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&written); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": true})
	}))
	defer server.Close()

	ratio := "0.35"
	repository := &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "低价", Ratio: &ratio}}}
	private := &privateStub{platform: configstore.NewAPIPlatform{ID: "platform-1", BaseURL: server.URL, AdminKey: "key", UserID: "1"}}
	service := New(private, repository, server.Client(), nil, nil)
	bindings, err := service.SaveBindings(context.Background(), "platform-1", []GroupBindingInput{{
		NewAPIGroupID: "vip", NewAPIGroupName: "VIP", Sub2APIGroupID: "6", SyncRatio: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || written["key"] != "GroupRatio" || written["value"] != `{"vip":0.35}` {
		t.Fatalf("bindings=%#v written=%#v", bindings, written)
	}
}

func TestSavePlatformRejectsASecondMainPlatform(t *testing.T) {
	private := &privateStub{platform: configstore.NewAPIPlatform{ID: "primary", Name: "主平台"}}
	service := New(private, &repositoryStub{}, nil, nil, nil)

	_, err := service.SavePlatform(context.Background(), PlatformInput{
		ID: "second", Name: "第二平台", BaseURL: "https://second.example", AdminKey: "key", UserID: "2",
	})
	if err == nil || err.Error() != "New API 只允许配置一个主平台" {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchChannelModelsUsesConfiguredSub2APIAddress(t *testing.T) {
	var requestedPath string
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedPath = request.URL.Path
		if request.Header.Get("Authorization") != "Bearer sub2api-user-key" {
			t.Errorf("authorization=%q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]string{
			{"id": "gpt-5.2"}, {"id": "claude-sonnet-4"}, {"id": "gpt-5.2"},
		}})
	}))
	defer target.Close()

	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1"},
		target:   configstore.TargetSettings{BaseURL: target.URL, AdminKey: "management-secret"},
		storedSecret: &configstore.UpstreamKeySecret{
			Host: configstore.CanonicalHost(target.URL), KeyID: "key-7", GroupID: "6", Secret: "sub2api-user-key",
		},
	}
	keys := &keyManagerStub{created: upstreamsync.CreatedKey{KeyID: "key-7", GroupID: "6", Secret: "sub2api-user-key"}}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, target.Client(), keys, nil)
	models, err := service.FetchChannelModels(context.Background(), "platform-1", ChannelModelsInput{
		Sub2APIGroupID: "6", KeyID: "key-7", BaseURL: target.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if keys.revealKeyID != "" || requestedPath != "/v1/models" || !reflect.DeepEqual(models, []string{"claude-sonnet-4", "gpt-5.2"}) {
		t.Fatalf("path=%q models=%v", requestedPath, models)
	}
}

func TestCreateChannelKeyLogsInWithSelectedVaultEntryAndStoresSecret(t *testing.T) {
	username, password, token := "operator@example.test", "password", "user-jwt"
	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1"},
		target:   configstore.TargetSettings{BaseURL: "https://sub2api.example", AdminKey: "must-not-be-used"},
		vaultEntries: map[string]configstore.VaultEntry{
			"运营账号": {Entry: "运营账号", Username: &username, Password: &password},
		},
	}
	authenticator := &authenticatorStub{result: configstore.AuthRecord{
		Host: "sub2api.example", BaseURL: "https://sub2api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_login", AccessToken: &token,
	}}
	keys := &keyManagerStub{created: upstreamsync.CreatedKey{
		KeyID: "17", Name: "NewAPI-标准-marker", GroupID: "6", Secret: "service-secret",
	}}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, nil, keys, authenticator)

	created, err := service.CreateChannelKey(context.Background(), "platform-1", ChannelKeyInput{
		Sub2APIGroupID: "6", CredentialSource: "vault", VaultEntry: "运营账号",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.KeyID != "17" || created.GroupID != "6" || !strings.Contains(created.Name, "标准") {
		t.Fatalf("created=%#v", created)
	}
	if len(created.Endpoints) != 1 || created.Endpoints[0].BaseURL != "https://sub2api.example" || created.Endpoints[0].Name != "管理平台地址" {
		t.Fatalf("fallback endpoints=%#v", created.Endpoints)
	}
	if authenticator.loginRecord.AuthMode != "sub2api_user_login" || authenticator.loginRecord.AdminKey != nil ||
		authenticator.loginCredential.Entry != "运营账号" || keys.createRecord.AccessToken == nil || *keys.createRecord.AccessToken != token {
		t.Fatalf("login=%#v credential=%#v create=%#v", authenticator.loginRecord, authenticator.loginCredential, keys.createRecord)
	}
	if private.storedSecret == nil || private.storedSecret.Secret != "service-secret" || private.storedSecret.KeyID != "17" || private.storedSecret.GroupID != "6" {
		t.Fatalf("stored secret=%#v", private.storedSecret)
	}
	encoded, marshalErr := json.Marshal(created)
	if marshalErr != nil || strings.Contains(string(encoded), "service-secret") {
		t.Fatalf("channel key response leaked secret: %s", encoded)
	}
}

func TestCreateChannelKeyUsesOfficialSub2APIUserEndpoints(t *testing.T) {
	paths := []string{}
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["email"] != "operator@example.test" || body["password"] != "password" || request.Header.Get("Authorization") != "" {
				t.Fatalf("login body=%#v headers=%#v", body, request.Header)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]string{"access_token": "user-jwt"}})
		case "/api/v1/user/profile":
			if request.Header.Get("Authorization") != "Bearer user-jwt" {
				t.Fatalf("profile authorization=%q", request.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{"id": 9}})
		case "/api/v1/keys":
			if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer user-jwt" || request.Header.Get("X-API-Key") != "" {
				t.Fatalf("key request headers=%#v", request.Header)
			}
			var body struct {
				Name    string `json:"name"`
				GroupID int64  `json:"group_id"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"id": 17, "name": body.Name, "group_id": body.GroupID, "key": "service-secret",
			}})
		case "/api/v1/settings/public":
			if request.Header.Get("Authorization") != "" || request.Header.Get("X-API-Key") != "" {
				t.Fatalf("public settings reused authentication: %#v", request.Header)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"api_base_url": "https://api.example.test/",
				"custom_endpoints": []map[string]string{
					{"name": "Docker 内网", "endpoint": "http://sub2api:8080"},
					{"name": "重复", "endpoint": "https://api.example.test"},
					{"name": "无效", "endpoint": "javascript:alert(1)"},
				},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer target.Close()

	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1"},
		target:   configstore.TargetSettings{BaseURL: target.URL, AdminKey: "must-not-be-used"},
	}
	service := New(
		private,
		&repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}},
		target.Client(),
		upstreamsync.NewReader(target.Client()),
		upstreamauth.New(target.Client()),
	)
	created, err := service.CreateChannelKey(context.Background(), "platform-1", ChannelKeyInput{
		Sub2APIGroupID: "6", CredentialSource: "custom", Username: "operator@example.test", Password: "password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.KeyID != "17" || private.storedSecret == nil || private.storedSecret.Secret != "service-secret" {
		t.Fatalf("created=%#v stored=%#v", created, private.storedSecret)
	}
	wantEndpoints := []ChannelEndpoint{
		{Name: "API 端点", BaseURL: "https://api.example.test", Default: true},
		{Name: "Docker 内网", BaseURL: "http://sub2api:8080"},
	}
	if !reflect.DeepEqual(created.Endpoints, wantEndpoints) {
		t.Fatalf("endpoints=%#v want=%#v", created.Endpoints, wantEndpoints)
	}
	wantPaths := []string{"POST /api/v1/auth/login", "GET /api/v1/user/profile", "POST /api/v1/keys", "GET /api/v1/settings/public"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("paths=%#v want=%#v", paths, wantPaths)
	}
}

func TestCreateChannelKeyUsesRequestScopedCustomCredentials(t *testing.T) {
	token := "user-jwt"
	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1"},
		target:   configstore.TargetSettings{BaseURL: "https://sub2api.example"},
	}
	authenticator := &authenticatorStub{result: configstore.AuthRecord{AccessToken: &token, AuthMode: "sub2api_user_login"}}
	keys := &keyManagerStub{created: upstreamsync.CreatedKey{KeyID: "17", Name: "marker", GroupID: "6", Secret: "secret"}}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, nil, keys, authenticator)

	_, err := service.CreateChannelKey(context.Background(), "platform-1", ChannelKeyInput{
		Sub2APIGroupID: "6", CredentialSource: "custom", Username: "user@example.test", Password: "pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	if authenticator.loginCredential.Entry != "" || authenticator.loginCredential.Username == nil ||
		*authenticator.loginCredential.Username != "user@example.test" || authenticator.loginCredential.Password == nil ||
		*authenticator.loginCredential.Password != "pass" || len(private.vaultEntries) != 0 {
		t.Fatalf("custom credential=%#v vault=%#v", authenticator.loginCredential, private.vaultEntries)
	}
}

func TestCreateChannelKeyRejectsMissingCredentialAndLoginFailure(t *testing.T) {
	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1"},
		target:   configstore.TargetSettings{BaseURL: "https://sub2api.example"},
		vaultEntries: map[string]configstore.VaultEntry{
			"incomplete": {Entry: "incomplete"},
		},
	}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, nil, &keyManagerStub{}, &authenticatorStub{})
	for name, input := range map[string]ChannelKeyInput{
		"missing vault":    {Sub2APIGroupID: "6", CredentialSource: "vault", VaultEntry: "missing"},
		"incomplete vault": {Sub2APIGroupID: "6", CredentialSource: "vault", VaultEntry: "incomplete"},
		"empty custom":     {Sub2APIGroupID: "6", CredentialSource: "custom"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.CreateChannelKey(context.Background(), "platform-1", input); err == nil {
				t.Fatal("expected credential validation error")
			}
		})
	}

	authenticator := &authenticatorStub{err: errors.New("invalid credentials")}
	service = New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, nil, &keyManagerStub{}, authenticator)
	_, err := service.CreateChannelKey(context.Background(), "platform-1", ChannelKeyInput{
		Sub2APIGroupID: "6", CredentialSource: "custom", Username: "user@example.test", Password: "wrong",
	})
	if err == nil || !strings.Contains(err.Error(), "登录失败") {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateChannelUsesSub2APITypeAndNewAPIGroups(t *testing.T) {
	var created map[string]any
	newAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/option/":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]string{
				"GroupRatio": `{"default":1,"vip":2}`,
			}})
		case request.Method == http.MethodPost && request.URL.Path == "/api/channel/":
			if err := json.NewDecoder(request.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]any{"id": 12}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer newAPI.Close()

	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1", BaseURL: newAPI.URL, AdminKey: "admin", UserID: "1"},
		target:   configstore.TargetSettings{BaseURL: "https://sub2api.example", AdminKey: "management-secret"},
		storedSecret: &configstore.UpstreamKeySecret{
			Host: "sub2api.example", KeyID: "key-7", GroupID: "6", Secret: "sub2api-user-key",
		},
	}
	keys := &keyManagerStub{created: upstreamsync.CreatedKey{KeyID: "key-7", GroupID: "6", Secret: "sub2api-user-key"}}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, newAPI.Client(), keys, nil)
	_, err := service.CreateChannel(context.Background(), "platform-1", ChannelInput{
		Sub2APIGroupID: "6", KeyID: "key-7", BaseURL: "https://edge.example/v1", Models: []string{"gpt-5.2"},
		NewAPIGroups: []string{"vip", "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	channel, ok := created["channel"].(map[string]any)
	if !ok {
		t.Fatalf("request missing channel wrapper: %#v", created)
	}
	if created["mode"] != "single" || channel["type"] != float64(59) || channel["name"] != "标准" ||
		channel["base_url"] != "https://edge.example/v1" || channel["key"] != "sub2api-user-key" ||
		channel["models"] != "gpt-5.2" || channel["group"] != "default,vip" {
		t.Fatalf("unexpected create request: %#v", created)
	}
}
