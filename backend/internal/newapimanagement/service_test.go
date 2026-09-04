package newapimanagement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type keyManagerStub struct {
	created       upstreamsync.CreatedKey
	createErr     error
	createCalls   int
	createRecord  configstore.AuthRecord
	createName    string
	createGroupID string
	revealRecord  configstore.AuthRecord
	revealKeyID   string
	revealGroupID string
	reconcileErr  error
}

func (stub *keyManagerStub) CreateKeyWithVerification(_ context.Context, record configstore.AuthRecord, name, groupID string, _ bool) (upstreamsync.CreatedKey, error) {
	stub.createCalls++
	stub.createRecord, stub.createName, stub.createGroupID = record, name, groupID
	return stub.created, stub.createErr
}

func (stub *keyManagerStub) ReconcileCreatedKey(_ context.Context, _ configstore.AuthRecord, name, groupID string) (upstreamsync.CreatedKey, bool, error) {
	if stub.reconcileErr != nil {
		return upstreamsync.CreatedKey{}, false, stub.reconcileErr
	}
	if stub.createCalls == 0 {
		return upstreamsync.CreatedKey{}, false, nil
	}
	result := stub.created
	result.Name = name
	result.GroupID = groupID
	return result, true, nil
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

type upstreamPricingPrivateStub struct {
	*privateStub
	records    map[string]configstore.AuthRecord
	indexCalls int
}

func (stub *upstreamPricingPrivateStub) AuthRecordIndex(context.Context) ([]configstore.AuthRecordSummary, error) {
	stub.indexCalls++
	result := make([]configstore.AuthRecordSummary, 0, len(stub.records))
	for host, record := range stub.records {
		result = append(result, configstore.AuthRecordSummary{Host: host, BaseURL: record.BaseURL, UpstreamType: record.UpstreamType})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Host < result[right].Host })
	return result, nil
}

func (stub *upstreamPricingPrivateStub) AuthRecord(_ context.Context, host string) (*configstore.AuthRecord, error) {
	record, found := stub.records[host]
	if !found {
		return nil, nil
	}
	copy := record
	return &copy, nil
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

func (stub *repositoryStub) DeleteNewAPIGroupBindings(context.Context, string) error {
	stub.bindings = nil
	return nil
}

func TestRefreshReadsAuthenticatedOptionsAsPriceSource(t *testing.T) {
	t.Helper()
	pricingRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/model-plaza" && (request.Header.Get("Authorization") != "Bearer admin-secret" || request.Header.Get("New-Api-User") != "7") {
			t.Errorf("authentication headers=%v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/option/":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []map[string]string{
				{"key": "GroupRatio", "value": `{"default":1,"vip":2}`},
				{"key": "ModelRatio", "value": `{"gpt-5":0.5,"gpt-5-mini":0.1}`},
				{"key": "CompletionRatio", "value": `{"gpt-5":8,"gpt-5-mini":4}`},
				{"key": "ModelPrice", "value": `{"dall-e-3":0.04}`},
				{"key": "CacheRatio", "value": `{"gpt-5":0.1,"cache-only":0.2}`},
				{"key": "ImageRatio", "value": `{"dall-e-3":1.2}`},
				{"key": "billing_setting.billing_mode", "value": `{"expr-only":"tiered_expr"}`},
				{"key": "billing_setting.billing_expr", "value": `{"expr-only":"tier(0, 1)"}`},
			}})
		case "/api/channel/models_enabled":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []string{
				"gpt-5", "gpt-5-mini", "dall-e-3", "cache-only", "expr-only", "unset-a", "unset-b",
			}})
		case "/api/pricing":
			pricingRequests++
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []map[string]any{
				{"model_name": "gpt-5", "model_ratio": 0.625, "completion_ratio": 8},
				{"model_name": "claude-sonnet-4", "model_ratio": 1.5, "completion_ratio": 5},
				{"model_name": "MiniMax-H3-768p", "model_ratio": 1, "completion_ratio": 1},
				{"model_name": "dall-e-3", "model_ratio": 1, "completion_ratio": 1},
			}})
		case "/api/v1/model-plaza":
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"groups": []map[string]any{
				{"models": []map[string]any{
					{"name": "gpt-5", "official_pricing": map[string]any{"input_price": 0.000001, "output_price": 0.000008}},
					{"name": "sub2api-only", "official_pricing": map[string]any{"input_price": 0.000002, "output_price": 0.000004}},
				}},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	private := &privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", Name: "Production", BaseURL: server.URL, AdminKey: "admin-secret", UserID: "7",
	}, target: configstore.TargetSettings{BaseURL: server.URL}}
	service := New(private, &repositoryStub{}, server.Client(), nil, nil)
	snapshot, err := service.Refresh(context.Background(), "platform-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Groups) != 2 || len(snapshot.Models) != 5 || len(snapshot.References) != 7 || len(snapshot.UnsetModels) != 3 || len(snapshot.ToolPrices) != 9 || len(snapshot.Sub2APIModels) != 0 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if pricingRequests != 1 {
		t.Fatalf("site pricing endpoint was requested %d times", pricingRequests)
	}
	if len(snapshot.Differences) != 4 {
		t.Fatalf("differences=%#v", snapshot.Differences)
	}
	byModel := make(map[string]ModelPrice, len(snapshot.Models))
	for _, model := range snapshot.Models {
		byModel[model.Model] = model
	}
	if byModel["dall-e-3"].ModelPrice != "0.04" || byModel["dall-e-3"].ImageRatio != "1.2" || byModel["gpt-5"].CacheRatio != "0.1" || byModel["gpt-5"].InputPrice != "1" || byModel["gpt-5"].CompletionPrice != "8" {
		t.Fatalf("option price dimensions were not decoded: %#v", byModel)
	}
	unsetNames := make([]string, 0, len(snapshot.UnsetModels))
	for _, model := range snapshot.UnsetModels {
		unsetNames = append(unsetNames, model.Model)
	}
	if !reflect.DeepEqual(unsetNames, []string{"cache-only", "unset-a", "unset-b"}) {
		t.Fatalf("unset models=%#v", snapshot.UnsetModels)
	}
	if len(snapshot.References) != 7 {
		t.Fatalf("site model catalog was not included: %#v", snapshot.References)
	}
}

