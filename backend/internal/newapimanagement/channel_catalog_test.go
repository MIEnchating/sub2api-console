package newapimanagement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestChannelLookupFollowsServerCappedPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("p") == "0" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":1,"name":"other"}],"total":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":2,"name":"desired"}],"total":2}}`))
	}))
	defer server.Close()
	service := New(nil, nil, server.Client(), nil, nil)
	_, found, err := service.findChannelByName(context.Background(), configstore.NewAPIPlatform{BaseURL: server.URL}, "desired")
	if err != nil || !found {
		t.Fatalf("existing channel on capped page was missed: found=%t err=%v", found, err)
	}
}

func TestChannelLookupRejectsIncompleteCatalog(t *testing.T) {
	for _, body := range []string{`{"success":true,"data":{}}`, `{"success":true,"data":{"items":[],"total":2}}`, `{"success":true,"data":{"items":[false],"total":1}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			service := New(nil, nil, server.Client(), nil, nil)
			if _, _, err := service.findChannelByName(context.Background(), configstore.NewAPIPlatform{BaseURL: server.URL}, "desired"); err == nil {
				t.Fatal("incomplete channel catalog treated as authoritative absence")
			}
		})
	}
}

func TestChannelModelReadRejectsBusinessFailureWithData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"message":"failed","data":[{"id":"model"}]}`))
	}))
	defer server.Close()
	service := New(nil, nil, server.Client(), nil, nil)
	if models, err := service.fetchSub2APIModels(context.Background(), server.URL, "test"); err == nil {
		t.Fatalf("failed model response accepted: %v", models)
	}
}

func TestChannelEndpointsUseFallbackWhenPublicSettingsReportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"message":"unavailable","data":{"api_base_url":"https://unverified.example"}}`))
	}))
	defer server.Close()
	service := New(nil, nil, server.Client(), nil, nil)
	endpoints := service.channelEndpoints(context.Background(), configstore.TargetSettings{BaseURL: server.URL})
	if len(endpoints) != 1 || endpoints[0].BaseURL != server.URL {
		t.Fatalf("business failure accepted as endpoint discovery: %#v", endpoints)
	}
}
