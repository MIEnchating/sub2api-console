package accountworkbench_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func TestAccountCatalogPagesPreservePublicFieldsAndFilterOtherPlatforms(t *testing.T) {
	service, _ := fixture(t, "")
	pages := 0
	service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "isolated.invalid" || r.Method != http.MethodGet {
			t.Fatal("unexpected request")
		}
		pages++
		body := `{"data":{"items":[{"id":1,"platform":"openai","type":"apikey","credentials":{"api_key":"private-key"}},{"id":2,"platform":"openai","type":"oauth","credentials":{"email":"one@example.test","access_token":"private-token"}}],"total":3}}`
		if pages == 2 {
			body = `{"data":{"items":[{"id":3,"platform":"openai","type":"oauth","rate_multiplier":0.1234567890123456789,"credentials":{"model_mapping":{"source":"target"}},"account_groups":[{"group":{"id":7,"name":"团队"}}]}],"total":3}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	accounts, err := service.Accounts(t.Context())
	if err != nil || len(accounts) != 2 || pages != 2 {
		t.Fatalf("count=%d pages=%d err=%v", len(accounts), pages, err)
	}
	if accounts[0].ID != "2" || accounts[0].Email != "one@example.test" || accounts[1].ID != "3" || accounts[1].RateMultiplier != "0.1234567890123456789" || accounts[1].ModelMapping["source"] != "target" || len(accounts[1].Groups) != 1 || accounts[1].Groups[0].Name != "团队" {
		t.Fatal("public catalog fields changed")
	}
	raw, err := json.Marshal(accounts)
	if err != nil || strings.Contains(string(raw), "private-") {
		t.Fatal("public catalog contains credentials")
	}
}

func TestAccountCatalogDiscardsEarlierPagesWhenLaterPageFails(t *testing.T) {
	for _, body := range []string{
		`{"data":{"items":[{"id":1,"platform":"openai","type":"apikey"}],"total":2}}`,
		`{"data":{"items":[],"total":2}}`,
		`{"data":{"items":[{"id":2}],"total":3}}`,
		`{"success":false}`,
	} {
		t.Run(body, func(t *testing.T) {
			service, _ := fixture(t, "")
			calls := 0
			service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				response := `{"data":{"items":[{"id":1,"platform":"openai","type":"oauth"}],"total":2}}`
				if calls > 1 {
					response = body
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
			}))
			accounts, err := service.Accounts(t.Context())
			if err == nil || accounts != nil || calls != 2 {
				t.Fatalf("partial result count=%d calls=%d err=%v", len(accounts), calls, err)
			}
		})
	}
}

func TestAccountCatalogRejectsTargetChangeDuringRead(t *testing.T) {
	service, store := fixture(t, "")
	service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if err := store.ConfigureTarget(t.Context(), "https://other.invalid", "test-other", 10); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"items":[{"id":1,"platform":"openai","type":"oauth"}],"total":1}}`))}, nil
	}))
	accounts, err := service.Accounts(t.Context())
	if !errors.Is(err, targetguard.ErrChanged) || accounts != nil {
		t.Fatalf("count=%d err=%v", len(accounts), err)
	}
}