func TestRefreshReadsSub2APIManagementPlaza(t *testing.T) {
	newAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/option/":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]string{
				"GroupRatio": `{"default":1}`, "ModelRatio": `{"shared":1}`, "CompletionRatio": `{"shared":2}`,
			}})
		case "/api/channel/models_enabled":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []string{"shared"}})
		case "/api/pricing":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []map[string]any{
				{"model_name": "shared", "model_ratio": 0.8, "completion_ratio": 2},
				{"model_name": "newapi-only", "model_ratio": 1.2, "completion_ratio": 3},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer newAPI.Close()

	newAPIUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/pricing" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []map[string]any{
			{"model_name": "shared", "model_ratio": 0.9, "completion_ratio": 2},
		}})
	}))
	defer newAPIUpstream.Close()

	sub2APIUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/model-plaza" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"groups": []map[string]any{{
			"models": []map[string]any{{"name": "sub2api-only", "official_pricing": map[string]any{"input_price": 0.000002, "output_price": 0.000004}}},
		}}}})
	}))
	defer sub2APIUpstream.Close()

	private := &upstreamPricingPrivateStub{
		privateStub: &privateStub{platform: configstore.NewAPIPlatform{ID: "platform-1", BaseURL: newAPI.URL, AdminKey: "admin", UserID: "1"}, target: configstore.TargetSettings{BaseURL: sub2APIUpstream.URL}},
		records: map[string]configstore.AuthRecord{
			"a-newapi.example":  {Host: "a-newapi.example", BaseURL: newAPIUpstream.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token"},
			"b-sub2api.example": {Host: "b-sub2api.example", BaseURL: sub2APIUpstream.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token"},
		},
	}
	service := New(private, &repositoryStub{}, newAPI.Client(), nil, nil)
	snapshot, err := service.Refresh(context.Background(), "platform-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.NewAPIModels) != 2 || len(snapshot.UnsetModels) != 0 || len(snapshot.Sub2APIModels) != 0 || len(snapshot.UpstreamPrices) != 2 {
		t.Fatalf("snapshot catalogs=%#v", snapshot)
	}
	if len(snapshot.Differences) != 2 {
		t.Fatalf("differences=%#v", snapshot.Differences)
	}
}

func TestDecodeSub2APIPricingJSON(t *testing.T) {
	prices, err := decodeSub2APIPricingJSON([]byte(`{
		"gpt-image-1": {"input_cost_per_token": 0.000005, "input_cost_per_image_token": 0.00001, "output_cost_per_image_token": 0.00004},
		"gpt-5.6-sol": {
			"input_cost_per_token": 0.000005,
			"output_cost_per_token": 0.00003,
			"cache_creation_input_token_cost": 0.00000625,
			"cache_read_input_token_cost": 0.0000005,
			"input_cost_per_token_above_272k_tokens": 0.00001,
			"output_cost_per_token_above_272k_tokens": 0.000045,
			"cache_creation_input_token_cost_above_272k_tokens": 0.00002,
			"cache_read_input_token_cost_above_272k_tokens": 0.000001
		},
		"grok-tier": {
			"input_cost_per_token": 0.000002,
			"output_cost_per_token": 0.000006,
			"input_cost_per_token_above_200k_tokens": 0.000004,
			"output_cost_per_token_above_200k_tokens": 0.000012,
			"litellm_provider": "xai"
		},
		"z-model": {"input_cost_per_token": 0.000003, "output_cost_per_token": 0.000012},
		"image-only": {"output_cost_per_image": 0.04},
		"sample_spec": {"input_cost_per_token": 1}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 5 || prices[0].Model != "gpt-5.6-sol" || prices[1].Model != "gpt-image-1" || prices[2].Model != "grok-tier" || prices[3].Model != "image-only" || prices[4].Model != "z-model" {
		t.Fatalf("prices=%#v", prices)
	}
	if prices[0].LongContextThreshold != 272000 || prices[0].LongContextInputPrice != "0.00001" || prices[0].LongContextOutputPrice != "0.000045" || prices[0].LongContextCacheWritePrice != "0.0000125" || prices[0].LongContextCacheReadPrice != "0.000001" {
		t.Fatalf("long-context prices were not preserved: %#v", prices[0])
	}
	if prices[1].ModelRatio != "2.5" || prices[1].CompletionRatio != "8" || prices[1].ImageRatio != "2" {
		t.Fatalf("image ratios were not derived: %#v", prices[1])
	}
	if !prices[2].LongContextThresholdInclusive || prices[2].LongContextThreshold != 200000 {
		t.Fatalf("xAI threshold semantics were not preserved: %#v", prices[2])
	}
	if prices[4].ModelRatio == "" || prices[4].CompletionRatio == "" {
		t.Fatalf("ratios were not derived: %#v", prices[4])
	}
}

func TestRemotePricingSourcePreservesExactDownloadedBytes(t *testing.T) {
	raw := []byte("{\n  \"gpt-test\": {\"input_cost_per_token\": 1e-6}\n}\n")
	source := buildRemotePricingSource(raw, time.Date(2026, time.September, 4, 12, 30, 0, 0, time.UTC))
	wantHash := sha256.Sum256(raw)
	if source.Content != string(raw) || source.SizeBytes != len(raw) || source.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("source=%#v", source)
	}
	if source.SourceURL != defaultSub2APIPricingURL || source.FetchedAt != "2026-09-04T12:30:00Z" {
		t.Fatalf("source metadata=%#v", source)
	}
}

func TestManagementRequestRejectsHTTP200BusinessFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":17,"error":"invalid admin key"}`))
	}))
	defer server.Close()
	service := New(nil, nil, server.Client(), nil, nil)
	_, err := service.request(context.Background(), configstore.NewAPIPlatform{
		BaseURL: server.URL, AdminKey: "secret", UserID: "1",
	}, http.MethodGet, "/api/option/", nil)
	if err == nil || !strings.Contains(err.Error(), "code=17") {
		t.Fatalf("business failure was accepted: %v", err)
	}
}

