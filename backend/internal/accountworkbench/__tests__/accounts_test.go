package accountworkbench_test

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixture(t *testing.T, payload string) (*accountworkbench.Service, *configstore.Store) {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "workbench.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err = store.ConfigureTarget(context.Background(), "https://isolated.invalid", "test-admin", 10); err != nil {
		t.Fatal(err)
	}
	service := accountworkbench.New(store)
	service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "isolated.invalid" {
			t.Fatalf("unexpected host: %s", r.URL.Host)
		}
		if r.Method != "GET" {
			t.Fatal("account directory attempted write")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	return service, store
}

func TestAccountListIncludesOnlyOAuthAndPublicConfiguration(t *testing.T) {
	service, _ := fixture(t, `{"data":{"items":[{"id":41,"platform":"openai","type":"oauth","name":"team","status":"error","schedulable":false,"concurrency":0,"rate_multiplier":0.1234567890123456789,"credentials":{"email":"owner@example.test","access_token":"private-secret","plan_type":"prolite","model_mapping":{"gpt-5":"gpt-5.6"}},"extra":{"codex_fingerprint_mode":"session","password":"private-password"},"groups":[{"id":7,"name":"team group"}]},{"id":42,"platform":"openai","type":"apikey"}],"total":2}}`)
	accounts, err := service.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("accounts=%d", len(accounts))
	}
	account := accounts[0]
	if account.ID != "41" || account.Email != "owner@example.test" || account.Concurrency != "0" || account.RateMultiplier != "0.1234567890123456789" || account.Plan != "prolite" || account.Fingerprint != "session" || account.ModelMapping["gpt-5"] != "gpt-5.6" {
		t.Fatal("account configuration not preserved")
	}
	raw, _ := json.Marshal(accounts)
	for _, secret := range []string{"private-secret", "private-password", "access_token"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("public response exposed credential")
		}
	}
}

func TestAccountListRejectsDuplicateIDs(t *testing.T) {
	service, _ := fixture(t, `{"data":{"items":[{"id":41,"platform":"openai","type":"oauth"},{"id":41,"platform":"openai","type":"oauth"}],"total":2}}`)
	if _, err := service.Accounts(context.Background()); err == nil {
		t.Fatal("duplicate account IDs accepted")
	}
}
