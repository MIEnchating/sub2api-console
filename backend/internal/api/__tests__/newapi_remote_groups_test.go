package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestNewAPIRemoteGroupsRequiresAuthAndReadsOnlyGroupOptions(t *testing.T) {
	var reads atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/option/" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected remote request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"success":true,"data":{"GroupRatio":"{\"vip\":0.500000000000000001}","ModelRatio":"invalid unused pricing","private":"must-not-leak"}}`)
	}))
	defer remote.Close()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, AdminKey: "test-key", UserID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	service := newapimanagement.New(store, nil, remote.Client(), nil, nil)
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil, api.Dependencies{NewAPIManagement: service})
	path := "/api/newapi/platforms/primary/groups"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusUnauthorized || reads.Load() != 0 {
		t.Fatalf("unauthorized read: %d remote=%d", response.Code, reads.Load())
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer test-admin")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var result struct {
		Groups []newapimanagement.RemoteGroup `json:"groups"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || len(result.Groups) != 1 {
		t.Fatalf("group read: %d %s", response.Code, response.Body.String())
	}
	if reads.Load() != 1 || result.Groups[0].ID != "vip" || result.Groups[0].Ratio == nil || *result.Groups[0].Ratio != "0.500000000000000001" {
		t.Fatalf("reads=%d result=%+v", reads.Load(), result)
	}
	if strings.Contains(response.Body.String(), "must-not-leak") || strings.Contains(response.Body.String(), "ModelRatio") {
		t.Fatalf("unrelated options exposed: %s", response.Body.String())
	}
}

func TestNewAPIRemoteGroupsDistinguishesEmptyGroupsFromUpstreamFailure(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		status  int
	}{
		{"empty groups", `{"success":true,"data":{"GroupRatio":"{}"}}`, http.StatusOK},
		{"invalid ratio map", `{"success":true,"data":{"GroupRatio":"invalid"}}`, http.StatusBadGateway},
		{"business failure", `{"success":false,"message":"denied"}`, http.StatusBadGateway},
	} {
		t.Run(test.name, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, test.payload) }))
			defer remote.Close()
			store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			_, err = store.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, AdminKey: "test-key", UserID: "1"})
			if err != nil {
				t.Fatal(err)
			}
			service := newapimanagement.New(store, nil, remote.Client(), nil, nil)
			router := api.New(config.Config{AdminToken: "test-admin"}, store, nil, api.Dependencies{NewAPIManagement: service})
			request := httptest.NewRequest(http.MethodGet, "/api/newapi/platforms/primary/groups", nil)
			request.Header.Set("Authorization", "Bearer test-admin")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.status == http.StatusOK && response.Body.String() != `{"groups":[]}` {
				t.Fatalf("empty response=%s", response.Body.String())
			}
		})
	}
}
