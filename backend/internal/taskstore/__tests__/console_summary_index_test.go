package taskstore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestConsoleSummaryOldDatabaseAddsIndexWithoutChangingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY,skill TEXT NOT NULL,operation TEXT NOT NULL,status TEXT NOT NULL,progress INTEGER NOT NULL,
		message TEXT NOT NULL,result_json TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL
	);
	INSERT INTO tasks VALUES('existing','console','export','succeeded',100,'历史导出',
		'{"system_info":true,"object_label":"导出账号","run_key":"existing-run","items":["保留历史内容"]}',
		'2026-10-01T00:00:00.000000000Z','2026-10-01T00:00:00.000000000Z');`); err != nil {
		t.Fatal(err)
	}
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ix_tasks_console_summary'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("old database lacks console summary index: count=%d, err=%v", count, err)
	}
	rows, err := store.ListConsoleSummaries(context.Background(), nil)
	if err != nil || len(rows) != 1 || rows[0].ID != "existing" || rows[0].Result["run_key"] != "existing-run" || rows[0].Result["object_label"] != "导出账号" {
		t.Fatalf("old history summary changed: %#v, %v", rows, err)
	}
	full, err := store.Get(context.Background(), "existing")
	if err != nil || len(full.Result["items"].([]any)) != 1 || full.Result["items"].([]any)[0] != "保留历史内容" {
		t.Fatalf("old full result changed: %#v, %v", full, err)
	}
}

func TestConsoleSummaryUpdatesVisibilityAndKeepsUnrelatedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at) VALUES
		('explicit','console','export','running',0,'导出中','{"system_info":true}','now','2026-10-01T00:00:00.000000000Z'),
		('probe','console','active-probe','running',0,'探活中','{"platform":"openai","model":"model-a"}','now','2026-10-02T00:00:00.000000000Z'),
		('animation','console','account-model-animation','succeeded',100,'检测完成','broken JSON','now','2026-10-03T00:00:00.000000000Z'),
		('invalid-probe','console','active-probe','failed',0,'探活记录损坏','broken JSON','now','2026-10-04T00:00:00.000000000Z'),
		('hidden','console','automatic-inspection','succeeded',100,'巡检完成','{}','now','2026-10-04T00:00:00.000000000Z');`); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListConsoleSummaries(context.Background(), nil)
	if err != nil || len(rows) != 3 || rows[0].ID != "animation" || rows[1].ID != "probe" || rows[2].ID != "explicit" {
		t.Fatalf("initial visibility changed: %#v, %v", rows, err)
	}
	if _, err := db.Exec(`UPDATE tasks SET result_json='{}' WHERE id IN ('explicit','probe');
		UPDATE tasks SET result_json='{"system_info":true}' WHERE id='hidden'`); err != nil {
		t.Fatal(err)
	}
	rows, err = store.ListConsoleSummaries(context.Background(), nil)
	if err != nil || len(rows) != 2 || rows[0].ID != "hidden" || rows[1].ID != "animation" {
		t.Fatalf("visibility update not reflected: %#v, %v", rows, err)
	}
	logs, err := store.ListLogSummaries(context.Background(), nil)
	if err != nil || len(logs) != 5 {
		t.Fatalf("visibility filtering removed history: count=%d, %v", len(logs), err)
	}
}
