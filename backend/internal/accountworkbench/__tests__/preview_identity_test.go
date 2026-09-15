package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func identityAccount(id string, credentials map[string]any) map[string]any {
	return map[string]any{"id": json.Number(id), "platform": "openai", "type": "oauth", "credentials": credentials}
}

func TestPreviewIdentityMatchingPreservesStableIdentityAndCredentialFallback(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		incoming map[string]any
		current  map[string]any
		matches  bool
	}{
		{"complete-aliases", map[string]any{"access_token": "new-private", "account_id": "workspace", "user_id": "user"}, map[string]any{"access_token": "old-private", "chatgpt_account_id": "workspace", "chatgpt_user_id": "user"}, true},
		{"partial-same-token", map[string]any{"access_token": "same-private", "user_id": "user"}, map[string]any{"access_token": "same-private", "user_id": "user", "account_id": "workspace"}, true},
		{"partial-conflicting-user", map[string]any{"access_token": "same-private", "user_id": "other"}, map[string]any{"access_token": "same-private", "user_id": "user", "account_id": "workspace"}, false},
		{"partial-conflicting-workspace", map[string]any{"access_token": "same-private", "account_id": "other"}, map[string]any{"access_token": "same-private", "user_id": "user", "account_id": "workspace"}, false},
		{"complete-requires-complete-current", map[string]any{"access_token": "same-private", "user_id": "user", "account_id": "workspace"}, map[string]any{"access_token": "same-private", "user_id": "user"}, false},
		{"fingerprint-prefers-refresh", map[string]any{"access_token": "same-private", "refresh_token": "rt_new_private"}, map[string]any{"access_token": "same-private", "refresh_token": "rt_other_private"}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newImportFixture(t, nil)
			f.remote.accounts["101"] = identityAccount("101", scenario.current)
			raw, err := json.Marshal(map[string]any{"credentials": scenario.incoming})
			if err != nil {
				t.Fatal(err)
			}
			view := f.preview(t, string(raw), false)
			defer f.service.DeletePreview("owner", view.ID)
			if len(view.Items) != 1 || view.Items[0].Duplicate != scenario.matches {
				t.Fatalf("identity match = %+v", view.Items)
			}
			if scenario.matches && view.Items[0].AccountID != "101" {
				t.Fatal("matched identity lost its stable account ID")
			}
		})
	}
}

func TestPreviewRejectsAmbiguousStableIdentityFromJWTAndExtraMetadata(t *testing.T) {
	f := newImportFixture(t, nil)
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"workspace-1","chatgpt_user_id":"user-1"}}`))
	f.remote.accounts["101"] = identityAccount("101", map[string]any{"access_token": "header." + claims + ".signature"})
	f.remote.accounts["102"] = identityAccount("102", map[string]any{"access_token": "other-private"})
	f.remote.accounts["102"]["extra"] = map[string]any{"account_id": "workspace-1", "user_id": "user-1"}
	view, err := f.service.Preview(context.Background(), "owner", accountworkbench.PreviewInput{Content: importedJSON})
	if err == nil || view.ID != "" {
		t.Fatal("multiple stable account IDs produced an executable import preview")
	}
}

func BenchmarkPreviewIdentityMatching(b *testing.B) {
	accounts := make([]map[string]any, 1000)
	incoming := make([]map[string]any, 100)
	for index := range accounts {
		id := strconv.Itoa(index + 1)
		claims := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"https://api.openai.com/auth":{"chatgpt_account_id":"workspace-%s","chatgpt_user_id":"user-%s"}}`, id, id)))
		credentials := map[string]any{"access_token": "header." + claims + ".signature", "model_mapping": map[string]any{"requested": "served-model"}}
		accounts[index] = identityAccount(id, credentials)
		if index < len(incoming) {
			incoming[index] = map[string]any{"credentials": credentials}
		}
	}
	response, err := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"items": accounts, "total": len(accounts)}})
	if err != nil {
		b.Fatal(err)
	}
	content, err := json.Marshal(incoming)
	if err != nil {
		b.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(b.TempDir(), "config.sqlite3"))
	if err != nil {
		b.Fatal(err)
	}
	defer private.Close()
	if err := private.ConfigureTarget(context.Background(), "https://benchmark.example", "isolated-key", 3); err != nil {
		b.Fatal(err)
	}
	service := accountworkbench.New(private, nil, nil, nil, nil)
	service.UseTransport(oauthTransportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/admin/accounts" {
			return nil, fmt.Errorf("unexpected isolated benchmark request")
		}
		return oauthResponse(string(response)), nil
	}))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		view, err := service.Preview(context.Background(), "benchmark-owner", accountworkbench.PreviewInput{Content: string(content)})
		if err != nil || len(view.Items) != len(incoming) || !view.Items[0].Duplicate {
			b.Fatalf("preview failed: %v", err)
		}
		service.DeletePreview("benchmark-owner", view.ID)
	}
}
