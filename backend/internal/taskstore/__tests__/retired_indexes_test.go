package taskstore_test

import (
	"database/sql"
	"path/filepath"
	"strings"
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

func TestOpenAddsCleanupIndexWithoutTemporarySort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup-index.db")
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
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type='index' AND name='ix_tasks_log_cleanup'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("cleanup index missing: count=%d err=%v", count, err)
	}
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT id FROM tasks
		WHERE status NOT IN ('queued','running','waiting_input') AND updated_at < ?
		AND console_history_time(updated_at) < ? AND (updated_at,id) > (?,?)
		ORDER BY updated_at,id LIMIT 2000`, "2026-01-03T00:00:00Z", "2026-01-01T00:00:00.000000000Z",
		"2025-12-01T00:00:00Z", "previous-batch")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	usesCleanupIndex := false
	usesCursorRange := false
	var plan []string
	for rows.Next() {
		var id, parent, auxiliary int
		var detail string
		if err := rows.Scan(&id, &parent, &auxiliary, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
		if strings.Contains(detail, "TEMP B-TREE") {
			t.Fatalf("cleanup query still sorts through a temporary tree: %s", detail)
		}
		usesCleanupIndex = usesCleanupIndex || strings.Contains(detail, "ix_tasks_log_cleanup")
		usesCursorRange = usesCursorRange || strings.Contains(detail, "(updated_at,id)>(?,?)") || strings.Contains(detail, "updated_at>?")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !usesCleanupIndex {
		t.Fatal("cleanup no longer uses the terminal task index")
	}
	if !usesCursorRange {
		t.Fatalf("cleanup does not seek past the previous batch: %v", plan)
	}
}

func TestReopenRepairsCleanupIndexAndPreservesRetainedTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup-upgrade.db")
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
	if _, err := db.Exec(`DROP INDEX ix_tasks_log_cleanup;
		INSERT INTO tasks VALUES('retained','fixture','fixture','running',0,'active','{}','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	store, err = taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name='ix_tasks_log_cleanup'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("cleanup index not repaired: count=%d err=%v", count, err)
	}
	if task, err := store.Get(t.Context(), "retained"); err != nil || task.Status != "running" {
		t.Fatalf("upgrade changed retained task: task=%+v err=%v", task, err)
	}
}
