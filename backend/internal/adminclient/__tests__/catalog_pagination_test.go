package adminclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

// Model a management server whose large responses fail while reading the body.
type catalogTimeoutBody struct{}

func (catalogTimeoutBody) Read([]byte) (int, error) { return 0, context.DeadlineExceeded }
func (catalogTimeoutBody) Close() error             { return nil }

func TestCatalogUsesSmallPagesWhenLargeResponsesTimeOut(t *testing.T) {
	for _, path := range []string{"/admin/accounts", "/admin/groups"} {
		t.Run(path, func(t *testing.T) {
			var pages []int
			client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path != "/api/v1"+path {
					t.Fatalf("unexpected directory path: %s", request.URL.Path)
				}
				page, _ := strconv.Atoi(request.URL.Query().Get("page"))
				size, _ := strconv.Atoi(request.URL.Query().Get("page_size"))
				if size > 50 {
					return &http.Response{StatusCode: http.StatusOK, Body: catalogTimeoutBody{}, Header: make(http.Header)}, nil
				}
				if size < 1 || request.URL.Query().Get("lite") != "" {
					t.Fatalf("directory must retain full account fields: %s", request.URL.RawQuery)
				}
				pages = append(pages, page)
				items := []map[string]any{}
				for id := (page-1)*size + 1; id <= page*size && id <= 121; id++ {
					items = append(items, map[string]any{"id": id, "group_ids": []int{7}, "credentials": map[string]any{"base_url": "https://upstream.test/v1"}, "rate_multiplier": json.Number("1.000000000000000001")})
				}
				raw, _ := json.Marshal(map[string]any{"data": map[string]any{"items": items, "total": 121}})
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			fetch := client.Accounts
			if path == "/admin/groups" {
				fetch = client.Groups
			}
			items, err := fetch(t.Context())
			if err != nil {
				t.Fatalf("small-page directory read failed: %v", err)
			}
			if len(items) != 121 || !reflect.DeepEqual(pages, []int{1, 2, 3}) {
				t.Fatalf("incomplete catalog: count=%d pages=%v", len(items), pages)
			}
			for index, item := range items {
				if item["id"] != json.Number(strconv.Itoa(index+1)) || item["rate_multiplier"] != json.Number("1.000000000000000001") || item["credentials"].(map[string]any)["base_url"] != "https://upstream.test/v1" {
					t.Fatalf("catalog fields changed for row %d", index)
				}
			}
		})
	}
}

func TestCatalogBodyTimeoutReportsPageAndDiscardsPartialDirectory(t *testing.T) {
	client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("page") == "1" {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"items":[{"id":1}],"total":2}}`)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: catalogTimeoutBody{}, Header: make(http.Header)}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.Accounts(t.Context())
	if items != nil || err == nil {
		t.Fatalf("partial catalog was published: count=%d err=%v", len(items), err)
	}
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "第 2 页") || !strings.Contains(err.Error(), "已读取 1 项") || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("timeout lacks actionable pagination details: %v", err)
	}
}

func TestCatalogBodyTimeoutStopsAtConfiguredRetryLimit(t *testing.T) {
	calls := 0
	client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 2}, responseTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: catalogTimeoutBody{}, Header: make(http.Header)}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.Accounts(t.Context())
	if items != nil || !errors.Is(err, context.DeadlineExceeded) || calls != 2 {
		t.Fatalf("timeout retry limit changed: count=%d calls=%d err=%v", len(items), calls, err)
	}
}

func TestCatalogCancellationStopsWithoutRetryOrTimeoutAdvice(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 3}, responseTransport(func(*http.Request) (*http.Response, error) {
		calls++
		cancel()
		return nil, context.Canceled
	}))
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.Accounts(ctx)
	if items != nil || !errors.Is(err, context.Canceled) || calls != 1 || strings.Contains(err.Error(), "超时设置") {
		t.Fatalf("cancellation retried or reported as a request timeout: count=%d calls=%d err=%v", len(items), calls, err)
	}
}
