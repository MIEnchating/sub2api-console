package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

type catalogTestTransport func(*http.Request) (*http.Response, error)

func (f catalogTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelPricingRoutesRequireAuthAndManualRefreshBypassesCache(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Initialize(ctx, "operator", "a secure test password", "https://sub2api.example", "admin-secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveNewAPIPlatform(ctx, configstore.NewAPIPlatform{ID: "primary", Name: "test", BaseURL: "https://newapi.example", AdminKey: "newapi-secret", UserID: "1"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: catalogTestTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"success":true,"data":[]}`
		if r.URL.Host == "raw.githubusercontent.com" {
			calls++
			body = `{"remote":{"input_cost_per_token":0.000001,"output_cost_per_token":0.000004}}`
		} else if r.URL.Host != "newapi.example" {
			return nil, fmt.Errorf("unexpected test endpoint")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := newapimanagement.New(store, nil, client, nil, nil)
	router := New(config.Config{AdminToken: "test-token"}, store, fakeBusiness{}, Dependencies{NewAPIManagement: service})
	path := "/api/newapi/platforms/primary/management-model-prices"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		endpoint := path
		if method == http.MethodPost {
			endpoint += "/refresh"
		}
		if response := request(t, router, method, endpoint, nil, ""); response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: %d", method, response.Code)
		}
	}
	if calls != 0 {
		t.Fatal("anonymous request reached upstream")
	}
	for range 2 {
		response := authenticatedRequest(t, router, http.MethodGet, path, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"source":"remote"`) || !strings.Contains(response.Body.String(), `"expires_at"`) {
			t.Fatalf("cached response: %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "secret") {
			t.Fatal("response exposed credentials")
		}
	}
	if calls != 1 {
		t.Fatalf("cached GET fetched %d times", calls)
	}
	response := authenticatedRequest(t, router, http.MethodPost, path+"/refresh", nil)
	if response.Code != http.StatusOK || calls != 2 {
		t.Fatalf("manual refresh: status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}
