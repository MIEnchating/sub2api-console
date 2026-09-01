package onboarding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGatewayRequestDoesNotForwardBearerAcrossRedirect(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		destinationCalls.Add(1)
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, destination.URL, http.StatusFound)
	}))
	defer source.Close()

	_, status, err := gatewayRequest(context.Background(), source.URL, "probe-secret", http.MethodGet, "/v1/models", nil, nil)
	if err != nil || status != http.StatusFound {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if calls := destinationCalls.Load(); calls != 0 {
		t.Fatalf("redirect destination received %d requests", calls)
	}
}

func TestFetchProbeModelsRejectsTrailingJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-5.2"}]} {"data":[]}`))
	}))
	defer server.Close()

	_, err := fetchProbeModels(context.Background(), server.URL, "probe-secret")
	if err == nil || !strings.Contains(err.Error(), "尾随数据") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunGatewayProbeRejectsInvalidSuccessfulResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "empty 204", status: http.StatusNoContent, want: "不是有效 JSON"},
		{name: "trailing JSON", status: http.StatusOK, body: `{"model":"gpt-5.2"} trailing`, want: "尾随数据"},
		{name: "empty JSON object", status: http.StatusOK, body: `{}`, want: "缺少可验证"},
		{name: "business failure", status: http.StatusOK, body: `{"success":false,"error":{"message":"upstream unavailable"}}`, want: "业务失败"},
		{name: "business failure code", status: http.StatusOK, body: `{"code":500,"data":{"model":"gpt-5.2"}}`, want: "业务失败"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			result, err := runGatewayProbe(context.Background(), server.URL, "probe-secret", "gpt-5.2", nil)
			if err == nil || !strings.Contains(err.Error(), test.want) || result.Status != "failed" {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}
