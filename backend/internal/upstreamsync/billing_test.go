package upstreamsync

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		_, _ = writer.Write([]byte(`{"code":0,"data":{"total_actual_cost":0}}`))
	}))
	defer server.Close()
	token := "token"
	value, err := NewReader(server.Client()).ReadSub2APIKeyUsage(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, "91", "2026-08-29", "Asia/Shanghai")
	if err != nil || value.Cost != 0 || value.Source != "sub2api-key-stats" {
		t.Fatalf("value=%#v err=%v", value, err)
	}
}

func TestReadNewAPIKeyUsageSubtractsRefundsAfterStableDoublePass(t *testing.T) {
	logCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/log/self":
			logCalls++
			if request.URL.Query().Get("p") != "1" || request.URL.Query().Get("page_size") != "100" {
				t.Fatalf("query=%s", request.URL.RawQuery)
			}
			if request.URL.Query().Get("type") == "2" {
				_, _ = writer.Write([]byte(`{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"id":1,"type":2,"token_id":17,"quota":500000,"created_at":100,"request_id":"consume"}]}}`))
			} else {
				_, _ = writer.Write([]byte(`{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"id":1,"type":6,"token_id":17,"quota":100000,"created_at":101,"request_id":"refund"}]}}`))
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	token := "token"
	values, err := NewReader(server.Client()).ReadNewAPIKeyUsage(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token", AccessToken: &token,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, time.Unix(1, 0), time.Unix(200, 0))
	if err != nil || logCalls != 4 || values.Keys["17"].Cost != 0.8 || values.QuotaPerUnit != 500000 {
		t.Fatalf("values=%#v calls=%d err=%v", values, logCalls, err)
	}
}

func TestReadNewAPIKeyUsageRejectsNonzeroRowsWithoutStableTokenID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/status" {
			_, _ = writer.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"id":1,"type":2,"token_id":0,"quota":1,"created_at":100}]}}`))
	}))
	defer server.Close()
	_, err := NewReader(server.Client()).ReadNewAPIKeyUsage(context.Background(), configstore.AuthRecord{
		BaseURL: server.URL, UpstreamType: "newapi", Headers: map[string]string{}, Cookies: map[string]string{},
	}, time.Unix(1, 0), time.Unix(200, 0))
	if err == nil {
		t.Fatal("missing stable token id was accepted")
	}
}
