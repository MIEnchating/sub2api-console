package adminclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

type responseTransport func(*http.Request) (*http.Response, error)

func (f responseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSystemLogTraceRejectsMissingOrInvalidStableID(t *testing.T) {
	for name, item := range map[string]string{
		"missing": `{"request_id":"request-1"}`,
		"null":    `{"id":null,"request_id":"request-1"}`,
		"zero":    `{"id":0,"request_id":"request-1"}`,
		"boolean": `{"id":true,"request_id":"request-1"}`,
		"object":  `{"id":{"value":1},"request_id":"request-1"}`,
		"text":    `{"id":"invalid","request_id":"request-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"items":[` + item + `],"total":1}}`)), Header: make(http.Header), Request: request}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			rows, err := client.SystemLogsByRequestID(context.Background(), "request-1", 60, 100)
			if err == nil || !strings.Contains(err.Error(), "稳定 ID") {
				t.Fatalf("invalid system log identity must fail: rows=%v, error=%v", rows, err)
			}
			if rows != nil {
				t.Fatal("invalid evidence must not be returned")
			}
		})
	}
}

func TestSystemLogTracePreservesNumericAndStringStableIDs(t *testing.T) {
	client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"items":[{"id":9007199254740993,"request_id":"request-1"},{"id":"9007199254740994","request_id":"request-1"}],"total":2}}`)), Header: make(http.Header), Request: request}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := client.SystemLogsByRequestID(context.Background(), "request-1", 60, 100)
	if err != nil || len(rows) != 2 {
		t.Fatalf("valid distinct log identities must be retained: rows=%v, error=%v", rows, err)
	}
	if rows[0]["id"] != json.Number("9007199254740993") || rows[1]["id"] != "9007199254740994" {
		t.Fatal("stable log identities lost their exact values")
	}
}
