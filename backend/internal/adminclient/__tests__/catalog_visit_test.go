package adminclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func TestVisitAccountsConsumesEachPageBeforeRequestingNext(t *testing.T) {
	var visited []string
	client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "management.test" || r.URL.Path != "/api/v1/admin/accounts" || r.URL.Query().Get("page_size") != "50" {
			t.Fatal("unexpected catalog request")
		}
		body := `{"data":{"items":[{"id":1,"rate_multiplier":0.1234567890123456789}],"total":2}}`
		if r.URL.Query().Get("page") == "2" {
			if !reflect.DeepEqual(visited, []string{"1"}) {
				t.Fatal("previous page was retained before consumption")
			}
			body = `{"data":{"items":[{"id":2}],"total":2}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = client.VisitAccounts(t.Context(), func(row map[string]any) error {
		id := row["id"].(json.Number).String()
		if id == "1" && row["rate_multiplier"] != json.Number("0.1234567890123456789") {
			t.Fatal("decimal lost precision")
		}
		visited = append(visited, id)
		return nil
	})
	if err != nil || !reflect.DeepEqual(visited, []string{"1", "2"}) {
		t.Fatalf("visited=%v err=%v", visited, err)
	}
}

func TestVisitAccountsRejectsInvalidPagesBeforeVisitingThem(t *testing.T) {
	for _, test := range []struct{ name, second string }{
		{"duplicate across pages", `{"data":{"items":[{"id":1}],"total":2}}`},
		{"changed total", `{"data":{"items":[{"id":2}],"total":3}}`},
		{"missing id", `{"data":{"items":[{}],"total":2}}`},
		{"incomplete catalog", `{"data":{"items":[],"total":2}}`},
		{"excess items", `{"data":{"items":[{"id":2},{"id":3}],"total":2}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls, visited := 0, 0
			client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body := `{"data":{"items":[{"id":1}],"total":2}}`
				if calls > 1 {
					body = test.second
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			err = client.VisitAccounts(t.Context(), func(map[string]any) error { visited++; return nil })
			if err == nil || visited != 1 || calls != 2 {
				t.Fatalf("calls=%d visited=%d err=%v", calls, visited, err)
			}
		})
	}
}

func TestVisitAccountsStopsOnConsumerErrorOrCancellation(t *testing.T) {
	stop := errors.New("stop consumer")
	for _, cancelRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "consumer failure", true: "cancelled"}[cancelRequest], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls, visited := 0, 0
			client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"items":[{"id":1},{"id":2}],"total":3}}`))}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			err = client.VisitAccounts(ctx, func(map[string]any) error {
				visited++
				if cancelRequest {
					cancel()
					return nil
				}
				return stop
			})
			expected := stop
			if cancelRequest {
				expected = context.Canceled
			}
			if !errors.Is(err, expected) || calls != 1 || visited != 1 {
				t.Fatalf("calls=%d visited=%d err=%v", calls, visited, err)
			}
		})
	}
}
