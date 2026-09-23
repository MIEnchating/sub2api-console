package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestChannelDiscoveryRequiresAuthVersionAndReturnsOnlyModels(t *testing.T) {
	body := `{"success":true,"data":["new-model","old","new-model"]}`
	fetches := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("wrong discovery request")
		}
		switch r.URL.Path {
		case "/api/channel/42":
			fmt.Fprint(w, `{"success":true,"data":{"id":42,"name":"channel","type":59,"status":1,"group":"default","models":"old","key":"private-secret"}}`)
		case "/api/channel/fetch_models/42":
			fetches++
			fmt.Fprint(w, body)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.SaveNewAPIPlatform(context.Background(), configstore.NewAPIPlatform{ID: "primary", Name: "test", BaseURL: remote.URL, AdminKey: "test-key", UserID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	service := newapimanagement.New(store, nil, remote.Client(), nil, nil)
	page, err := service.ChannelsByID(context.Background(), "primary", []string{"42"}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{AdminToken: "test-admin"}, store, nil, api.Dependencies{NewAPIManagement: service})
	path := "/api/newapi/platforms/primary/channels/42/models/available?version=" + page.Items[0].Version
	call := func(path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer test-admin")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := call(path, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status %d", w.Code)
	}
	if w := call(path, true); w.Code != http.StatusOK || w.Body.String() != `{"models":["new-model","old"]}` {
		t.Fatalf("discovery failed %d %s", w.Code, w.Body.String())
	}
	before := fetches
	if w := call(path+"x", true); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid version status %d", w.Code)
	}
	if fetches != before {
		t.Fatal("invalid version reached discovery")
	}
	body = `{"success":false,"message":"private-secret","data":["unsafe"]}`
	w := call(path, true)
	var payload map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &payload)
	if w.Code != http.StatusBadGateway || payload["models"] != nil || payload["detail"] == "private-secret" {
		t.Fatalf("business failure accepted or leaked: %s", w.Body.String())
	}
}
