package taskstore_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"testing"
)

func TestDatabaseContractHistoryMatchesExactNestedEmailAndCountsEmptyPages(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			task := taskstore.Task{ID: "email", Skill: "contract", Operation: "import", Status: "succeeded", Message: "完成", CreatedAt: "2026-10-12T12:00:00Z", UpdatedAt: "2026-10-12T12:00:00Z", Result: map[string]any{"items": []any{map[string]any{"email": "User_Name@example.test"}}}}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			input := taskstore.HistoryQuery{Emails: []string{"user_name@example.test"}, Limit: 1}
			page, err := store.QueryBySkill(t.Context(), "contract", input)
			if err != nil || page.Total != 1 || len(page.Items) != 1 {
				t.Fatalf("email match: %+v %v", page, err)
			}
			input.Offset = 2
			page, err = store.QueryBySkill(t.Context(), "contract", input)
			if err != nil || page.Total != 1 || len(page.Items) != 0 {
				t.Fatalf("empty page total: %+v %v", page, err)
			}
			input.Offset = 0
			input.Emails = []string{"userXname@example.test"}
			page, err = store.QueryBySkill(t.Context(), "contract", input)
			if err != nil || page.Total != 0 {
				t.Fatalf("nonliteral email match: %+v %v", page, err)
			}
		})
	}
}