func TestManagementRequestDoesNotFollowCredentialedRedirect(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		redirected = true
		if request.Header.Get("Authorization") != "" {
			t.Error("redirect target received management authorization")
		}
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	service := New(nil, nil, source.Client(), nil, nil)
	_, err := service.request(context.Background(), configstore.NewAPIPlatform{
		BaseURL: source.URL, AdminKey: "secret", UserID: "1",
	}, http.MethodGet, "/api/option/", nil)
	if err == nil || redirected {
		t.Fatalf("credentialed redirect was followed: redirected=%v err=%v", redirected, err)
	}
}

func TestSavePlatformRequiresNewCredentialWhenOriginChanges(t *testing.T) {
	requests := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer target.Close()
	private := &privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", BaseURL: "https://old.example", AdminKey: "old-secret", UserID: "1",
	}}
	service := New(private, &repositoryStub{}, target.Client(), nil, nil)
	_, err := service.SavePlatform(context.Background(), PlatformInput{
		ID: "platform-1", Name: "new", BaseURL: target.URL, UserID: "1",
	})
	if err == nil || requests != 0 {
		t.Fatalf("old-origin credential was reused: requests=%d err=%v", requests, err)
	}
}

func TestDecodeSub2APIPricingJSONRejectsTrailingDocument(t *testing.T) {
	_, err := decodeSub2APIPricingJSON([]byte(`{"model":{"input_cost_per_token":0.1}} {}`))
	if err == nil {
		t.Fatal("trailing JSON document was accepted")
	}
}

