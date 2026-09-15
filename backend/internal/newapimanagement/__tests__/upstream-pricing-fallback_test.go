package newapimanagement_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const availableChannelPrices = `{"code":0,"data":[{"name":"channel-a","platforms":[{"platform":"anthropic","supported_models":[{"name":"model-a","pricing":{"billing_mode":"token","input_price":0.000003,"output_price":0.000015,"cache_read_price":0.0000003,"cache_write_price":0.00000375,"cache_write_1h_price":0.000006,"intervals":[]}}]}]}]}`

func TestSub2APIMissingPlazaReadsAuthenticatedAvailableChannelPrices(t *testing.T) {
	token := "valid-user-token"
	paths := []string{}
	snapshot := upstreamPricingAuthSnapshot(t, configstore.AuthRecord{UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token}, func(r *http.Request) (int, string) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("available channel fallback must preserve the configured authentication")
		}
		switch r.URL.Path {
		case "/api/v1/model-plaza":
			return http.StatusNotFound, `{"code":404,"message":"Model plaza is not enabled"}`
		case "/api/v1/channels/available":
			return http.StatusOK, availableChannelPrices
		default:
			t.Errorf("unexpected upstream endpoint: %s", r.URL.Path)
			return http.StatusNotFound, `{}`
		}
	})
	if snapshot.UpstreamPriceWarning != "" || len(snapshot.UpstreamPrices) != 1 {
		t.Fatalf("available channel prices should remain comparable when plaza is disabled: %s", snapshot.UpstreamPriceWarning)
	}
	if !reflect.DeepEqual(paths, []string{"/api/v1/model-plaza", "/api/v1/channels/available"}) {
		t.Fatalf("unexpected fallback requests: %v", paths)
	}
	catalog := snapshot.UpstreamPrices[0]
	if !strings.Contains(catalog.Name, "渠道价卡") || len(catalog.Models) != 1 {
		t.Fatalf("fallback must identify its channel pricing source: %+v", catalog)
	}
	price := catalog.Models[0]
	if price.InputRatio != "1.5" || price.CompletionRatio != "5" || price.InputPrice != "3" || price.CompletionPrice != "15" || price.CacheRatio != "0.1" || price.CreateCacheRatio != "1.25" || price.CreateCache1hRatio != "2" {
		t.Fatalf("channel prices must retain per-token units and exact cache ratios: %+v", price)
	}
}

func TestSub2APIPlazaAuthenticationAndServiceFailuresDoNotTryOtherEndpoints(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			paths := []string{}
			snapshot := upstreamPricingAuthSnapshot(t, configstore.AuthRecord{UpstreamType: "sub2api", AuthMode: "sub2api_user_token"}, func(r *http.Request) (int, string) {
				paths = append(paths, r.URL.Path)
				return status, `{"message":"private-upstream-response"}`
			})
			if !reflect.DeepEqual(paths, []string{"/api/v1/model-plaza"}) || snapshot.UpstreamPriceWarning == "" || len(snapshot.UpstreamPrices) != 0 {
				t.Fatalf("non-404 failure must remain visible without fallback: paths=%v warning=%s", paths, snapshot.UpstreamPriceWarning)
			}
		})
	}
}

func TestSub2APIUnavailableFallbackExplainsBothEndpointsWithoutLeakingResponse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		cause  string
	}{
		{"expired login", http.StatusUnauthorized, `{"message":"private-upstream-response"}`, "鉴权恢复"},
		{"unsupported version", http.StatusNotFound, `{"message":"private-upstream-response"}`, "可用渠道"},
		{"disabled channels", http.StatusOK, `{"code":0,"data":[]}`, "可用渠道"},
		{"invalid payload", http.StatusOK, `<html>private-upstream-response</html>`, "JSON"},
		{"business failure", http.StatusOK, `{"code":403,"message":"private-upstream-response"}`, "拒绝"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := upstreamPricingAuthSnapshot(t, configstore.AuthRecord{UpstreamType: "sub2api", AuthMode: "sub2api_user_token"}, func(r *http.Request) (int, string) {
				if r.URL.Path == "/api/v1/model-plaza" {
					return http.StatusNotFound, `{}`
				}
				return tc.status, tc.body
			})
			for _, want := range []string{"/api/v1/model-plaza", "/api/v1/channels/available", tc.cause} {
				if !strings.Contains(snapshot.UpstreamPriceWarning, want) {
					t.Errorf("warning missing %q: %s", want, snapshot.UpstreamPriceWarning)
				}
			}
			if len(snapshot.UpstreamPrices) != 0 || strings.Contains(snapshot.UpstreamPriceWarning, "private-upstream-response") {
				t.Fatal("failed fallback must not produce a catalog or expose the upstream response")
			}
		})
	}
}
