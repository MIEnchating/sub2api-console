package newapimanagement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type privateStub struct {
	platform configstore.NewAPIPlatform
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
	service := New(private, &repositoryStub{}, server.Client())
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
	service := New(private, repository, server.Client())
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