func TestDecodeSub2APIPricingJSONKeepsInputOnlyTokenModelsWritable(t *testing.T) {
	prices, err := decodeSub2APIPricingJSON([]byte(`{
		"embedding-model": {"input_cost_per_token": 0.0000001, "mode": "embedding"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 1 || prices[0].ModelRatio != "0.05" || prices[0].CompletionRatio != "0" {
		t.Fatalf("prices=%#v", prices)
	}
}

func TestDecodeSub2APIPricingJSONAppliesInputLadderToAllCacheClasses(t *testing.T) {
	prices, err := decodeSub2APIPricingJSON([]byte(`{
		"claude-tier": {
			"input_cost_per_token": 0.000003,
			"output_cost_per_token": 0.000015,
			"cache_creation_input_token_cost": 0.00000375,
			"cache_creation_input_token_cost_above_1hr": 0.000006,
			"cache_read_input_token_cost": 0.0000003,
			"input_cost_per_token_above_200k_tokens": 0.000006,
			"output_cost_per_token_above_200k_tokens": 0.0000225,
			"cache_creation_input_token_cost_above_200k_tokens": 0.000009,
			"cache_creation_input_token_cost_above_1hr_above_200k_tokens": 0.000015,
			"cache_read_input_token_cost_above_200k_tokens": 0.0000009,
			"litellm_provider": "anthropic"
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 1 {
		t.Fatalf("prices=%#v", prices)
	}
	price := prices[0]
	if price.LongContextThreshold != 200000 || price.LongContextThresholdInclusive {
		t.Fatalf("threshold=%#v", price)
	}
	// Sub2API intentionally applies the 2x input ladder to every cache class;
	// conflicting raw cache-above values are audit metadata, not billing inputs.
	if price.LongContextCacheWritePrice != "0.0000075" || price.LongContextCacheWrite1hPrice != "0.000012" || price.LongContextCacheReadPrice != "0.0000006" {
		t.Fatalf("cache ladder=%#v", price)
	}
}

func TestSaveModelPricesWritesSupportedFieldsAndClearsConflictingConfiguration(t *testing.T) {
	options := map[string]string{
		"ModelPrice":                   `{"gpt-5":9,"fixed-only":1}`,
		"ModelRatio":                   `{"gpt-5":99,"other":2}`,
		"CompletionRatio":              `{"gpt-5":99,"other":3}`,
		"CacheRatio":                   `{"gpt-5":9}`,
		"CreateCacheRatio":             `{"gpt-5":9}`,
		"ImageRatio":                   `{"gpt-5":9}`,
		"AudioRatio":                   `{"gpt-5":9}`,
		"AudioCompletionRatio":         `{"gpt-5":9}`,
		"billing_setting.billing_mode": `{"gpt-5":"tiered_expr","other":"per_second"}`,
		"billing_setting.billing_expr": `{"gpt-5":"tier(0, p * 1)","other":"tier(0, p * 2)"}`,
		"tool_price_setting.prices":    `{}`,
	}
	written := map[string]string{}
	newAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/option/":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": options})
		case request.Method == http.MethodPut && request.URL.Path == "/api/option/":
			var input struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			options[input.Key] = input.Value
			written[input.Key] = input.Value
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true})
		case request.Method == http.MethodGet && request.URL.Path == "/api/channel/models_enabled":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []string{"gpt-5", "other"}})
		case request.Method == http.MethodGet && request.URL.Path == "/api/pricing":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []any{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer newAPI.Close()

	service := New(&privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", BaseURL: newAPI.URL, AdminKey: "admin", UserID: "1",
	}}, &repositoryStub{}, newAPI.Client(), nil, nil)
	snapshot, err := service.SaveModelPrices(context.Background(), "platform-1", []ModelPriceInput{{
		Model: "gpt-5", InputRatio: "0.5", CompletionRatio: "8", CacheRatio: "0.1",
		CreateCacheRatio: "1.25", ImageRatio: "2",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Models) != 3 {
		t.Fatalf("models=%#v", snapshot.Models)
	}
	assertOptionValue := func(key, model, want string, present bool) {
		t.Helper()
		values, decodeErr := decodeDecimalMap(written[key])
		if decodeErr != nil {
			t.Fatalf("decode %s: %v", key, decodeErr)
		}
		got, found := values[model]
		if found != present || got != want {
			t.Fatalf("%s[%s]=%q, present=%t; want %q, present=%t", key, model, got, found, want, present)
		}
	}
	assertOptionValue("ModelPrice", "gpt-5", "", false)
	assertOptionValue("ModelRatio", "gpt-5", "0.5", true)
	assertOptionValue("CompletionRatio", "gpt-5", "8", true)
	assertOptionValue("CacheRatio", "gpt-5", "0.1", true)
	assertOptionValue("CreateCacheRatio", "gpt-5", "1.25", true)
	assertOptionValue("ImageRatio", "gpt-5", "2", true)
	assertOptionValue("AudioRatio", "gpt-5", "", false)
	for _, key := range []string{"billing_setting.billing_mode", "billing_setting.billing_expr"} {
		values, decodeErr := decodeStringMap(written[key])
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if _, found := values["gpt-5"]; found || values["other"] == "" {
			t.Fatalf("%s=%#v", key, values)
		}
	}
}

func TestValidDecimalRejectsNonFiniteAndNonDecimalValues(t *testing.T) {
	for _, value := range []string{"0", "0.125", "1e-6", "1E+6"} {
		if !validDecimal(value) {
			t.Fatalf("valid decimal %q was rejected", value)
		}
	}
	for _, value := range []string{"+Inf", "Inf", "NaN", "1/2", "-0.1", "1e1000000", "1e-1000000", "", strings.Repeat("9", 129)} {
		if validDecimal(value) {
			t.Fatalf("invalid decimal %q was accepted", value)
		}
	}
}

func TestSaveModelPricesRejectsDuplicateModelsBeforeRemoteWrite(t *testing.T) {
	putCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && request.URL.Path == "/api/option/" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]string{}})
			return
		}
		putCalls++
		writer.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": false})
	}))
	defer server.Close()
	service := New(&privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", BaseURL: server.URL, AdminKey: "admin", UserID: "1",
	}}, &repositoryStub{}, server.Client(), nil, nil)

	_, err := service.SaveModelPrices(context.Background(), "platform-1", []ModelPriceInput{
		{Model: "gpt-5", InputRatio: "1", CompletionRatio: "2"},
		{Model: " gpt-5 ", InputRatio: "3", CompletionRatio: "4"},
	})

	if err == nil || KindOf(err) != ErrorValidation {
		t.Fatalf("error=%v kind=%q", err, KindOf(err))
	}
	if putCalls != 0 {
		t.Fatalf("duplicate model triggered %d remote writes", putCalls)
	}
}

