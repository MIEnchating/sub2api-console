package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestChannelGroupPageReadsSavedIDsRegardlessOfUpstreamPage(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/channel/")
		if r.Method != http.MethodGet || (id != "101" && id != "202") {
			t.Errorf("unexpected upstream operation: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
			"id": id, "name": "跨页渠道", "type": 59, "status": 1, "group": "default", "models": "gpt-5", "key": "private-key",
		}})
	}))
	defer remote.Close()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, UserID: "1", AdminKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveNewAPIChannelGroups(context.Background(), "primary", []configstore.NewAPIChannelGroup{{ID: "prod", Name: "生产", ChannelIDs: []string{"101", "202"}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	manager := newapimanagement.New(store, nil, remote.Client(), nil, nil)
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil, api.Dependencies{NewAPIManagement: manager})
	request := httptest.NewRequest(http.MethodGet, "/api/newapi/platforms/primary/channels?group_id=prod&page=1&page_size=1", nil)
	request.Header.Set("Authorization", "Bearer test-admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var result newapimanagement.ChannelPage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Total != 2 || len(result.Items) != 1 || result.Items[0].ID != "202" || result.Items[0].Version == "" {
		t.Fatalf("group page failed: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private-key") {
		t.Fatal("group page exposed credential")
	}
}

func TestChannelGroupWritesRequireAuthAndExplicitGroupList(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: "https://group-fixture.invalid", UserID: "1", AdminKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil, api.Dependencies{})
	for _, tc := range []struct {
		name, platform, body string
		authenticated        bool
		status               int
	}{
		{"unauthenticated", "primary", `{"groups":[]}`, false, 401},
		{"missing platform", "missing", `{"groups":[]}`, true, 404},
		{"missing groups", "primary", `{}`, true, 400},
		{"invalid name", "primary", `{"groups":[{"id":"prod","name":"","channel_ids":[]}]}`, true, 400},
		{"empty list", "primary", `{"groups":[],"version":""}`, true, 200},
		{"stale write", "primary", `{"groups":[],"version":""}`, true, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, "/api/newapi/platforms/"+tc.platform+"/channel-groups", strings.NewReader(tc.body))
			if tc.authenticated {
				request.Header.Set("Authorization", "Bearer test-admin")
			}
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d, expected %d: %s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}
