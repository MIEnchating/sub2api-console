package taskstore_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestLogCleanupAfterReopenUsesActualLegacyTaskTimes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
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
	t.Cleanup(func() { _ = db.Close() })
	for _, fixture := range []struct{ id, stamp, status string }{
		{"older-utc", "2026-10-01T12:00:00.12Z", "succeeded"},
		{"older-offset", "2026-10-01T20:00:00.12+08:00", "failed"},
		{"equal", "2026-10-01T12:00:00.120000001Z", "partial"},
		{"newer", "2026-10-01T04:00:00.120000002-08:00", "cancelled"},
		{"invalid", "0000-invalid", "succeeded"},
		{"active", "2026-09-01T00:00:00Z", "running"},
	} {
		if _, err := db.Exec(`INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
			VALUES(?,'fixture','fixture',?,0,'fixture','{}',?,?)`, fixture.id, fixture.status, fixture.stamp, fixture.stamp); err != nil {
			t.Fatal(err)
		}
	}
	store, err = taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cutoff := time.Date(2026, 10, 1, 12, 0, 0, 120000001, time.UTC)
	deleted, protected, err := store.ClearLogs(t.Context(), &cutoff)
	if err != nil || deleted != 2 || protected != 1 {
		t.Fatalf("deleted=%d protected=%d error=%v", deleted, protected, err)
	}
	rows, err := db.Query(`SELECT id FROM tasks ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var retained []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		retained = append(retained, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retained, []string{"active", "equal", "invalid", "newer"}) {
		t.Fatalf("retained=%v", retained)
	}
}

func TestLogCleanupRetainsFutureTaskWhenOffsetCrossesMaximumUTCYear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boundary.db")
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
	if _, err := db.Exec(`INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
		VALUES('future','fixture','fixture','succeeded',100,'fixture','{}',
		'9999-12-31T23:30:00-01:00','9999-12-31T23:30:00-01:00')`); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(9999, 12, 31, 23, 0, 0, 0, time.UTC)
	deleted, _, err := store.ClearLogs(t.Context(), &cutoff)
	if err != nil || deleted != 0 {
		t.Fatalf("future task deleted=%d error=%v", deleted, err)
	}
	var retained int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks WHERE id='future'").Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("future task retained=%d error=%v", retained, err)
	}
}

func TestLogCleanupAtMaximumYearDeletesOnlyValidEarlierInstants(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boundary-retention.db")
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
	for _, fixture := range []struct{ id, stamp string }{
		{"earlier", "9999-12-31T22:59:59.999999999Z"},
		{"equal", "9999-12-31T22:00:00-01:00"},
		{"invalid", "0000-invalid"},
	} {
		if _, err := db.Exec(`INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
			VALUES(?,'fixture','fixture','succeeded',100,'fixture','{}',?,?)`, fixture.id, fixture.stamp, fixture.stamp); err != nil {
			t.Fatal(err)
		}
	}
	cutoff := time.Date(9999, 12, 31, 23, 0, 0, 0, time.UTC)
	deleted, _, err := store.ClearLogs(t.Context(), &cutoff)
	if err != nil || deleted != 1 {
		t.Fatalf("earlier records deleted=%d error=%v", deleted, err)
	}
	rows, err := db.Query("SELECT id FROM tasks ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var retained []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		retained = append(retained, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retained, []string{"equal", "invalid"}) {
		t.Fatalf("retained=%v", retained)
	}
}

func TestLogCleanupRetentionAcrossBatchesPreservesBoundaryAndActiveTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "batched-retention.db")
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
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2001)
		INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
		SELECT printf('expired-%04d',x),'fixture','fixture','succeeded',100,'fixture','{}',
		'2026-10-01T12:00:00Z',CASE WHEN x%2=0 THEN '2026-10-01T20:00:00.12+08:00'
		ELSE '2026-10-01T12:00:00.120000000Z' END FROM n`); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ id, stamp, status string }{
		{"equal", "2026-10-01T04:00:00.120000001-08:00", "succeeded"},
		{"newer", "2026-10-01T20:00:00.120000002+08:00", "failed"},
		{"invalid", "0000-invalid", "succeeded"},
		{"active", "2026-09-01T00:00:00Z", "running"},
	} {
		if _, err := db.Exec(`INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
			VALUES(?,'fixture','fixture',?,0,'fixture','{}',?,?)`, fixture.id, fixture.status, fixture.stamp, fixture.stamp); err != nil {
			t.Fatal(err)
		}
	}
	cutoff := time.Date(2026, 10, 1, 12, 0, 0, 120000001, time.UTC)
	var progress []int64
	deleted, protected, err := store.ClearLogsWithProgress(t.Context(), &cutoff, func(count, _ int64) error {
		progress = append(progress, count)
		return nil
	})
	if err != nil || deleted != 2001 || protected != 1 || !reflect.DeepEqual(progress, []int64{2000, 2001}) {
		t.Fatalf("deleted=%d protected=%d progress=%v error=%v", deleted, protected, progress, err)
	}
	rows, err := db.Query("SELECT id FROM tasks ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var retained []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		retained = append(retained, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retained, []string{"active", "equal", "invalid", "newer"}) {
		t.Fatalf("retained=%v", retained)
	}
}

func TestLogCleanupRetentionDefersEarlierBackfillUntilNextRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backfilled-retention.db")
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
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2001)
		INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
		SELECT printf('expired-%04d',x),'fixture','fixture','succeeded',100,'fixture','{}',
		'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z' FROM n`); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	deleted, _, err := store.ClearLogsWithProgress(t.Context(), &cutoff, func(count, _ int64) error {
		if count != 2000 {
			return nil
		}
		return store.Save(t.Context(), taskstore.Task{
			ID: "backfilled", Skill: "fixture", Operation: "fixture", Status: "succeeded", Message: "fixture", Result: map[string]any{},
			CreatedAt: "2025-01-01T00:00:00Z", UpdatedAt: "2025-01-01T00:00:00Z",
		})
	})
	if err != nil || deleted != 2001 {
		t.Fatalf("cleanup deleted=%d error=%v", deleted, err)
	}
	if _, err := store.Get(t.Context(), "backfilled"); err != nil {
		t.Fatalf("concurrent earlier backfill was not retained: %v", err)
	}
	deleted, _, err = store.ClearLogs(t.Context(), &cutoff)
	if err != nil || deleted != 1 {
		t.Fatalf("next cleanup deleted=%d error=%v", deleted, err)
	}
}