func TestSaveModelPricesWritesTieredExpressionAndReturnsReadback(t *testing.T) {
	options := map[string]string{
		"ModelPrice":                   `{}`,
		"ModelRatio":                   `{"other":2}`,
		"CompletionRatio":              `{"other":3}`,
		"CacheRatio":                   `{}`,
		"CreateCacheRatio":             `{}`,
		"ImageRatio":                   `{}`,
		"AudioRatio":                   `{}`,
		"AudioCompletionRatio":         `{}`,
		"billing_setting.billing_mode": `{"other":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"other":"tier(\"base\", p * 2 + c * 6)"}`,
		"GroupRatio":                   `{}`,
		"tool_price_setting.prices":    `{}`,
	}
	newAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/option/":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": options})
		case request.Method == http.MethodPut && request.URL.Path == "/api/option/":
			var input struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			options[input.Key] = input.Value
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true})
		case request.Method == http.MethodGet && request.URL.Path == "/api/channel/models_enabled":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []string{"claude-fable-5-1", "other"}})
		case request.Method == http.MethodGet && request.URL.Path == "/api/pricing":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": []any{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer newAPI.Close()

	service := New(&privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", BaseURL: newAPI.URL, AdminKey: "admin", UserID: "1",
	}}, &repositoryStub{}, newAPI.Client(), nil, nil)
	expression := `tier("base", p * 10 + c * 50 + cr * 0.25 + cc * 12.5 + cc1h * 20)`
	snapshot, err := service.SaveModelPrices(context.Background(), "platform-1", []ModelPriceInput{{
		Model: "claude-fable-5-1", InputRatio: "5", CompletionRatio: "5",
		CacheRatio: "0.025", CreateCacheRatio: "1.25",
		BillingMode: "tiered_expr", BillingExpr: expression,
	}})
	if err != nil {
		t.Fatal(err)
	}

	modes, err := decodeStringMap(options["billing_setting.billing_mode"])
	if err != nil {
		t.Fatal(err)
	}
	expressions, err := decodeStringMap(options["billing_setting.billing_expr"])
	if err != nil {
		t.Fatal(err)
	}
	if modes["claude-fable-5-1"] != "tiered_expr" || expressions["claude-fable-5-1"] != expression {
		t.Fatalf("mode=%q expression=%q", modes["claude-fable-5-1"], expressions["claude-fable-5-1"])
	}
	if modes["other"] != "tiered_expr" || expressions["other"] == "" {
		t.Fatalf("unrelated expression configuration was lost: modes=%#v expressions=%#v", modes, expressions)
	}

	var written *ModelPrice
	for index := range snapshot.Models {
		if snapshot.Models[index].Model == "claude-fable-5-1" {
			written = &snapshot.Models[index]
			break
		}
	}
	if written == nil || written.BillingMode != "tiered_expr" || written.BillingExpr != expression {
		t.Fatalf("written model was not read back: %#v", written)
	}
}

func TestSaveModelPricesRollsBackEarlierOptionsWhenLaterWriteFails(t *testing.T) {
	options := map[string]string{
		"ModelPrice":                   `{}`,
		"ModelRatio":                   `{}`,
		"CompletionRatio":              `{}`,
		"CacheRatio":                   `{}`,
		"CreateCacheRatio":             `{}`,
		"ImageRatio":                   `{}`,
		"AudioRatio":                   `{}`,
		"AudioCompletionRatio":         `{}`,
		"billing_setting.billing_mode": `{}`,
		"billing_setting.billing_expr": `{}`,
	}
	putCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": options})
			return
		}
		putCalls++
		var input struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if putCalls == 2 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": false, "error": "write failed"})
			return
		}
		options[input.Key] = input.Value
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": true})
	}))
	defer server.Close()
	service := New(&privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", BaseURL: server.URL, AdminKey: "admin", UserID: "1",
	}}, &repositoryStub{}, server.Client(), nil, nil)

	_, err := service.SaveModelPrices(context.Background(), "platform-1", []ModelPriceInput{{
		Model: "fixed-model", ModelPrice: "3.5",
	}})

	if err == nil {
		t.Fatal("later option failure was accepted")
	}
	if putCalls != 3 || options["ModelPrice"] != `{}` {
		t.Fatalf("put_calls=%d options=%#v", putCalls, options)
	}
}

