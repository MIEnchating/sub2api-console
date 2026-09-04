package upstreamsync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestReadSub2APIKeyUsageUsesStableKeyFilterAndAcceptsExactZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/usage/stats" || request.URL.Query().Get("api_key_id") != "91" || request.URL.Query().Get("start_date") != "2026-08-29" {
			t.Fatalf("request=%s?%s", request.URL.Path, request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":{"total_actual_cost":9007199254740993.123456}}`))
	}))
	defer server.Close()
	token := "token"
	value, err := NewReader(server.Client()).ReadSub2APIKeyUsage(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, "91", "2026-08-29", "Asia/Shanghai")
	if err != nil || value.Cost != "9007199254740993.123456" || value.Source != "sub2api-key-stats" {
		t.Fatalf("value=%#v err=%v", value, err)
	}
}

func TestReadNewAPIKeyUsageUsesFlowAggregationByStableTokenID(t *testing.T) {
	flowCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":0.1,"enable_data_export":true}}`))
		case "/api/data/flow/self":
			flowCalls++
			if request.URL.Query().Get("start_timestamp") != "1" || request.URL.Query().Get("end_timestamp") != "199" {
				t.Fatalf("query=%s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"success":true,"data":[{"token_id":17,"use_group":"default","model_name":"gpt-5","quota":0.1},{"token_id":17,"use_group":"default","model_name":"gpt-5-mini","quota":0.2},{"token_id":18,"use_group":"default","model_name":"gpt-5","quota":0}]}`))
		default:
			t.Fatalf("unexpected historical log fallback: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	token := "token"
	values, err := NewReader(server.Client()).ReadNewAPIKeyUsage(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, time.Unix(1, 0), time.Unix(200, 0))
	if err != nil || flowCalls != 1 || values.Keys["17"].Cost != "3" || values.Keys["18"].Cost != "0" || values.Keys["17"].Source != "newapi-token-flow" || values.QuotaPerUnit != "0.1" {
		t.Fatalf("values=%#v calls=%d err=%v", values, flowCalls, err)
	}
}

func TestReadNewAPIKeyUsageRejectsFlowRowsWithoutStableTokenID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/status" {
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000,"enable_data_export":true}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"success":true,"data":[{"token_id":0,"quota":1}]}`))
	}))
	defer server.Close()
	_, err := NewReader(server.Client()).ReadNewAPIKeyUsage(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", Headers: map[string]string{}, Cookies: map[string]string{},
	}, time.Unix(1, 0), time.Unix(200, 0))
	if err == nil {
		t.Fatal("missing stable token id was accepted")
	}
}

func TestReadNewAPIKeyUsageDoesNotScanLogsWhenFlowDataIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000,"enable_data_export":false}}`))
		case "/api/log/self":
			t.Fatal("NewAPI logs must not be the automatic revenue fallback")
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token := "token"
	_, err := NewReader(server.Client()).ReadNewAPIKeyUsage(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, time.Unix(1, 0), time.Unix(200, 0))
	if err == nil || !strings.Contains(err.Error(), "稳定 Token ID") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadNewAPIKeyUsageRejectsLegacyDashboardWithoutTokenFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000,"enable_data_export":true}}`))
		case "/api/data/flow/self":
			writer.WriteHeader(http.StatusNotFound)
		case "/api/log/self", "/api/data/self":
			t.Fatalf("legacy logs or host totals must not be used for account revenue: %s", request.URL.Path)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token := "token"
	_, err := NewReader(server.Client()).ReadNewAPIKeyUsage(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, time.Unix(1, 0), time.Unix(200, 0))
	if err == nil || !strings.Contains(err.Error(), "版本不支持稳定 Token ID") {
		t.Fatalf("err=%v", err)
	}
}
