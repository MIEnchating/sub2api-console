package officialpricing_test

import (
	"context"
	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPublicRedirectRejectsAnotherOriginAndHTTP(t *testing.T) {
	for _, location := range []string{"https://other.test/prices", "http://api-docs.deepseek.com/prices"} {
		t.Run(location, func(t *testing.T) {
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != pricing.DeepSeekURL {
					t.Fatalf("followed disallowed redirect: %s", r.URL)
				}
				return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			if _, e := pricing.FetchDeepSeek(context.Background(), client); e == nil || !strings.Contains(e.Error(), "跳转超出") {
				t.Fatalf("unsafe redirect accepted: %v", e)
			}
		})
	}
}
