package newapimanagement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestPositiveRatioRejectsUnboundedExponentAndNonDecimalSyntax(t *testing.T) {
	for _, raw := range []string{"1e1001", "1/2", "0x10"} {
		if positiveDecimal(raw) {
			t.Errorf("unsafe or non-decimal ratio accepted: %s", raw)
		}
	}
	if !positiveDecimal("0.35") {
		t.Fatal("valid positive decimal rejected")
	}
}

func TestPricingOptionsRejectTrailingJSONAndMalformedStringValues(t *testing.T) {
	if _, err := decodeDecimalMap(`{"model":1} {"other":2}`); err == nil {
		t.Error("numeric option accepted trailing JSON")
	}
	if _, err := decodeStringMap(`{"model":"tiered_expr"} {}`); err == nil {
		t.Error("billing option accepted trailing JSON")
	}
	if _, err := decodeStringMap(`{"model":"tiered_expr","other":123}`); err == nil {
		t.Error("billing option silently discarded existing non-string entry")
	}
}

func TestRefreshPreservesExactNumericReferencePrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/option/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"ModelRatio":"{\"precise-model\":9007199254740993}"}}`))
		case "/api/pricing":
			_, _ = w.Write([]byte(`{"success":true,"data":[{"model":"precise-model","model_ratio":9007199254740993,"completion_ratio":1}]}`))
		case "/api/channel/models_enabled":
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := New(&privateStub{platform: configstore.NewAPIPlatform{ID: "platform-1", BaseURL: server.URL, AdminKey: "test-key", UserID: "1"}}, &repositoryStub{}, server.Client(), nil, nil)
	result, err := service.Refresh(context.Background(), "platform-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.NewAPIModels) != 1 || result.NewAPIModels[0].InputRatio != "9007199254740993" {
		t.Fatalf("reference price lost decimal precision: %#v", result.NewAPIModels)
	}
}

func TestModelPlazaPreservesExactNumericPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"groups":[{"models":[{"name":"precise-model","official_pricing":{"input_price":0.1234567890123456789,"output_price":1}}]}]}}`))
	}))
	defer server.Close()
	service := New(nil, nil, server.Client(), nil, nil)
	payload, err := service.requestSub2APIModelPlaza(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	prices := decodeSub2APIModelPlaza(payload)
	if len(prices) != 1 || prices[0].InputPrice != "0.1234567890123456789" {
		t.Fatalf("model plaza price lost precision: %#v", prices)
	}
}

func TestPricingOptionMapsRejectAmbiguousModelKeys(t *testing.T) {
	for _, raw := range []string{`{" ":"1"}`, `{"model":"1"," model ":"2"}`} {
		t.Run(raw, func(t *testing.T) {
			if _, err := decodeDecimalMap(raw); err == nil {
				t.Error("numeric option silently lost an existing key")
			}
			if _, err := decodeStringMap(raw); err == nil {
				t.Error("string option silently lost an existing key")
			}
		})
	}
}
