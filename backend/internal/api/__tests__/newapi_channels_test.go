package api_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestChannelRoutesRequireAuthAndValidateStableIDAndBody(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page_size") != "20" || r.URL.Query().Get("p") != "1" {
			t.Errorf("pagination not forwarded: %s", r.URL.RawQuery)
		}
		if r.Method != http.MethodGet {
			t.Error("invalid request caused upstream write")
		}
		fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":42,"name":"渠道","type":59,"status":1,"group":"default","models":"gpt-5","key":"secret-must-not-leak"}],"total":1}}`)
	}))
	defer remote.Close()
	_, err = store.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "测试", BaseURL: remote.URL, UserID: "1", AdminKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	service := newapimanagement.New(store, nil, remote.Client(), nil, nil)
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil, api.Dependencies{NewAPIManagement: service})
	listPath := "/api/newapi/platforms/primary/channels"
	for _, path := range []string{listPath, listPath + "/42/models", listPath + "/models/batch", "/api/newapi/platforms/primary/channel-groups"} {
		response := httptest.NewRecorder()
		method := http.MethodGet
		if strings.HasSuffix(path, "/models") {
			method = http.MethodPut
		}
		if strings.HasSuffix(path, "/batch") {
			method = http.MethodPost
		}
		if strings.HasSuffix(path, "/channel-groups") {
			method = http.MethodGet
		}
		router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous channel request accepted: %d", response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/newapi/platforms/primary/channel-groups", nil)
	request.Header.Set("Authorization", "Bearer test-admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != `{"groups":[],"version":""}` {
		t.Fatalf("group list failed: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPut, "/api/newapi/platforms/primary/channel-groups", strings.NewReader(`{"groups":[{"id":"prod","name":"生产","channel_ids":["42"]}],"version":""}`))
	request.Header.Set("Authorization", "Bearer test-admin")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"生产"`) {
		t.Fatalf("group save failed: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, listPath+"?page=1&page_size=20", nil)
	request.Header.Set("Authorization", "Bearer test-admin")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var page newapimanagement.ChannelPage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 {
		t.Fatalf("list failed: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret-must-not-leak") {
		t.Fatal("list leaked upstream secret")
	}
	for _, query := range []string{"page_size=0", "page_size=101", "page_size=abc", "page=-1", "page=1000"} {
		request := httptest.NewRequest(http.MethodGet, listPath+"?"+query, nil)
		request.Header.Set("Authorization", "Bearer test-admin")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid pagination accepted: %s: %d", query, response.Code)
		}
	}

	for _, invalid := range []struct{ id, body string }{
		{"not-an-id", `{"action":"add","models":["gpt-5-mini"],"version":"` + page.Items[0].Version + `"}`},
		{"42", `{"action":"invalid","models":["gpt-5-mini"],"version":"` + page.Items[0].Version + `"}`},
		{"42", `{"action":"add","models":[],"version":"` + page.Items[0].Version + `"}`},
		{"42", `{"action":"add","models":["gpt-5-mini"],"version":"` + page.Items[0].Version + `","key":"injected"}`},
	} {
		request := httptest.NewRequest(http.MethodPut, listPath+"/"+invalid.id+"/models", strings.NewReader(invalid.body))
		request.Header.Set("Authorization", "Bearer test-admin")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		expected := http.StatusBadRequest
		if invalid.id == "not-an-id" {
			expected = http.StatusUnprocessableEntity
		}
		if response.Code != expected {
			t.Fatalf("invalid change returned %d: %s", response.Code, response.Body.String())
		}
	}
}
