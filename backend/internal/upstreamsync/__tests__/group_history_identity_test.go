package upstreamsync_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestGroupRenameKeepsBindingWithoutHistoryEvents(t *testing.T) {
	for _, field := range []string{"id", "groupId", "group_id"} {
		t.Run(field, func(t *testing.T) {
			store, syncCatalog := groupHistoryIdentityFixture(t)
			for _, name := range []string{"分组a", "分组b", "分组b"} {
				syncCatalog(fmt.Sprintf(`{"%s":{"%s":7,"ratio":"0.5"}}`, name, field))
			}
			rows, err := store.AllUpstreamGroupHistory(t.Context(), 100)
			if err != nil || len(rows) != 0 {
				t.Fatalf("rename produced addition/removal events: %+v, err=%v", rows, err)
			}
			audit, err := store.UpstreamGroupBindingAudit(t.Context())
			if err != nil || len(audit.Items) != 1 {
				t.Fatalf("binding audit=%+v err=%v", audit, err)
			}
			item := audit.Items[0]
			if item.GroupID == nil || *item.GroupID != "7" || item.Status != "present" || item.GroupName != "分组b" || item.AccountCount != 1 || item.Accounts[0].ID != "41" {
				t.Fatalf("rename changed account binding: %+v", item)
			}
		})
	}
}

func TestDifferentGroupIDWithSameNameRecordsAdditionAndConfirmedRemoval(t *testing.T) {
	store, syncCatalog := groupHistoryIdentityFixture(t)
	syncCatalog(`{"分组a":{"id":7,"ratio":"0.5"}}`)
	syncCatalog(`{"分组a":{"id":8,"ratio":"0.5"}}`)
	rows, err := store.AllUpstreamGroupHistory(t.Context(), 100)
	if err != nil || len(rows) != 1 || rows[0].GroupID != "8" || rows[0].ChangeType != "added" {
		t.Fatalf("new identity must produce only an addition before deletion confirmation: %+v err=%v", rows, err)
	}
	syncCatalog(`{"分组a":{"id":8,"ratio":"0.5"}}`)
	rows, err = store.AllUpstreamGroupHistory(t.Context(), 100)
	if err != nil || len(rows) != 2 || rows[0].GroupID != "7" || rows[0].ChangeType != "removed" {
		t.Fatalf("missing identity must produce a confirmed removal: %+v err=%v", rows, err)
	}
}

func TestNewGroupHistoryKeepsOnlyFinalRenamedName(t *testing.T) {
	store, syncCatalog := groupHistoryIdentityFixture(t)
	syncCatalog(`{"分组a":{"id":7,"ratio":"0.5"}}`)
	syncCatalog(`{"中间名称":{"id":7,"ratio":"0.5"},"新增分组":{"id":8,"ratio":"0.5"}}`)
	syncCatalog(`{"最终名称一":{"id":7,"ratio":"0.5"},"新增分组改名一":{"id":8,"ratio":"0.5"}}`)
	syncCatalog(`{"最终名称二":{"id":7,"ratio":"0.5"},"新增分组改名二":{"id":8,"ratio":"0.5"}}`)

	rows, err := store.AllUpstreamGroupHistory(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ChangeType != "added" || rows[0].GroupID != "8" || rows[0].GroupName != "新增分组改名二" {
		t.Fatalf("renamed addition history=%+v", rows)
	}
}

func groupHistoryIdentityFixture(t *testing.T) (*business.Store, func(string)) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "history.example", BaseURL: "https://history.example", UpstreamType: "newapi",
		AuthMode: "newapi_user_token", RechargeRate: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at)
		VALUES('41','账号a',1,'{}','now');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,metadata_json,updated_at)
		VALUES('41','history.example','17','key-a','分组a','7','local','{}','now')`)
	if err != nil {
		t.Fatal(err)
	}
	var payload atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self/groups":
			_, _ = fmt.Fprintf(w, `{"success":true,"data":%s}`, payload.Load().(string))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":17,"name":"key-a","group_id":7,"status":1}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	reader := upstreamsync.NewReader(server.Client())
	return store, func(groups string) {
		t.Helper()
		payload.Store(groups)
		catalog, err := reader.ReadCatalog(t.Context(), configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "newapi"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
			Host: "history.example", Catalog: &catalog, AuthenticationOK: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
}
