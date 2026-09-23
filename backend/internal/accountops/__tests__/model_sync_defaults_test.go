package accountops_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
)

func TestModelDiscoveryDefaultsToExplicitRemoteModels(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mapping map[string]string
		want    []string
	}{
		{"explicit alias", map[string]string{"public-model": "known", "*": "*"}, []string{"known"}},
		{"unrestricted", map[string]string{}, []string{}},
		{"explicit wildcard target", map[string]string{"*": "known"}, []string{"known"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSettingsFixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/admin/accounts/41/models/sync-upstream" {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []string{"known", "discovered"}})
					return
				}
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "credentials": map[string]any{"model_mapping": tc.mapping}}})
			}))
			defer server.Close()
			service := accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
			service.UseTaskRunner(f.runner)
			if _, err := service.EnqueueModelDiscovery(context.Background(), []string{"41"}, "test"); err != nil {
				t.Fatal(err)
			}
			f.runner.run(context.Background())
			if f.tasks.last.Status != "succeeded" {
				t.Fatalf("discovery=%+v", f.tasks.last)
			}
			preview, err := service.ModelSyncPreview(context.Background(), []string{"41"})
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(preview.Accounts[0])
			var account struct {
				EnabledModels []string `json:"enabled_models"`
			}
			if err := json.Unmarshal(encoded, &account); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(account.EnabledModels, tc.want) {
				t.Fatalf("enabled=%v want=%v", account.EnabledModels, tc.want)
			}
		})
	}
}
