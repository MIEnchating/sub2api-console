package newapimanagement_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestUpstreamChannelPricingConvertsTokenUnitsWithoutLosingSourcePrecision(t *testing.T) {
	snapshot := availableChannelPricingSnapshot(t, `{"code":0,"data":[{"platforms":[{"supported_models":[{"name":"precise","pricing":{"billing_mode":"token","input_price":0.000001234567890123,"output_price":0.000002469135780246,"cache_write_price":0.000002469135780246,"cache_write_1h_price":0.000003703703670369,"cache_read_price":0.0000006172839450615}}]}]}]}`)
	if snapshot.UpstreamPriceWarning != "" || len(snapshot.UpstreamPrices) != 1 || len(snapshot.UpstreamPrices[0].Models) != 1 {
		t.Fatalf("expected available channel pricing: %#v, %s", snapshot.UpstreamPrices, snapshot.UpstreamPriceWarning)
	}
	price := snapshot.UpstreamPrices[0].Models[0]
	if price.Model != "precise" || price.InputPrice != "1.234567890123" || price.CompletionPrice != "2.469135780246" || price.InputRatio != "0.617283945062" || price.CompletionRatio != "2" {
		t.Fatalf("token prices must retain decimal source precision: %#v", price)
	}
	if price.CacheRatio != "0.5" || price.CreateCacheRatio != "2" || price.CreateCache1hRatio != "3" || price.CacheCreatePrice != "2.469135780246" || price.CacheReadPrice != "0.617283945062" {
		t.Fatalf("cache prices must use the same token units: %#v", price)
	}
	if price.BillingMode != "" && price.BillingMode != "per-token" {
		t.Fatalf("token billing must use the console contract: %q", price.BillingMode)
	}
}

func TestUpstreamChannelPricingReadsLegacyModelsAndExplicitPerRequestPrice(t *testing.T) {
	snapshot := availableChannelPricingSnapshot(t, `{"code":0,"data":[{"supported_models":[{"name":"fixed","pricing":{"billing_mode":"per_request","per_request_price":0.123456789012}},{"name":"free","pricing":{"billing_mode":"token","input_price":0,"output_price":0}}]}]}`)
	if snapshot.UpstreamPriceWarning != "" || len(snapshot.UpstreamPrices) != 1 || len(snapshot.UpstreamPrices[0].Models) != 2 {
		t.Fatalf("expected legacy fixed and free prices: %#v, %s", snapshot.UpstreamPrices, snapshot.UpstreamPriceWarning)
	}
	prices := snapshot.UpstreamPrices[0].Models
	if prices[0].Model != "fixed" || prices[0].ModelPrice != "0.123456789012" || prices[0].BillingMode != "per-request" || prices[0].InputRatio != "" {
		t.Fatalf("fixed prices must remain per request: %#v", prices[0])
	}
	if prices[1].Model != "free" || prices[1].InputRatio != "0" || prices[1].CompletionRatio != "1" || prices[1].InputPrice != "0" || prices[1].CompletionPrice != "0" {
		t.Fatalf("explicit free token prices must remain comparable: %#v", prices[1])
	}
}

func TestUpstreamChannelPricingRejectsEmptyOrUnusableCatalogWithAction(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"disabled channels", `{"code":0,"data":[]}`},
		{"missing channels", `{"code":0,"data":{}}`},
		{"missing output", `{"code":0,"data":[{"supported_models":[{"name":"partial","pricing":{"input_price":0.000001}}]}]}`},
		{"missing fixed price", `{"code":0,"data":[{"supported_models":[{"name":"partial","pricing":{"billing_mode":"per_request","input_price":0.000001,"output_price":0.000002}}]}]}`},
		{"zero input and paid output", `{"code":0,"data":[{"supported_models":[{"name":"partial","pricing":{"input_price":0,"output_price":0.000002}}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := availableChannelPricingSnapshot(t, tc.body)
			if len(snapshot.UpstreamPrices) != 0 || !strings.Contains(snapshot.UpstreamPriceWarning, "可用渠道") {
				t.Fatalf("unusable catalog must identify available channel settings: %#v, %s", snapshot.UpstreamPrices, snapshot.UpstreamPriceWarning)
			}
		})
	}
}

func TestUpstreamChannelPricingRejectsInvalidSourcePrices(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pricing string
	}{
		{"negative input", `{"input_price":-0.000001,"output_price":0.000002}`},
		{"negative output", `{"input_price":0.000001,"output_price":-0.000002}`},
		{"negative cache", `{"input_price":0.000001,"output_price":0.000002,"cache_read_price":-1}`},
		{"nonnumeric price", `{"input_price":"not-a-number","output_price":0.000002}`},
		{"excessive exponent", `{"input_price":1e100000,"output_price":0.000002}`},
		{"negative fixed price", `{"billing_mode":"per_request","per_request_price":-1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := availableChannelPricingSnapshot(t, `{"code":0,"data":[{"supported_models":[{"name":"invalid","pricing":`+tc.pricing+`}]}]}`)
			if len(snapshot.UpstreamPrices) != 0 || !strings.Contains(snapshot.UpstreamPriceWarning, "价格") {
				t.Fatalf("invalid pricing must not become a comparable catalog: %#v, %s", snapshot.UpstreamPrices, snapshot.UpstreamPriceWarning)
			}
		})
	}
}

