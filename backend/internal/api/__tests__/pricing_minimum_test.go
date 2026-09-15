package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
)

func pricingMinimumRouter(t *testing.T) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.Initialize(ctx, "tester", "isolated-password", "https://target.example", "test-admin-key"); err != nil {
		t.Fatal(err)
	}
	biz, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = biz.Close() })
	if err := biz.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = biz.SyncManagementSnapshot(ctx, nil, []map[string]any{
		{"id": json.Number("6"), "name": "standard", "platform": "openai", "rate_multiplier": json.Number("0.2")},
		{"id": json.Number("7"), "name": "budget", "platform": "openai", "rate_multiplier": json.Number("0.15")},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	service := pricing.New(biz, private, nil)
	router := api.New(config.Config{}, private, biz, api.Dependencies{Pricing: service})
	token, err := private.CreateSession(ctx, "tester", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return router, token
}

func pricingMinimumRequest(router http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://console.test"+path, strings.NewReader(body))
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://console.test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestPricingMinimumHTTPRoundTripPreservesDecimalAndClearsRestriction(t *testing.T) {
	router, token := pricingMinimumRouter(t)
	for _, minimums := range []string{`{"6":"0.10000000000000000001"}`, `{}`} {
		body := `{"enabled":false,"profit_margin":0.2,"exchange_group_sets":[["6","7"]],"exchange_group_set_names":["standard"],"interval_seconds":120,"write_concurrency":1,"group_min_cost_multipliers":` + minimums + `}`
		saved := pricingMinimumRequest(router, token, http.MethodPut, "/api/pricing/config", body)
		if saved.Code != http.StatusOK {
			t.Fatalf("save status=%d body=%s", saved.Code, saved.Body.String())
		}
		read := pricingMinimumRequest(router, token, http.MethodGet, "/api/pricing", "")
		if read.Code != http.StatusOK {
			t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
		}
		var want map[string]string
		if err := json.Unmarshal([]byte(minimums), &want); err != nil {
			t.Fatal(err)
		}
		for _, response := range []*httptest.ResponseRecorder{saved, read} {
			var snapshot struct {
				Config struct {
					Minimums map[string]string `json:"group_min_cost_multipliers"`
				} `json:"config"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(snapshot.Config.Minimums, want) {
				t.Fatalf("minimums=%v want=%v", snapshot.Config.Minimums, want)
			}
		}
	}
}

func TestPricingMinimumHTTPRejectsInvalidValuesWithoutSaving(t *testing.T) {
	router, token := pricingMinimumRouter(t)
	for _, minimums := range []string{`{"6":0.1}`, `{"6":"-0.1"}`, `{"99":"0.1"}`} {
		t.Run(minimums, func(t *testing.T) {
			body := `{"enabled":false,"profit_margin":0.2,"exchange_group_sets":[["6","7"]],"exchange_group_set_names":["standard"],"interval_seconds":120,"write_concurrency":1,"group_min_cost_multipliers":` + minimums + `}`
			response := pricingMinimumRequest(router, token, http.MethodPut, "/api/pricing/config", body)
			if response.Code < 400 || response.Code >= 500 {
				t.Fatalf("invalid minimum status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	read := pricingMinimumRequest(router, token, http.MethodGet, "/api/pricing", "")
	var snapshot struct {
		Config struct {
			Minimums map[string]string `json:"group_min_cost_multipliers"`
		} `json:"config"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if read.Code != http.StatusOK || len(snapshot.Config.Minimums) != 0 {
		t.Fatalf("invalid update changed configuration: %s", read.Body.String())
	}
}
