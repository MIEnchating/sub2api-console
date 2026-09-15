package adminclient_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func BenchmarkAccountCatalogResponse(b *testing.B) {
	var payload strings.Builder
	payload.WriteString(`{"data":{"items":[`)
	for id := 1; id <= 1000; id++ {
		if id > 1 {
			payload.WriteByte(',')
		}
		fmt.Fprintf(&payload, `{"id":%d,"name":"account-%d","platform":"openai","type":"apikey","status":"active","group_ids":[1,2,3],"credentials":{"base_url":"https://upstream.test/v1","api_key":"test-only"}}`, id, id)
	}
	payload.WriteString(`],"total":1000}}`)
	raw := payload.String()
	client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.test", AdminKey: "test-only", Attempts: 1}, responseTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(raw)), Header: make(http.Header), Request: request}, nil
	}))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for b.Loop() {
		items, err := client.Accounts(context.Background())
		if err != nil || len(items) != 1000 {
			b.Fatalf("catalog response failed: items=%d, error=%v", len(items), err)
		}
	}
}
