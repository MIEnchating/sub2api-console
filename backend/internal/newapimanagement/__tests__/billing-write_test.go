package newapimanagement_test

import (
	"context"
	"encoding/json"
	n "github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestBillingModeTransitionsKeepRequiredExpressions(t *testing.T) {
	for _, failure := range []string{"", "prepare", "mode", "cleanup"} {
		t.Run("committed failure "+failure, func(t *testing.T) {
			options := map[string]string{"ModelRatio": `{"old":1}`, "CompletionRatio": `{"old":2}`, "billing_setting.billing_mode": `{"old":"tiered_expr"}`, "billing_setting.billing_expr": `{"old":"tier(\"old\", p * 2 + c * 4)"}`}
			for _, k := range []string{"ModelPrice", "CacheRatio", "CreateCacheRatio", "ImageRatio", "AudioRatio", "AudioCompletionRatio"} {
				options[k] = `{}`
			}
			original := map[string]string{}
			for k, v := range options {
				original[k] = v
			}
			exprWrites := 0
			failed := false
			page := officialPage
			service := setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "newapi.test" {
					return nil, nil
				}
				var result any = map[string]any{"success": true, "data": []any{}}
				if r.URL.Path == "/api/option/" && r.Method == http.MethodGet {
					result = map[string]any{"success": true, "data": options}
				}
				if r.URL.Path == "/api/option/" && r.Method == http.MethodPut {
					var input struct {
						Key   string `json:"key"`
						Value string `json:"value"`
					}
					if e := json.NewDecoder(r.Body).Decode(&input); e != nil {
						t.Error(e)
					}
					candidate := map[string]string{}
					for k, v := range options {
						candidate[k] = v
					}
					candidate[input.Key] = input.Value
					var modes, exprs map[string]string
					_ = json.Unmarshal([]byte(candidate["billing_setting.billing_mode"]), &modes)
					_ = json.Unmarshal([]byte(candidate["billing_setting.billing_expr"]), &exprs)
					valid := true
					for model, mode := range modes {
						if mode == "tiered_expr" && strings.TrimSpace(exprs[model]) == "" {
							valid = false
						}
					}
					if !valid {
						result = map[string]any{"success": false, "message": "billing expression is required"}
					} else {
						options[input.Key] = input.Value
						stage := ""
						if input.Key == "billing_setting.billing_expr" {
							exprWrites++
							stage = "prepare"
							if exprWrites > 1 {
								stage = "cleanup"
							}
						}
						if input.Key == "billing_setting.billing_mode" {
							stage = "mode"
						}
						if failure != "" && stage == failure && !failed {
							failed = true
							result = map[string]any{"success": false, "message": "committed write response failed"}
						}
					}
				}
				raw, _ := json.Marshal(result)
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			}))
			snapshot, e := service.SaveModelPrices(context.Background(), "test", []n.ModelPriceInput{{Model: "new", InputRatio: "0.5", CompletionRatio: "4", BillingMode: "tiered_expr", BillingExpr: `tier("peak", p * 1 + c * 4)`}, {Model: "old", InputRatio: "1", CompletionRatio: "2"}})
			if failure != "" {
				if e == nil || !failed {
					t.Fatalf("failure not exercised: %v", e)
				}
				if !reflect.DeepEqual(options, original) {
					t.Fatalf("rollback lost original settings: %v", options)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			var modes, exprs map[string]string
			_ = json.Unmarshal([]byte(options["billing_setting.billing_mode"]), &modes)
			_ = json.Unmarshal([]byte(options["billing_setting.billing_expr"]), &exprs)
			if modes["new"] != "tiered_expr" || exprs["new"] == "" || modes["old"] != "" || exprs["old"] != "" {
				t.Fatalf("wrong final state: %v %v", modes, exprs)
			}
			for _, p := range snapshot.Models {
				if p.Model == "new" && p.BillingExpr == exprs["new"] {
					return
				}
			}
			t.Fatal("missing expression readback")
		})
	}
}
