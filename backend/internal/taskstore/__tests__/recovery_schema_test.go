package taskstore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestOpeningLegacyTaskDatabaseAddsRecoveryWithoutChangingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE tasks(id TEXT PRIMARY KEY,skill TEXT NOT NULL,operation TEXT NOT NULL,status TEXT NOT NULL,progress INTEGER NOT NULL,message TEXT NOT NULL,result_json TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
 INSERT INTO tasks VALUES('legacy','test','account-model-animation','succeeded',100,'检测完成','{"animations":[{"account_id":"41","status":"succeeded"}]}','2026-09-24T00:00:00Z','2026-09-24T00:01:00Z')`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.PendingRecovery(context.Background())
	if err != nil || len(pending) != 0 {
		t.Fatalf("legacy task incorrectly resumed: %v %v", pending, err)
	}
	task, err := store.Get(context.Background(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "succeeded" || len(task.Result["animations"].([]any)) != 1 {
		t.Fatalf("legacy results changed: %#v", task)
	}
}
