package taskstore_test

import (
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"testing"
	"time"
)

func TestDatabaseContractCompactionSkipsNonObjectJSONWithoutBlockingOlderObjects(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, item := range []struct{ id, raw string }{{"a-array", "[]"}, {"b-null", "null"}, {"c-number", "42"}, {"d-object", `{"routing":{"account_targets":["41"]}}`}, {"z-recent", "{}"}} {
				_, err := db.ExecContext(t.Context(), `INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at) VALUES(?, 'contract','automatic-inspection','succeeded',100,'完成',?,'2026-10-12T12:00:00Z','2026-10-12T12:00:00Z')`, item.id, item.raw)
				if err != nil {
					t.Fatal(err)
				}
			}
			changed, err := store.CompactAutomaticInspectionHistory(t.Context(), 1)
			if err != nil || changed != 1 {
				t.Fatalf("compaction must only change the old object: %d %v", changed, err)
			}
			var raw string
			if err := db.QueryRowContext(t.Context(), `SELECT result_json FROM tasks WHERE id='b-null'`).Scan(&raw); err != nil || raw != "null" {
				t.Fatalf("non-object modified: %q %v", raw, err)
			}
			changed, err = store.CompactAutomaticInspectionHistory(t.Context(), 1)
			if err != nil || changed != 0 {
				t.Fatalf("compaction must be idempotent: %d %v", changed, err)
			}
		})
	}
}

func TestDatabaseContractHistoryDeletionRequiresVersionAndPreventsResurrection(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			task := taskstore.Task{ID: "delete", Skill: "contract", Operation: "fixture", Status: "succeeded", Message: "完成", Result: map[string]any{}, CreatedAt: "2026-10-12T12:00:00Z", UpdatedAt: "2026-10-12T12:00:00.123456789Z"}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			selected := []taskstore.HistorySelection{{ID: task.ID, UpdatedAt: "2026-10-12T12:00:00.123456788Z"}}
			if err := store.DeleteTerminalBySkill(t.Context(), task.Skill, selected); err == nil {
				t.Fatal("stale delete accepted")
			}
			selected[0].UpdatedAt = "2026-10-12T20:00:00.123456789+08:00"
			if err := store.DeleteTerminalBySkill(t.Context(), task.Skill, selected); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(t.Context(), task); !errors.Is(err, taskstore.ErrTaskDeleted) {
				t.Fatalf("deleted task resurrected: %v", err)
			}
		})
	}
}

func TestDatabaseContractLogCleanupPreservesNanosecondBoundaryAndActiveTasks(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			cutoff := time.Date(2026, 10, 12, 12, 0, 0, 123456789, time.UTC)
			for _, item := range []struct {
				id, status string
				stamp      time.Time
			}{{"old", "succeeded", cutoff.Add(-time.Nanosecond)}, {"boundary", "succeeded", cutoff}, {"active", "running", cutoff.Add(-time.Hour)}} {
				task := taskstore.Task{ID: item.id, Skill: "contract", Operation: "fixture", Status: item.status, Message: "记录", Result: map[string]any{}, CreatedAt: item.stamp.Format(time.RFC3339Nano), UpdatedAt: item.stamp.Format(time.RFC3339Nano)}
				if err := store.Save(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			removed, protected, err := store.ClearLogs(t.Context(), &cutoff)
			if err != nil || removed != 1 || protected != 1 {
				t.Fatalf("cleanup: %d %d %v", removed, protected, err)
			}
			if _, err := store.Get(t.Context(), "boundary"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDatabaseContractCompactionRetainsRecentDetailsAndExactSummaryNumbers(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			for _, id := range []string{"old", "recent"} {
				task := taskstore.Task{ID: id, Skill: "contract", Operation: "automatic-inspection", Status: "succeeded", Message: "完成", Result: map[string]any{"exact": json.Number("1.234567890123456789"), "routing": map[string]any{"account_targets": []any{"41"}, "count": 1}}, CreatedAt: "2026-10-12T12:00:00Z", UpdatedAt: "2026-10-12T12:00:00Z"}
				if err := store.Save(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			changed, err := store.CompactAutomaticInspectionHistory(t.Context(), 1)
			if err != nil || changed != 1 {
				t.Fatalf("compaction: %d %v", changed, err)
			}
			got, err := store.Get(t.Context(), "old")
			if err != nil || got.Result["exact"] != json.Number("1.234567890123456789") || got.Result["compacted"] != true {
				t.Fatalf("compacted precision: %+v %v", got, err)
			}
			if _, found := got.Result["routing"].(map[string]any)["account_targets"]; found {
				t.Fatal("old details retained")
			}
			got, err = store.Get(t.Context(), "recent")
			if err != nil || got.Result["compacted"] != nil {
				t.Fatalf("recent compacted: %+v %v", got, err)
			}
		})
	}
}
