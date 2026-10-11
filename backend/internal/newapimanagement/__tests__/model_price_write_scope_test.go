package newapimanagement_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestModelPriceUpdateWritesOnlyChangedOptionsAndStillReadsBack(t *testing.T) {
	for _, test := range []struct {
		name, input string
		keys        []string
	}{
		{"equivalent decimals", "1.0", nil},
		{"changed input ratio", "3", []string{"ModelRatio"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			options := map[string]string{"ModelRatio": `{"model-a":1,"untouched":5}`, "CompletionRatio": `{"model-a":2}`, "billing_setting.billing_mode": ` { } `, "billing_setting.billing_expr": `{}`}
			var writes []string
			page := officialPage
			service := setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
				mu.Lock()
				defer mu.Unlock()
				body := `{"success":true,"data":[]}`
				if r.URL.Path == "/api/option/" {
					if r.Method == http.MethodPut {
						var write struct{ Key, Value string }
						if err := json.NewDecoder(r.Body).Decode(&write); err != nil {
							return nil, err
						}
						writes = append(writes, write.Key)
						options[write.Key] = write.Value
					}
					raw, err := json.Marshal(map[string]any{"success": true, "data": options})
					if err != nil {
						return nil, err
					}
					body = string(raw)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			result, err := service.SaveModelPrices(context.Background(), "test", []newapimanagement.ModelPriceInput{{Model: "model-a", InputRatio: test.input, CompletionRatio: "2.0"}})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(writes, test.keys) {
				t.Fatalf("writes=%v want=%v", writes, test.keys)
			}
			if len(result.Models) != 2 || result.Models[1].Model != "untouched" || result.Models[1].InputRatio != "5" {
				t.Fatalf("readback lost models: %+v", result.Models)
			}
		})
	}
}
