package taskstore_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// Go allocation metrics exclude SQLite's temporary B-trees and allocator.
func BenchmarkHistoryPageLargeResults(b *testing.B) {
	store, err := taskstore.Open(filepath.Join(b.TempDir(), "history.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	payload := strings.Repeat("x", 128<<10)
	for i := range 200 {
		task := taskstore.Task{ID: fmt.Sprintf("task-%03d", i), Skill: "account-workbench", Operation: "import", Status: "succeeded", Progress: 100, Message: "已处理", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"email": "alice@example.com", "detail": payload}}
		if err := store.Save(b.Context(), task); err != nil {
			b.Fatal(err)
		}
	}
	for _, filtered := range []bool{false, true} {
		b.Run(fmt.Sprintf("email=%t", filtered), func(b *testing.B) {
			input := taskstore.HistoryQuery{Offset: 100, Limit: 10}
			if filtered {
				input.Emails = []string{"alice@example.com"}
			}
			b.ReportAllocs()
			for b.Loop() {
				page, err := store.QueryBySkill(b.Context(), "account-workbench", input)
				if err != nil {
					b.Fatal(err)
				}
				if page.Total != 200 || len(page.Items) != 10 || page.Items[0].ID != "task-100" || page.Items[9].ID != "task-109" || page.Items[0].Result["detail"] != payload {
					b.Fatal("page content or total changed")
				}
			}
		})
	}
}
