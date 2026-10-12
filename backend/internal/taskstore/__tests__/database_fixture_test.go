package taskstore_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"testing"
)

func contractTaskStore(t *testing.T, config dbconn.Config) *taskstore.Store {
	t.Helper()
	store, err := taskstore.OpenDatabase(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
		db, err := dbconn.Open(context.Background(), config)
		if err != nil {
			t.Error(err)
			return
		}
		defer db.Close()
		for _, table := range []string{"task_recovery", "active_task_operations", "deleted_tasks", "tasks", "task_write_locks"} {
			if table == "task_write_locks" && config.Driver == dbconn.SQLite {
				continue
			}
			if _, err := db.Exec("DELETE FROM " + table); err != nil {
				t.Error(err)
			}
		}
	})
	return store
}