func TestSaveModelPricesSerializesConcurrentReadModifyWriteOperations(t *testing.T) {
	options := map[string]string{
		"ModelPrice": `{}`, "ModelRatio": `{}`, "CompletionRatio": `{}`, "CacheRatio": `{}`,
		"CreateCacheRatio": `{}`, "ImageRatio": `{}`, "AudioRatio": `{}`, "AudioCompletionRatio": `{}`,
		"billing_setting.billing_mode": `{}`, "billing_setting.billing_expr": `{}`,
	}
	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var requestMu sync.Mutex
	getCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestMu.Lock()
		getCalls++
		call := getCalls
		requestMu.Unlock()
		if call == 1 {
			close(firstEntered)
			<-releaseFirst
		} else if call == 2 {
			close(secondEntered)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": options})
	}))
	defer server.Close()
	service := New(&privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", BaseURL: server.URL, AdminKey: "admin", UserID: "1",
	}}, &repositoryStub{}, server.Client(), nil, nil)
	results := make(chan error, 2)
	go func() {
		_, err := service.SaveModelPrices(context.Background(), "platform-1", []ModelPriceInput{{Model: "first", InputRatio: "invalid", CompletionRatio: "1"}})
		results <- err
	}()
	<-firstEntered
	go func() {
		_, err := service.SaveModelPrices(context.Background(), "platform-1", []ModelPriceInput{{Model: "second", InputRatio: "invalid", CompletionRatio: "1"}})
		results <- err
	}()
	select {
	case <-secondEntered:
		t.Fatal("second read-modify-write operation entered before the first completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	for range 2 {
		if err := <-results; err == nil {
			t.Fatal("invalid model input was accepted")
		}
	}
}

