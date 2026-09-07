package adminclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelPricingValidatesDefaultPricingResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		wantErr, found bool
	}{
		{"exact decimals", `{"code":0,"data":{"found":true,"input_price":1.23e-6,"output_price":0.00000492,"cache_read_price":0}}`, false, true},
		{"missing model", `{"code":0,"data":{"found":false}}`, false, false},
		{"business failure", `{"code":401,"message":"invalid key"}`, true, false},
		{"missing found", `{"code":0,"data":{"input_price":1}}`, true, false},
		{"missing input", `{"code":0,"data":{"found":true,"output_price":1}}`, true, false},
		{"negative price", `{"code":0,"data":{"found":true,"input_price":-1,"output_price":1}}`, true, false},
		{"invalid optional price", `{"code":0,"data":{"found":true,"input_price":1,"output_price":1,"cache_read_price":"NaN"}}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/prefix/api/v1/admin/channels/model-pricing" || r.URL.Query().Get("model") != "vendor/model + v1" || r.Header.Get("X-API-Key") != "test-secret" {
					t.Errorf("incorrect request path, model or authentication")
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL + "/prefix", AdminKey: "test-secret", Attempts: 1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			price, err := client.ModelPricing(context.Background(), "vendor/model + v1")
			if (err != nil) != tc.wantErr || price.Found != tc.found {
				t.Fatalf("price=%#v err=%v", price, err)
			}
			if tc.found && (price.InputPrice != "0.00000123" || price.CacheReadPrice != "0") {
				t.Fatalf("decimal precision lost: %#v", price)
			}
		})
	}
}
