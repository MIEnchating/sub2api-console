package management_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
)

type snapshotTarget struct{ url string }

func (target snapshotTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: target.url, AdminKey: "test-key", TimeoutSeconds: 1}, nil
}

func (snapshotTarget) AccountDefaults(context.Context) (configstore.AccountDefaultsSettings, error) {
	return configstore.AccountDefaultsSettings{Concurrency: 10, Priority: 1}, nil
}

func snapshotStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = store.SyncManagementSnapshot(context.Background(), []map[string]any{
		{"id": "11", "name": "same-name", "group_ids": []any{"7"}},
		{"id": "12", "name": "same-name", "group_ids": []any{"7"}},
	}, []map[string]any{{"id": "7", "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestSyncCompleteCatalogRemovesMissingAccountsByStableID(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		accounts []map[string]any
		wantIDs  []string
	}{
		{"one account deleted with another using the same name", []map[string]any{{"id": "12", "name": "same-name", "group_ids": []any{"7"}}}, []string{"12"}},
		{"all accounts deleted", []map[string]any{}, []string{}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := snapshotStore(t)
			if _, err := db.Exec(`
				INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
				VALUES('11','upstream.example','key-1','test','codex','now');
				INSERT INTO paused_accounts(account_id,enabled,updated_at) VALUES('11',1,'now');
				INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('11',1,'now','now');
				INSERT INTO routing_baselines(account_id,captured_at) VALUES('11','now');
				INSERT INTO cleanup_states(account_id,eligible_since,updated_at) VALUES('11','now','now');
				INSERT INTO routing_decisions(account_id,group_name,updated_at) VALUES('11','codex','now');
				INSERT INTO account_health_evaluations(account_id,group_name,evaluated_at) VALUES('11','codex','now');
				INSERT INTO health_samples(account_id,group_name,result,source,evidence_key) VALUES('11','codex','passed','active-probe','history');
			`); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected remote write: %s", r.Method)
					http.Error(w, "read only", http.StatusMethodNotAllowed)
					return
				}
				items := scenario.accounts
				if r.URL.Path == "/api/v1/admin/groups" {
					items = []map[string]any{{"id": "7", "name": "codex"}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": items, "total": len(items)}})
			}))
			t.Cleanup(server.Close)
			service := management.New(snapshotTarget{server.URL}, store, nil)
			result, err := service.Sync(context.Background(), "test")
			if err != nil {
				t.Fatal(err)
			}
			if result.DeletedAccounts != 2-len(scenario.wantIDs) || result.RemoteWrite || !result.ReadOnly {
				t.Fatalf("incorrect deletion result: %#v", result)
			}
			ids, err := store.ManagementAccountIDs(context.Background())
			if err != nil || !reflect.DeepEqual(ids, scenario.wantIDs) {
				t.Fatalf("accounts after sync = %v, want %v: %v", ids, scenario.wantIDs, err)
			}
			for _, table := range []string{"account_groups", "paused_accounts", "manual_priority_accounts", "routing_baselines", "cleanup_states", "routing_decisions", "account_health_evaluations"} {
				var count int
				if err := db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE account_id='11'").Scan(&count); err != nil || count != 0 {
					t.Errorf("deleted account retained in %s: count=%d err=%v", table, count, err)
				}
			}
			var bindings, history, groupCount int
			if err := db.QueryRow(`SELECT
				(SELECT COUNT(*) FROM bindings WHERE local_account_id='11'),
				(SELECT COUNT(*) FROM health_samples WHERE account_id='11'),
				(SELECT account_count FROM local_groups WHERE remote_id='7')`).Scan(&bindings, &history, &groupCount); err != nil {
				t.Fatal(err)
			}
			if bindings != 0 || history != 1 || groupCount != len(scenario.wantIDs) {
				t.Fatalf("bindings=%d history=%d group count=%d", bindings, history, groupCount)
			}
			again, err := service.Sync(context.Background(), "test")
			if err != nil || again.DeletedAccounts != 0 {
				t.Fatalf("repeated sync must not report another deletion: %#v, %v", again, err)
			}
		})
	}
}

func TestSyncIncompleteOrInvalidCatalogKeepsLocalAccounts(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		payload string
	}{
		{"incomplete pagination", `{"data":{"items":[],"total":2}}`},
		{"duplicate IDs", `{"data":{"items":[{"id":12},{"id":12}],"total":2}}`},
		{"business failure", `{"success":false,"message":"catalog unavailable"}`},
		{"invalid account fields", `{"data":{"items":[{"id":12,"priority":"invalid"}],"total":1}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, _ := snapshotStore(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/admin/groups" {
					_, _ = w.Write([]byte(`{"data":{"items":[{"id":7,"name":"codex"}],"total":1}}`))
					return
				}
				_, _ = w.Write([]byte(scenario.payload))
			}))
			t.Cleanup(server.Close)
			service := management.New(snapshotTarget{server.URL}, store, nil)
			if _, err := service.Sync(context.Background(), "test"); err == nil {
				t.Fatal("invalid catalog accepted")
			}
			ids, err := store.ManagementAccountIDs(context.Background())
			if err != nil || !reflect.DeepEqual(ids, []string{"11", "12"}) {
				t.Fatalf("failed sync changed accounts: %v, %v", ids, err)
			}
		})
	}
}
