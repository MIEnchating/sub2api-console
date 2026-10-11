package taskstore_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func legacyHistoryFixture(t *testing.T, stamp string) (*taskstore.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.db")
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
	if _, err := db.Exec(`INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at)
		VALUES('selected','account-workbench','account-workbench-import','succeeded',100,'已完成','{}',?,?),
		('retained','account-workbench','account-workbench-import','failed',100,'已失败','{}',?,?)`, stamp, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	store, err = taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, db
}

func TestHistoryDeleteAfterReopenAcceptsEquivalentLegacyVersions(t *testing.T) {
	for _, scenario := range []struct{ name, stored, selected string }{
		{"whole-second", "2026-10-01T12:00:00Z", "2026-10-01T12:00:00Z"},
		{"fraction", "2026-10-01T12:00:00.12Z", "2026-10-01T12:00:00.120000000Z"},
		{"positive-offset", "2026-10-01T20:00:00.120000001+08:00", "2026-10-01T12:00:00.120000001Z"},
		{"negative-offset", "2026-10-01T04:00:00.12-08:00", "2026-10-01T12:00:00.12Z"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, _ := legacyHistoryFixture(t, scenario.stored)
			selected, err := store.Get(t.Context(), "selected")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.DeleteTerminalBySkill(t.Context(), selected.Skill, []taskstore.HistorySelection{{ID: selected.ID, UpdatedAt: scenario.selected}}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Get(t.Context(), "selected"); !errors.Is(err, taskstore.ErrNotFound) {
				t.Fatalf("deleted task: %v", err)
			}
			if _, err := store.Get(t.Context(), "retained"); err != nil {
				t.Fatalf("unselected task removed: %v", err)
			}
			if err := store.Save(t.Context(), selected); !errors.Is(err, taskstore.ErrTaskDeleted) {
				t.Fatalf("deleted legacy task resurrected: %v", err)
			}
		})
	}
}

func TestHistoryDeleteRejectsNanosecondChangedLegacyVersionAndRollsBackBatch(t *testing.T) {
	store, db := legacyHistoryFixture(t, "2026-10-01T20:00:00.120000001+08:00")
	if err := store.DeleteTerminalBySkill(t.Context(), "account-workbench", []taskstore.HistorySelection{
		{ID: "selected", UpdatedAt: "2026-10-01T12:00:00.120000001Z"},
		{ID: "retained", UpdatedAt: "2026-10-01T12:00:00.12Z"},
	}); err == nil {
		t.Fatal("nanosecond-stale confirmation deleted history")
	}
	var tasks, tombstones int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM tasks),(SELECT count(*) FROM deleted_tasks)`).Scan(&tasks, &tombstones); err != nil {
		t.Fatal(err)
	}
	if tasks != 2 || tombstones != 0 {
		t.Fatalf("partial deletion: tasks=%d tombstones=%d", tasks, tombstones)
	}
}

func TestHistoryDeleteCannotConfirmInvalidStoredTimestamp(t *testing.T) {
	store, db := legacyHistoryFixture(t, "2026-10-01T12:00:00Z")
	if _, err := db.Exec(`UPDATE tasks SET updated_at='invalid' WHERE id='selected'`); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTerminalBySkill(t.Context(), "account-workbench", []taskstore.HistorySelection{{ID: "selected", UpdatedAt: "2026-10-01T12:00:00Z"}}); err == nil {
		t.Fatal("invalid stored version accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("tasks=%d err=%v", count, err)
	}
}
