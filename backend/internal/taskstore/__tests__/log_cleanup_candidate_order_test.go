package taskstore_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestLogCleanupWithReversedIDsContinuesInTimestampOrderAcrossBatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// IDs decrease as dates increase; equal dates span the first batch edge.
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<4001)
		INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
		SELECT printf('expired-%04d',5000-x),'fixture','fixture','succeeded',100,'fixture','{}',
		'2026-01-01T00:00:00Z',CASE WHEN x<=1999 THEN '2026-01-01T00:00:00Z'
		WHEN x<=2002 THEN '2026-01-02T00:00:00Z' ELSE '2026-01-03T00:00:00Z' END FROM n`); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var reports []int64
	deleted, protected, err := store.ClearLogsWithProgress(t.Context(), &cutoff, func(count, _ int64) error {
		reports = append(reports, count)
		return nil
	})
	if err != nil || deleted != 4001 || protected != 0 || !reflect.DeepEqual(reports, []int64{2000, 4000, 4001}) {
		t.Fatalf("deleted=%d protected=%d reports=%v error=%v", deleted, protected, reports, err)
	}
	var remaining, tombstones int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("remaining=%d error=%v", remaining, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM deleted_tasks").Scan(&tombstones); err != nil || tombstones != 4001 {
		t.Fatalf("tombstones=%d error=%v", tombstones, err)
	}
}
