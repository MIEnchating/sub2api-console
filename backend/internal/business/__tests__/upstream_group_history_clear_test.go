package business_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func TestClearGroupHistoryForUpstreamKeepsOtherUpstreamsAndCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`INSERT INTO upstream_group_change_events(upstream_id,group_id,group_name,change_type,changed_at) VALUES
 ('one','7','同名组','added','2026-09-28T01:00:00Z'),
 ('one','8','旧组','removed','2026-09-28T02:00:00Z'),
 ('two','7','同名组','added','2026-09-28T03:00:00Z');
 INSERT INTO upstream_groups(host,group_id,name,updated_at) VALUES('one.example','7','同名组','now');`)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", " "} {
		if _, err := store.ClearUpstreamGroupHistoryForUpstream(t.Context(), invalid); err == nil {
			t.Fatal("empty scope must not clear all history")
		}
	}
	deleted, err := store.ClearUpstreamGroupHistoryForUpstream(t.Context(), "one")
	if err != nil || deleted != 2 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	rows, err := store.AllUpstreamGroupHistory(t.Context(), 500)
	if err != nil || len(rows) != 1 || rows[0].UpstreamID != "two" {
		t.Fatalf("remaining=%+v err=%v", rows, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM upstream_groups WHERE host='one.example'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("catalog count=%d err=%v", count, err)
	}
	deleted, err = store.ClearUpstreamGroupHistoryForUpstream(t.Context(), "one")
	if err != nil || deleted != 0 {
		t.Fatalf("repeated delete=%d err=%v", deleted, err)
	}
}