func TestUpstreamChannelPricingDeduplicatesEqualPricesAndOmitsConflictingModels(t *testing.T) {
	snapshot := availableChannelPricingSnapshot(t, `{"code":0,"data":[{"supported_models":[{"name":"stable","pricing":{"input_price":0.000001,"output_price":0.000002}},{"name":"conflict","pricing":{"input_price":0.000001,"output_price":0.000002}}]},{"platforms":[{"supported_models":[{"name":"stable","pricing":{"input_price":1e-6,"output_price":2e-6}},{"name":"conflict","pricing":{"input_price":0.000001,"output_price":0.000003}}]}]}]}`)
	if snapshot.UpstreamPriceWarning != "" || len(snapshot.UpstreamPrices) != 1 || len(snapshot.UpstreamPrices[0].Models) != 1 || snapshot.UpstreamPrices[0].Models[0].Model != "stable" {
		t.Fatalf("only unambiguous distinct model prices are comparable: %#v, %s", snapshot.UpstreamPrices, snapshot.UpstreamPriceWarning)
	}
}

func TestUpstreamChannelPricingDoesNotFlattenConditionalOrUnsupportedBilling(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pricing string
	}{
		{"token intervals", `{"billing_mode":"token","input_price":0.000001,"output_price":0.000002,"intervals":[{"min_tokens":1000,"input_price":0.000003}]}`},
		{"fixed intervals", `{"billing_mode":"per_request","per_request_price":0.1,"intervals":[{"per_request_price":0.2}]}`},
		{"image tokens", `{"billing_mode":"token","input_price":0.000001,"output_price":0.000002,"image_input_price":0.000003}`},
		{"image billing", `{"billing_mode":"image","input_price":0.000001,"output_price":0.000002,"per_request_price":0.1}`},
		{"unknown billing", `{"billing_mode":"future-mode","input_price":0.000001,"output_price":0.000002}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := availableChannelPricingSnapshot(t, `{"code":0,"data":[{"supported_models":[{"name":"complex","pricing":`+tc.pricing+`},{"name":"plain","pricing":{"input_price":0.000001,"output_price":0.000002}}]},{"supported_models":[{"name":"complex","pricing":{"input_price":0.000001,"output_price":0.000002}}]}]}`)
			if snapshot.UpstreamPriceWarning != "" || len(snapshot.UpstreamPrices) != 1 || len(snapshot.UpstreamPrices[0].Models) != 1 || snapshot.UpstreamPrices[0].Models[0].Model != "plain" {
				t.Fatalf("complex models must not borrow another channel's flat price: %#v, %s", snapshot.UpstreamPrices, snapshot.UpstreamPriceWarning)
			}
		})
	}
}

func availableChannelPricingSnapshot(t *testing.T, body string) newapimanagement.RemoteSnapshot {
	t.Helper()
	token := "channel-user-token"
	return upstreamPricingAuthSnapshot(t, configstore.AuthRecord{UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token}, func(r *http.Request) (int, string) {
		switch r.URL.Path {
		case "/api/v1/model-plaza":
			return http.StatusNotFound, `{"code":404,"message":"Model plaza is not enabled"}`
		case "/api/v1/channels/available":
			if r.Header.Get("Authorization") != "Bearer channel-user-token" {
				t.Error("available channels require configured authentication")
				return http.StatusUnauthorized, `{"code":401}`
			}
			return http.StatusOK, body
		default:
			t.Errorf("unexpected pricing endpoint: %s", r.URL.Path)
			return http.StatusNotFound, `{}`
		}
	})
}
