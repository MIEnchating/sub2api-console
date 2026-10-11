package taskstore_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// The allocation metrics cover Go allocations, not SQLite scratch space or RSS.
func BenchmarkHistoryPayloads(b *testing.B) {
	for _, kind := range []string{"animation", "model-check"} {
		b.Run(kind, func(b *testing.B) {
			store, err := taskstore.Open(filepath.Join(b.TempDir(), "history.db"))
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			payload := strings.Repeat("x", 32<<10)
			for account := range 30 {
				for attempt := range 10 {
					stamp := fmt.Sprintf("2026-10-08T00:%02d:00.123456789Z", attempt)
					task := historyTimeTask(fmt.Sprintf("task-%03d-%02d", account, attempt), fmt.Sprint(account), stamp, stamp)
					if kind == "animation" {
						task.Result["animations"].([]any)[0].(map[string]any)["html"] = payload
					} else {
						task.Skill = "sub2api-model-check"
						task.Operation = "account-model-behavior-check"
						task.Result = map[string]any{
							"account_ids": []string{fmt.Sprint(account)},
							"diagnostic":  payload,
							"tests": []any{map[string]any{
								"account_id": fmt.Sprint(account), "claimed_model": "fixture-model", "verdict": "MATCH",
								"requests": map[string]int{"total": 2, "successful": 2}, "response": payload,
							}},
						}
					}
					if err := store.Save(b.Context(), task); err != nil {
						b.Fatal(err)
					}
				}
			}
			read := func() {
				var tasks []taskstore.Task
				var err error
				if kind == "animation" {
					tasks, err = store.AnimationHistory(b.Context())
				} else {
					tasks, err = store.ModelCheckHistory(b.Context(), time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
				}
				if err != nil || len(tasks) != 30 {
					b.Fatalf("history size=%d error=%v", len(tasks), err)
				}
				for _, task := range tasks {
					if kind == "animation" {
						if len(task.Result) != 0 {
							b.Fatal("animation payload leaked into history")
						}
					} else {
						tests, ok := task.Result["tests"].([]any)
						if !ok || len(tests) != 1 || tests[0].(map[string]any)["response"] != nil || task.Result["diagnostic"] != nil {
							b.Fatal("model-check public projection changed")
						}
					}
				}
			}
			read()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				read()
			}
		})
	}
}
