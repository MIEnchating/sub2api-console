package management_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestUpstreamNameRepairPropagatesRemoteSiteNameToBoundAccount(t *testing.T) {
	for _, scenario := range []string{"success", "read-failure", "monitoring", "manual-name", "balance", "readback-failure"} {
		t.Run(scenario, func(t *testing.T) {
			store, db := rateCollectionStore(t)
			if scenario == "manual-name" {
				name := "自定义"
				if _, err := store.UpdateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "upstream.example", Name: &name, BaseURL: "https://upstream.example", AccountBaseURL: "https://models.example/v1", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "monitoring" {
				if _, err := store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
					t.Fatal(err)
				}
			}
			remoteName := "Relay-1"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if scenario == "balance" && r.URL.Path == "/api/v1/user/profile" {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 1, "balance": 10, "concurrency": 1}})
					return
				}
				if r.URL.Path != "/api/v1/settings/public" {
					t.Errorf("unexpected upstream request: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if scenario == "read-failure" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(`{"data":{"site_name":"夏雨"}}`))
			}))
			t.Cleanup(upstream.Close)
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/api/v1/admin/accounts/11" {
					t.Errorf("unexpected admin request: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if r.Method == http.MethodPut {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					if len(body) != 1 {
						t.Errorf("name repair changed other fields: %v", body)
					}
					if scenario != "readback-failure" {
						remoteName, _ = body["name"].(string)
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 11, "name": remoteName}})
			}))
			t.Cleanup(admin.Close)
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			token := "test-token"
			if err := private.SaveAuthRecord(t.Context(), configstore.AuthRecord{Host: "upstream.example", BaseURL: upstream.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token}, nil); err != nil {
				t.Fatal(err)
			}
			tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tasks.Close() })
			runner := &recoveryRunner{}
			maintenance := management.New(snapshotTarget{admin.URL}, store, tasks)
			maintenance.UseTaskRunner(runner)
			syncer := upstreamsync.New(store, private, upstreamsync.NewReader(upstream.Client()), nil, tasks, maintenance)
			syncer.UseTaskRunner(runner)
			scope := upstreamsync.Scope{Name: true}
			if scenario == "balance" {
				scope = upstreamsync.Scope{Balance: true}
			}
			queued, err := syncer.EnqueueAll(t.Context(), scope, "test", "upstream-name-repair")
			if err != nil {
				t.Fatal(err)
			}
			// Run the parent and any linked account task deterministically.
			for index := 0; index < len(runner.runs); index++ {
				runner.runs[index](context.Background())
			}
			final, err := tasks.Get(t.Context(), queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			expectedSite, expectedAccount := "夏雨", "夏雨-1"
			if scenario == "read-failure" {
				expectedSite = "Relay"
			}
			if scenario != "success" && scenario != "balance" {
				expectedAccount = "Relay-1"
			}
			if scenario == "manual-name" {
				expectedSite, expectedAccount = "自定义", "自定义-1"
			}
			config, err := store.UpstreamConfigurationHost(t.Context(), "upstream.example")
			if err != nil || config.Name != expectedSite {
				t.Fatalf("site name=%+v err=%v", config, err)
			}
			var localName string
			if err := db.QueryRow(`SELECT name FROM accounts WHERE id='11'`).Scan(&localName); err != nil {
				t.Fatal(err)
			}
			if localName != expectedAccount || remoteName != expectedAccount {
				t.Fatalf("account name local=%q remote=%q want=%q task=%+v", localName, remoteName, expectedAccount, final)
			}
			if scenario == "readback-failure" || scenario == "read-failure" {
				if final.Status != "failed" {
					t.Fatalf("failure reported success: %+v", final)
				}
			} else if final.Status != "succeeded" {
				t.Fatalf("name task failed: %+v", final)
			}
		})
	}
}
