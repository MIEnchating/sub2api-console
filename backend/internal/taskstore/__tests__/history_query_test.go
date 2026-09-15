package taskstore_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func historyQueryFixture(t *testing.T) *taskstore.Store {
	t.Helper()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for i := range 125 {
		updated := time.Date(2026, 9, 14, 0, 0, i, 0, time.UTC).Format(time.RFC3339Nano)
		task := taskstore.Task{ID: fmt.Sprintf("task-%03d", i), Skill: "account-workbench", Operation: "import", Status: "succeeded", Progress: 100, Message: "账号处理", CreatedAt: updated, UpdatedAt: updated, Result: map[string]any{"items": []any{map[string]any{"email": "alice@example.com"}}}}
		if i < 2 {
			task.Status = "running"
		}
		if i == 3 {
			task.Result = map[string]any{"items": []any{map[string]any{"email": "bob@example.com"}}}
		}
		if i == 4 {
			task.Result = map[string]any{"name": "alice@example.com", "message": "alice@example.com"}
		}
		if i == 5 {
			task.Result = map[string]any{"email": "malice@example.com"}
		}
		if i == 6 {
			task.Skill = "another-module"
			task.Status = "running"
		}
		if err := store.Save(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestHistoryQueryPaginatesPastLatestHundredAndReturnsExactTotal(t *testing.T) {
	store := historyQueryFixture(t)
	page, err := store.QueryBySkill(context.Background(), "account-workbench", taskstore.HistoryQuery{Offset: 100, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 124 || len(page.Items) != 24 || page.Items[23].ID != "task-000" {
		t.Fatalf("page mismatch: %+v", page)
	}
	first, err := store.QueryBySkill(context.Background(), "account-workbench", taskstore.HistoryQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 100 || first.Items[99].ID == page.Items[0].ID {
		t.Fatal("page boundaries repeated records")
	}
}

func TestHistoryEmailQueryMatchesExplicitMetadataAndCombinesStateFilters(t *testing.T) {
	store := historyQueryFixture(t)
	page, err := store.QueryBySkill(context.Background(), "account-workbench", taskstore.HistoryQuery{Emails: []string{" ALICE@example.com ", "alice@example.com", "bob@example.com"}, Status: "succeeded", Operation: "import", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 120 {
		t.Fatalf("exact matching total = %d", page.Total)
	}
	for _, task := range page.Items {
		if task.ID == "task-004" || task.ID == "task-005" || task.ID == "task-006" || task.Status != "succeeded" {
			t.Fatal("unrelated task matched email filter")
		}
	}
	page, err = store.QueryBySkill(context.Background(), "account-workbench", taskstore.HistoryQuery{Emails: []string{"bob@example.com"}, Search: "task-003", Limit: 10})
	if err != nil || page.Total != 1 || page.Items[0].ID != "task-003" {
		t.Fatal("combined search failed", err)
	}
}

func TestHistoryQueryRejectsMalformedEmailsAndTreatsSQLCharactersAsLiteralSearch(t *testing.T) {
	store := historyQueryFixture(t)
	for _, input := range []taskstore.HistoryQuery{{Emails: []string{"Alice <alice@example.com>"}}, {Emails: []string{"alice@example.com\n" + "bob@example.com"}}, {Limit: 101}, {Offset: -1}, {Status: "invalid"}} {
		if _, err := store.QueryBySkill(context.Background(), "account-workbench", input); err == nil {
			t.Fatalf("invalid query accepted: %+v", input)
		}
	}
	page, err := store.QueryBySkill(context.Background(), "account-workbench", taskstore.HistoryQuery{Search: "%' OR 1=1 --"})
	if err != nil || page.Total != 0 {
		t.Fatal("search was interpreted as SQL", err)
	}
}

func TestActiveHistoryIncludesOlderTasksAndExcludesOtherModulesAndPayloads(t *testing.T) {
	store := historyQueryFixture(t)
	tasks, err := store.ActiveBySkill(context.Background(), "account-workbench")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].ID != "task-001" || tasks[1].ID != "task-000" {
		t.Fatalf("incomplete global cancellation scope: %+v", tasks)
	}
	for _, task := range tasks {
		if len(task.Result) != 0 {
			t.Fatal("cancellation scope included result payload")
		}
	}
}
