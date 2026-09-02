package newapimanagement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type keyManagerStub struct {
	created       upstreamsync.CreatedKey
	createRecord  configstore.AuthRecord
	createName    string
	createGroupID string
	revealRecord  configstore.AuthRecord
	revealKeyID   string
	revealGroupID string
}

func (stub *keyManagerStub) CreateKeyWithVerification(_ context.Context, record configstore.AuthRecord, name, groupID string, _ bool) (upstreamsync.CreatedKey, error) {
	stub.createRecord, stub.createName, stub.createGroupID = record, name, groupID
	return stub.created, nil
}

func (stub *keyManagerStub) RevealKey(_ context.Context, record configstore.AuthRecord, keyID, groupID string) (upstreamsync.CreatedKey, error) {
	stub.revealRecord, stub.revealKeyID, stub.revealGroupID = record, keyID, groupID
	return stub.created, nil
}

type privateStub struct {
	platform configstore.NewAPIPlatform
	target   configstore.TargetSettings
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
	service := New(private, &repositoryStub{}, server.Client(), nil)
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
	service := New(private, repository, server.Client(), nil)
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
	service := New(private, &repositoryStub{}, nil, nil)

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
	}
	keys := &keyManagerStub{created: upstreamsync.CreatedKey{KeyID: "key-7", GroupID: "6", Secret: "sub2api-user-key"}}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, target.Client(), keys)
	models, err := service.FetchChannelModels(context.Background(), "platform-1", ChannelModelsInput{
		Sub2APIGroupID: "6", KeyID: "key-7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if keys.revealKeyID != "key-7" || keys.revealGroupID != "6" || requestedPath != "/v1/models" || !reflect.DeepEqual(models, []string{"claude-sonnet-4", "gpt-5.2"}) {
		t.Fatalf("path=%q models=%v", requestedPath, models)
	}
}

func TestCreateChannelKeyUsesManagementCredentialsWithoutReturningSecret(t *testing.T) {
	var receivedName string
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/api/v1/keys" || request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("X-API-Key") != "management-secret" || request.Header.Get("Authorization") != "" {
			writer.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(writer).Encode(map[string]any{"message": "Invalid token"})
			return
		}
		var body struct {
			Name    string `json:"name"`
			GroupID int64  `json:"group_id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedName = body.Name
		_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
			"id": 17, "name": body.Name, "group_id": body.GroupID, "key": "service-secret",
		}})
	}))
	defer target.Close()

	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1"},
		target:   configstore.TargetSettings{BaseURL: target.URL, AdminKey: "management-secret"},
	}
	service := New(
		private,
		&repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}},
		target.Client(),
		upstreamsync.NewReader(target.Client()),
	)

	created, err := service.CreateChannelKey(context.Background(), "platform-1", ChannelKeyInput{Sub2APIGroupID: "6"})
	if err != nil {
		t.Fatal(err)
	}
	if created.KeyID != "17" || created.GroupID != "6" || created.Name != receivedName || !strings.Contains(created.Name, "标准") {
		t.Fatalf("created=%#v", created)
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
	}
	keys := &keyManagerStub{created: upstreamsync.CreatedKey{KeyID: "key-7", GroupID: "6", Secret: "sub2api-user-key"}}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, newAPI.Client(), keys)
	_, err := service.CreateChannel(context.Background(), "platform-1", ChannelInput{
		Sub2APIGroupID: "6", KeyID: "key-7", Models: []string{"gpt-5.2"},
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
		channel["base_url"] != "https://sub2api.example" || channel["key"] != "sub2api-user-key" ||
		channel["models"] != "gpt-5.2" || channel["group"] != "default,vip" {
		t.Fatalf("unexpected create request: %#v", created)
	}
}
