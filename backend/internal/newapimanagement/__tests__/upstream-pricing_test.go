package newapimanagement_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func upstreamPricingSnapshot(t *testing.T, status int, body string) newapimanagement.RemoteSnapshot {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "upstream-prices.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, host := range []string{"upstream.test", "available.test"} {
		if err := store.SaveAuthRecord(context.Background(), configstore.AuthRecord{
			Host: host, BaseURL: "https://" + host, UpstreamType: "newapi", AuthMode: "newapi_user_token",
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		responseBody, responseStatus := `{"success":true,"data":[]}`, http.StatusOK
		switch r.URL.Host {
		case "newapi.test":
			if r.URL.Path != "/api/option/" && r.URL.Path != "/api/channel/models_enabled" && r.URL.Path != "/api/pricing" {
				return nil, fmt.Errorf("unexpected platform path %s", r.URL.Path)
			}
		case "upstream.test", "available.test":
			if r.URL.Path != "/api/pricing" {
				return nil, fmt.Errorf("unexpected upstream path %s", r.URL.Path)
			}
			responseBody = `{"success":true,"data":[{"model_name":"available-model","quota_type":0,"model_ratio":1,"completion_ratio":2,"model_price":0}]}`
			if r.URL.Host == "upstream.test" {
				responseBody, responseStatus = body, status
			}
		default:
			return nil, fmt.Errorf("test refuses external endpoint %s", r.URL.Host)
		}
		return &http.Response{StatusCode: responseStatus, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(responseBody))}, nil
	})}
	service := newapimanagement.New(&catalogStore{store}, nil, client, nil, nil)
	snapshot, err := service.Refresh(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestUpstreamTokenPricingIgnoresDefaultFixedPrice(t *testing.T) {
	snapshot := upstreamPricingSnapshot(t, http.StatusOK, `{"success":true,"data":[
		{"model_name":"token-model","quota_type":0,"model_ratio":1.25,"completion_ratio":4,"model_price":0}
	]}`)
	if len(snapshot.UpstreamPrices) != 2 || len(snapshot.UpstreamPrices[1].Models) != 1 {
		t.Fatalf("missing upstream prices: %+v", snapshot.UpstreamPrices)
	}
	price := snapshot.UpstreamPrices[1].Models[0]
	if price.ModelPrice != "" || price.InputPrice != "2.5" || price.CompletionPrice != "10" {
		t.Fatalf("token prices mistaken for fixed pricing: %+v", price)
	}
}

func TestUpstreamFreePerRequestPricingPreservesExplicitZero(t *testing.T) {
	snapshot := upstreamPricingSnapshot(t, http.StatusOK, `{"success":true,"data":[
		{"model_name":"free-request-model","quota_type":1,"model_ratio":0,"completion_ratio":0,"model_price":0}
	]}`)
	if len(snapshot.UpstreamPrices) != 2 || len(snapshot.UpstreamPrices[1].Models) != 1 {
		t.Fatalf("missing upstream prices: %+v", snapshot.UpstreamPrices)
	}
	if price := snapshot.UpstreamPrices[1].Models[0]; price.ModelPrice != "0" {
		t.Fatalf("explicit free per-request price lost: %+v", price)
	}
}

func TestUpstreamPricingFailureReportsCauseAndRetainsOtherCatalogs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		cause  string
	}{
		{"expired authentication", 401, `{"message":"private-test-token"}`, "鉴权"},
		{"blocked HTML response", 403, `<html>private-test-token</html>`, "403"},
		{"endpoint not enabled", 404, `<html>Not Found</html>`, "未开放"},
		{"invalid response", 200, `<html>private-test-token</html>`, "JSON"},
		{"business failure", 200, `{"success":false,"message":"private-test-token"}`, "拒绝"},
		{"empty catalog", 200, `{"success":true,"data":[]}`, "有效模型价格"},
		{"unrecognized catalog", 200, `{"success":true,"data":{"unexpected":[]}}`, "有效模型价格"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := upstreamPricingSnapshot(t, tc.status, tc.body)
			if len(snapshot.UpstreamPrices) != 1 || snapshot.UpstreamPrices[0].Host != "available.test" {
				t.Fatalf("successful catalog not retained: %+v", snapshot.UpstreamPrices)
			}
			raw, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Warning string `json:"upstream_price_warning"`
			}
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(response.Warning, "upstream.test") || !strings.Contains(response.Warning, tc.cause) {
				t.Fatalf("missing actionable upstream failure: %s", response.Warning)
			}
			if strings.Contains(string(raw), "private-test-token") {
				t.Fatal("upstream response leaked through warning")
			}
		})
	}
}
