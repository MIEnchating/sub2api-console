package accountops

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type poolModeSyncTarget struct {
	settings configstore.AccountCreationSettings
	target   configstore.TargetSettings
}

func (target *poolModeSyncTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return target.target, nil
}

func (target *poolModeSyncTarget) AccountCreationSettings(context.Context) (configstore.AccountCreationSettings, error) {
	return target.settings, nil
}

func TestPoolModeSyncAppliesGroupPolicyAndPreservesOtherCredentials(t *testing.T) {
	repository, _, _ := accountRepository(t)
	credentials := map[string]any{"api_key": "retained", "base_url": "https://provider.example/v1", "pool_mode": false}
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		if request.Method == http.MethodPut {
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Fatal(err)
			}
			credentials = body["credentials"].(map[string]any)
			_, _ = io.WriteString(writer, `{"success":true,"data":{"id":41}}`)
			return
		}
		encoded, _ := json.Marshal(map[string]any{"data": map[string]any{
			"id": 41, "name": "alpha", "group_ids": []int{6}, "credentials": credentials,
		}})
		_, _ = writer.Write(encoded)
	}))
	defer server.Close()
	target := &poolModeSyncTarget{
		target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2},
		settings: configstore.AccountCreationSettings{
			Default: configstore.AccountCreationPolicy{PoolMode: false, PoolModeRetryCount: 3, PoolModeRetryStatusCodes: []int{401, 403, 429}},
			Groups: []configstore.AccountCreationGroupSettings{{
				GroupID: "6", AccountCreationPolicy: configstore.AccountCreationPolicy{PoolMode: true, PoolModeRetryCount: 2, PoolModeRetryStatusCodes: []int{429, 503}},
			}},
		},
	}
	service := New(target, repository, nil)

	result := service.syncPoolModeAccount(context.Background(), "41", target.settings, "operator")

	if result.Status != "succeeded" || !result.RemoteWrite || !result.ReadbackConfirmed {
		t.Fatalf("result=%#v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	if credentials["api_key"] != "retained" || credentials["base_url"] != "https://provider.example/v1" ||
		credentials["pool_mode"] != true || credentials["pool_mode_retry_count"] != json.Number("2") ||
		!reflect.DeepEqual(credentials["pool_mode_retry_status_codes"], []any{json.Number("429"), json.Number("503")}) {
		t.Fatalf("credentials=%#v", credentials)
	}
}

func TestPoolModeSyncSkipsAccountWithConflictingGroupPolicies(t *testing.T) {
	repository, _, _ := accountRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":{"id":41,"name":"alpha","group_ids":[6,7],"credentials":{}}}`)
	}))
	defer server.Close()
	settings := configstore.AccountCreationSettings{
		Default: configstore.AccountCreationPolicy{PoolModeRetryCount: 3, PoolModeRetryStatusCodes: []int{401}},
		Groups: []configstore.AccountCreationGroupSettings{
			{GroupID: "6", AccountCreationPolicy: configstore.AccountCreationPolicy{PoolMode: true, PoolModeRetryCount: 2, PoolModeRetryStatusCodes: []int{429}}},
			{GroupID: "7", AccountCreationPolicy: configstore.AccountCreationPolicy{PoolMode: false, PoolModeRetryCount: 3, PoolModeRetryStatusCodes: []int{401}}},
		},
	}
	service := New(&poolModeSyncTarget{
		target: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}, settings: settings,
	}, repository, nil)

	result := service.syncPoolModeAccount(context.Background(), "41", settings, "operator")

	if result.Status != "skipped" || result.RemoteWrite || result.Error == "" {
		t.Fatalf("result=%#v", result)
	}
}
