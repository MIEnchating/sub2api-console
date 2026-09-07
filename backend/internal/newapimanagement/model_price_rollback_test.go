package newapimanagement

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestSaveModelPricesRollsBackCurrentOptionAfterCommittedResponseFails(t *testing.T) {
	for _, failedKey := range []string{"ModelRatio", "billing_setting.billing_mode", "billing_setting.billing_expr"} {
		t.Run(failedKey, func(t *testing.T) {
			options := map[string]string{
				"ModelPrice": `{}`, "ModelRatio": `{"model-a":1}`, "CompletionRatio": `{"model-a":2}`,
				"CacheRatio": `{}`, "CreateCacheRatio": `{}`, "ImageRatio": `{}`, "AudioRatio": `{}`, "AudioCompletionRatio": `{}`,
				"billing_setting.billing_mode": `{}`, "billing_setting.billing_expr": `{}`,
			}
			previous := maps.Clone(options)
			failed := false
			client := &http.Client{Transport: pricingTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host != "newapi.example" || request.URL.Path != "/api/option/" {
					return nil, errors.New("unexpected test endpoint")
				}
				if request.Method == http.MethodPut {
					var input struct{ Key, Value string }
					if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
						return nil, err
					}
					options[input.Key] = input.Value
					if input.Key == failedKey && !failed {
						failed = true
						return nil, errors.New("response lost after remote commit")
					}
				}
				body, err := json.Marshal(map[string]any{"success": true, "data": options})
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}
			service := New(&privateStub{platform: configstore.NewAPIPlatform{
				ID: "primary", BaseURL: "https://newapi.example", AdminKey: "test-key", UserID: "1",
			}}, &repositoryStub{}, client, nil, nil)

			_, err := service.SaveModelPrices(context.Background(), "primary", []ModelPriceInput{{
				Model: "model-a", InputRatio: "3", CompletionRatio: "4",
				BillingMode: "tiered_expr", BillingExpr: `tier("standard", p * 6 + c * 24)`,
			}})

			if err == nil || !failed {
				t.Fatalf("expected the committed response failure, got %v", err)
			}
			if !reflect.DeepEqual(options, previous) {
				t.Fatalf("partial pricing update remained after rollback: got %#v, want %#v", options, previous)
			}
		})
	}
}
