package probe_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestAutomaticProbeSelectionRequiresTheStableGroupID(t *testing.T) {
	for _, test := range []struct {
		name      string
		groupID   any
		automatic bool
		allowed   bool
	}{
		{"matching name cannot include a different group ID", "8", true, false},
		{"matching name cannot replace a missing group ID", nil, true, false},
		{"matching stable group ID allows automatic probe", "7", true, true},
		{"manual diagnosis is independent of scheduling scope", "8", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "probe-scope.sqlite3")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','scope-test','{"known_models":["test-model"]}','now');
				INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','7',?)`, test.groupID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{
				"managed_group_mode": "selected", "managed_group_ids": []any{"7"},
			}}}, "test"); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("enqueue must not contact the upstream: %s", r.URL.Path)
				http.NotFound(w, r)
			}))
			t.Cleanup(server.Close)
			service := probe.New(store, protectionTarget{endpoint: server.URL}, &protectionTasks{})
			service.UseTaskRunner(&protectionRunner{})
			_, err = service.Enqueue(t.Context(), probe.Request{Automatic: test.automatic}, "test")
			if test.allowed && err != nil {
				t.Fatal(err)
			}
			if !test.allowed && (err == nil || !strings.Contains(err.Error(), "没有符合当前分组、参与范围和探测策略的账号")) {
				t.Fatalf("automatic probe incorrectly included matching group name: %v", err)
			}
		})
	}
}