func TestDecodeConfiguredModelsIncludesAllPriceDimensions(t *testing.T) {
	models, err := decodeConfiguredModels(map[string]string{
		"ModelRatio":           `{"gpt-5":1}`,
		"CompletionRatio":      `{"gpt-5":2}`,
		"CacheRatio":           `{"gpt-5":0.1}`,
		"CreateCacheRatio":     `{"gpt-5":0.2}`,
		"CreateCache1hRatio":   `{"gpt-5":0.3}`,
		"ImageRatio":           `{"gpt-5":0.4}`,
		"AudioRatio":           `{"gpt-5":0.5}`,
		"AudioCompletionRatio": `{"gpt-5":0.6}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("models=%#v", models)
	}
	model := models[0]
	if model.CacheRatio != "0.1" || model.CreateCacheRatio != "0.2" ||
		model.CreateCache1hRatio != "0.3" || model.ImageRatio != "0.4" ||
		model.AudioRatio != "0.5" || model.AudioCompletionRatio != "0.6" {
		t.Fatalf("model=%#v", model)
	}
}

func TestDecodeConfiguredModelsIncludesBillingOnlyModels(t *testing.T) {
	modelRatios := make(map[string]int, 30)
	for index := 1; index <= 30; index++ {
		modelRatios[fmt.Sprintf("token-model-%02d", index)] = index
	}
	ratioJSON, err := json.Marshal(modelRatios)
	if err != nil {
		t.Fatal(err)
	}

	models, err := decodeConfiguredModels(map[string]string{
		"ModelRatio":                   string(ratioJSON),
		"billing_setting.billing_mode": `{"expr-only":"tiered_expr","second-only":"per_second"}`,
		"billing_setting.billing_expr": `{"expr-only":"tier(0, 1)","expression-map-only":"tier(0, 2)"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 33 {
		t.Fatalf("models count=%d, want 33: %#v", len(models), models)
	}

	byModel := make(map[string]ModelPrice, len(models))
	for _, model := range models {
		byModel[model.Model] = model
	}
	if byModel["expr-only"].BillingMode != "tiered_expr" || byModel["expr-only"].BillingExpr != "tier(0, 1)" {
		t.Fatalf("expression model=%#v", byModel["expr-only"])
	}
	if byModel["second-only"].BillingMode != "per-second" {
		t.Fatalf("per-second model=%#v", byModel["second-only"])
	}
	if byModel["expression-map-only"].BillingExpr != "tier(0, 2)" {
		t.Fatalf("expression-only model=%#v", byModel["expression-map-only"])
	}
}

func TestDecodeToolPricesMergesNewAPIDefaultsAndOverrides(t *testing.T) {
	defaults := decodeToolPrices("")
	if len(defaults) != 9 {
		t.Fatalf("default tool prices count=%d, want 9: %#v", len(defaults), defaults)
	}
	if defaults[0].Tool != "web_search" || defaults[8].Tool != "image_generation" {
		t.Fatalf("default tool price order=%#v", defaults)
	}

	overridden := decodeToolPrices(`{"web_search":0,"custom_tool":3.5}`)
	if len(overridden) != 10 {
		t.Fatalf("overridden tool prices count=%d, want 10: %#v", len(overridden), overridden)
	}
	byTool := make(map[string]string, len(overridden))
	for _, item := range overridden {
		byTool[item.Tool] = item.Price
	}
	if byTool["web_search"] != "0" || byTool["custom_tool"] != "3.5" || byTool["image_generation"] != "150" {
		t.Fatalf("tool prices=%#v", byTool)
	}
}

func TestDecodeConfiguredModelsMatchesNewAPIModelPricingCount(t *testing.T) {
	modelRatios := make(map[string]int, 31)
	for index := 1; index <= 31; index++ {
		modelRatios[fmt.Sprintf("token-model-%02d", index)] = 1
	}
	ratioJSON, err := json.Marshal(modelRatios)
	if err != nil {
		t.Fatal(err)
	}

	models, err := decodeConfiguredModels(map[string]string{
		"ModelRatio":                   string(ratioJSON),
		"billing_setting.billing_mode": `{"expression-model-a":"tiered_expr","expression-model-b":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"expression-model-a":"tier(1,2)","expression-model-b":"tier(3,4)"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 33 {
		t.Fatalf("model pricing rows=%d, want 33: %#v", len(models), models)
	}
	if models[0].Model != "expression-model-a" || models[0].BillingMode != "tiered_expr" || models[0].BillingExpr != "tier(1,2)" {
		t.Fatalf("expression-only model was not preserved: %#v", models[0])
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

func TestSaveBindingsRejectsMalformedAndDuplicateGroupsBeforeWrite(t *testing.T) {
	valid := GroupBindingInput{
		NewAPIGroupID: "vip", NewAPIGroupName: "VIP", Sub2APIGroupID: "6",
	}
	for name, inputs := range map[string][]GroupBindingInput{
		"missing New API ID":   {{NewAPIGroupName: "VIP", Sub2APIGroupID: "6"}},
		"missing New API name": {{NewAPIGroupID: "vip", Sub2APIGroupID: "6"}},
		"duplicate New API ID": {valid, {NewAPIGroupID: " vip ", NewAPIGroupName: "重复", Sub2APIGroupID: "6"}},
	} {
		t.Run(name, func(t *testing.T) {
			repository := &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}
			service := New(&privateStub{platform: configstore.NewAPIPlatform{ID: "platform-1"}}, repository, nil, nil, nil)

			_, err := service.SaveBindings(context.Background(), "platform-1", inputs)

			if err == nil || KindOf(err) != ErrorValidation {
				t.Fatalf("error=%v kind=%q", err, KindOf(err))
			}
			if len(repository.bindings) != 0 {
				t.Fatalf("invalid bindings were persisted: %#v", repository.bindings)
			}
		})
	}
}

func TestSaveBindingsRestoresPreviousLocalBindingsWhenRatioSyncFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]string{"GroupRatio": `{"vip":1}`}})
			return
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": false, "error": "write failed"})
	}))
	defer server.Close()
	ratio := "0.35"
	previous := []business.NewAPIGroupBinding{{
		PlatformID: "platform-1", NewAPIGroupID: "old", NewAPIGroupName: "旧分组", Sub2APIGroupID: "6",
	}}
	repository := &repositoryStub{
		groups:   []business.NewAPILocalGroup{{ID: "6", Name: "低价", Ratio: &ratio}},
		bindings: append([]business.NewAPIGroupBinding(nil), previous...),
	}
	service := New(&privateStub{platform: configstore.NewAPIPlatform{
		ID: "platform-1", BaseURL: server.URL, AdminKey: "key", UserID: "1",
	}}, repository, server.Client(), nil, nil)

	_, err := service.SaveBindings(context.Background(), "platform-1", []GroupBindingInput{{
		NewAPIGroupID: "vip", NewAPIGroupName: "VIP", Sub2APIGroupID: "6", SyncRatio: true,
	}})

	if err == nil || !reflect.DeepEqual(repository.bindings, previous) {
		t.Fatalf("bindings=%#v err=%v", repository.bindings, err)
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

func TestDeletePlatformRemovesDependentGroupBindings(t *testing.T) {
	repository := &repositoryStub{bindings: []business.NewAPIGroupBinding{{
		PlatformID: "platform-1", NewAPIGroupID: "vip", Sub2APIGroupID: "6",
	}}}
	service := New(&privateStub{platform: configstore.NewAPIPlatform{ID: "platform-1"}}, repository, nil, nil, nil)

	deleted, err := service.DeletePlatform(context.Background(), "platform-1")

	if err != nil || !deleted || len(repository.bindings) != 0 {
		t.Fatalf("deleted=%t bindings=%#v err=%v", deleted, repository.bindings, err)
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
			if request.Header.Get("Authorization") != "Bearer user-jwt" || request.Header.Get("X-API-Key") != "" {
				t.Fatalf("key request headers=%#v", request.Header)
			}
			if request.Method == http.MethodGet {
				_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": []any{}})
				return
			}
			if request.Method != http.MethodPost {
				t.Fatalf("key request method=%s", request.Method)
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
	wantPaths := []string{"POST /api/v1/auth/login", "GET /api/v1/user/profile", "GET /api/v1/keys", "POST /api/v1/keys", "GET /api/v1/settings/public"}
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
	createCalls := 0
	newAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/option/":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]string{
				"GroupRatio": `{"default":1,"vip":2}`,
			}})
		case request.Method == http.MethodPost && request.URL.Path == "/api/channel/":
			createCalls++
			if err := json.NewDecoder(request.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]any{"id": 12}})
		case request.Method == http.MethodGet && request.URL.Path == "/api/channel/":
			items := []any{}
			if created != nil {
				items = append(items, created["channel"])
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]any{"items": items}})
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
	input := ChannelInput{
		Sub2APIGroupID: "6", KeyID: "key-7", BaseURL: "https://edge.example/v1", Models: []string{"gpt-5.2"},
		NewAPIGroups: []string{"vip", "default"},
	}
	_, err := service.CreateChannel(context.Background(), "platform-1", input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateChannel(context.Background(), "platform-1", input); err != nil {
		t.Fatal(err)
	}
	channel, ok := created["channel"].(map[string]any)
	if !ok {
		t.Fatalf("request missing channel wrapper: %#v", created)
	}
	name, _ := channel["name"].(string)
	if created["mode"] != "single" || channel["type"] != float64(59) || !strings.HasPrefix(name, "NewAPI-标准-console-") ||
		channel["base_url"] != "https://edge.example/v1" || channel["key"] != "sub2api-user-key" ||
		channel["models"] != "gpt-5.2" || channel["group"] != "default,vip" {
		t.Fatalf("unexpected create request: %#v", created)
	}
	if createCalls != 1 {
		t.Fatalf("idempotent retry issued %d create requests", createCalls)
	}
}

func TestCreateChannelReconcilesAfterResponseConnectionBreaks(t *testing.T) {
	var created map[string]any
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/option/":
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]string{"GroupRatio": `{"default":1}`}})
		case request.Method == http.MethodGet && request.URL.Path == "/api/channel/":
			items := []any{}
			if created != nil {
				items = append(items, created["channel"])
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "data": map[string]any{"items": items}})
		case request.Method == http.MethodPost && request.URL.Path == "/api/channel/":
			createCalls++
			if err := json.NewDecoder(request.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			connection, buffer, err := writer.(http.Hijacker).Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Fprint(buffer, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 200\r\n\r\n{\"success\":true")
			_ = buffer.Flush()
			_ = connection.Close()
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1", BaseURL: server.URL, AdminKey: "admin", UserID: "1"},
		target:   configstore.TargetSettings{BaseURL: "https://sub2api.example"},
		storedSecret: &configstore.UpstreamKeySecret{
			Host: "sub2api.example", KeyID: "key-7", GroupID: "6", Secret: "sub2api-user-key",
		},
	}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, server.Client(), nil, nil)

	result, err := service.CreateChannel(context.Background(), "platform-1", ChannelInput{
		Sub2APIGroupID: "6", KeyID: "key-7", BaseURL: "https://edge.example/v1", Models: []string{"gpt-5.2"},
		NewAPIGroups: []string{"default"},
	})

	if err != nil || createCalls != 1 || !strings.HasPrefix(fmt.Sprint(result["name"]), "NewAPI-标准-console-") {
		t.Fatalf("result=%#v create_calls=%d err=%v", result, createCalls, err)
	}
}

func TestCreateChannelKeyReusesStableMarkerOnRetry(t *testing.T) {
	username, password, token := "operator@example.test", "password", "user-jwt"
	private := &privateStub{
		platform: configstore.NewAPIPlatform{ID: "platform-1"},
		target:   configstore.TargetSettings{BaseURL: "https://sub2api.example"},
		vaultEntries: map[string]configstore.VaultEntry{
			"运营账号": {Entry: "运营账号", Username: &username, Password: &password},
		},
	}
	authenticator := &authenticatorStub{result: configstore.AuthRecord{AccessToken: &token}}
	keys := &keyManagerStub{created: upstreamsync.CreatedKey{KeyID: "17", GroupID: "6", Secret: "secret"}}
	service := New(private, &repositoryStub{groups: []business.NewAPILocalGroup{{ID: "6", Name: "标准"}}}, nil, keys, authenticator)
	input := ChannelKeyInput{Sub2APIGroupID: "6", CredentialSource: "vault", VaultEntry: "运营账号"}

	first, err := service.CreateChannelKey(context.Background(), "platform-1", input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateChannelKey(context.Background(), "platform-1", input)
	if err != nil {
		t.Fatal(err)
	}
	if keys.createCalls != 1 || first.KeyID != second.KeyID || !strings.HasPrefix(keys.createName, "NewAPI-标准-console-") {
		t.Fatalf("create_calls=%d first=%#v second=%#v marker=%q", keys.createCalls, first, second, keys.createName)
	}
}
