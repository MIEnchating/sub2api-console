package taskstore_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestReopenRemovesPrefixIndexAndKeepsTaskSearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "indexes.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS ix_tasks_log_listing ON tasks(updated_at DESC,id);
		INSERT INTO tasks VALUES('fixture','fixture','fixture','succeeded',100,'needle','{}','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	store, err = taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='ix_tasks_log_listing'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("prefix index still duplicates task search index")
	}
	limit := 10
	rows, err := store.SearchLogs(t.Context(), "needle", &limit)
	if err != nil || len(rows) != 1 || rows[0].ID != "fixture" {
		t.Fatalf("task search changed: %+v %v", rows, err)
	}
}
