package taskstore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestHistoryEmailQuerySkipsCorruptResultsAndPreservesMatchingRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	task := taskstore.Task{
		ID: "matching", Skill: "account-workbench", Operation: "import", Status: "succeeded",
		Progress: 100, Message: "完成", CreatedAt: "2026-09-14T12:00:00Z", UpdatedAt: "2026-09-14T12:00:00Z",
		Result: map[string]any{"email": "alice@example.com"},
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO tasks VALUES('corrupt','account-workbench','import','succeeded',100,'完成','{broken','2026-09-14T12:00:00.000000000Z','2026-09-14T12:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}

	page, err := store.QueryBySkill(ctx, task.Skill, taskstore.HistoryQuery{Emails: []string{"alice@example.com"}})
	if err != nil {
		t.Fatalf("corrupt unrelated history prevented email search: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != task.ID {
		t.Fatalf("email search lost its matching record: %#v", page)
	}
}
