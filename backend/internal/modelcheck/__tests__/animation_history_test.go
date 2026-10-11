package modelcheck_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func animationHistoryTask(id, accountID, mode string, minute int) taskstore.Task {
	at := time.Date(2026, 10, 8, 0, minute, 0, 0, time.UTC).Format(time.RFC3339Nano)
	operation := "account-model-animation"
	row := map[string]any{"account_id": accountID, "account_name": "测试账号", "model": "test-model", "mode": mode, "request_id": id + "-request", "status": "succeeded", "completed_at": at, "duration_ms": 1}
	if mode == "precheck" {
		operation = "account-model-precheck"
		row["precheck"] = map[string]any{"verdict": "passed", "profile_version": "candy-v1", "questions": []any{}}
	} else {
		row["svg"] = fixtureSVG
	}
	return taskstore.Task{ID: id, Skill: "sub2api-model-animation", Operation: operation, Status: "succeeded", Progress: 100, Message: "检测完成", CreatedAt: at, UpdatedAt: at,
		Result: map[string]any{"account_ids": []string{accountID}, "animations": []any{row}}}
}

func TestAnimationHistoryRetainsEachAccountStageBeyondTwentyNewTasks(t *testing.T) {
	f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) { t.Error("history must not call upstream") })
	ctx := context.Background()
	for _, task := range []taskstore.Task{
		animationHistoryTask("account-1-animation", "1", "animation", 0),
		animationHistoryTask("account-1-precheck", "1", "precheck", 1),
	} {
		if err := f.tasks.Store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 25 {
		if err := f.tasks.Store.Save(ctx, animationHistoryTask(fmt.Sprintf("account-2-%02d", i), "2", "animation", i+2)); err != nil {
			t.Fatal(err)
		}
	}
	history, err := f.service.AnimationHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, task := range history {
		ids[task.ID] = true
		if len(task.Result) != 0 {
			t.Fatal("history list loaded full animation payloads")
		}
	}
	for _, id := range []string{"account-1-animation", "account-1-precheck", "account-2-24"} {
		if !ids[id] {
			t.Fatalf("account result disappeared from history after unrelated detections: %s", id)
		}
	}
	if ids["account-2-00"] {
		t.Fatal("superseded history unnecessarily loaded")
	}
	stored, err := f.tasks.Get(ctx, "account-1-animation")
	if err != nil || stored.Result["animations"].([]any)[0].(map[string]any)["svg"] != fixtureSVG {
		t.Fatal("history lookup changed persisted animation")
	}
}

func TestAnimationHistoryRetainsStageResultsAndAttemptsAfterUnrelatedDetections(t *testing.T) {
	cases := []struct {
		name    string
		prepare func() []taskstore.Task
		want    []string
		omit    []string
	}{
		{
			name: "active task and previous animation remain visible",
			prepare: func() []taskstore.Task {
				previous := animationHistoryTask("previous", "1", "animation", 0)
				active := animationHistoryTask("active", "1", "animation", 1)
				active.Status = "running"
				active.Result["animations"].([]any)[0].(map[string]any)["phase"] = "generating"
				return []taskstore.Task{previous, active}
			},
			want: []string{"previous", "active"},
		},
		{
			name: "cancelled precheck attempt without rows remains visible",
			prepare: func() []taskstore.Task {
				previous := animationHistoryTask("previous", "1", "precheck", 0)
				cancelled := animationHistoryTask("cancelled", "1", "precheck", 1)
				cancelled.Status = "cancelled"
				cancelled.Result = map[string]any{"targets": []any{map[string]any{"account_id": "1"}}}
				return []taskstore.Task{previous, cancelled}
			},
			want: []string{"previous", "cancelled"},
		},
		{
			name: "managed precheck only does not supersede animation attempt",
			prepare: func() []taskstore.Task {
				animation := animationHistoryTask("animation-attempt", "1", "animation", 0)
				animation.Status = "failed"
				delete(animation.Result, "animations")
				precheck := animationHistoryTask("precheck", "1", "precheck", 1)
				precheck.Operation = "managed-model-detection"
				precheck.Result["configuration"] = map[string]any{"animation": false, "precheck": true}
				return []taskstore.Task{animation, precheck}
			},
			want: []string{"animation-attempt", "precheck"},
		},
		{
			name: "completion time determines latest result despite later manual update",
			prepare: func() []taskstore.Task {
				latest := animationHistoryTask("latest-result", "1", "animation", 0)
				latest.Result["animations"].([]any)[0].(map[string]any)["completed_at"] = "2026-10-08T00:05:00Z"
				older := animationHistoryTask("older-result", "1", "animation", 1)
				older.UpdatedAt = "2026-10-08T00:06:00Z"
				attempt := animationHistoryTask("latest-attempt", "1", "animation", 2)
				attempt.Status = "cancelled"
				delete(attempt.Result, "animations")
				return []taskstore.Task{latest, older, attempt}
			},
			want: []string{"latest-result", "latest-attempt"},
			omit: []string{"older-result"},
		},
		{
			name: "legacy managed animation without mode retains terminal streaming phase",
			prepare: func() []taskstore.Task {
				legacy := animationHistoryTask("legacy", "1", "animation", 0)
				legacy.Operation = "managed-model-detection"
				legacy.Result["configuration"] = map[string]any{"precheck": false}
				row := legacy.Result["animations"].([]any)[0].(map[string]any)
				delete(row, "mode")
				row["phase"] = "running"
				attempt := animationHistoryTask("latest-attempt", "1", "animation", 1)
				attempt.Status = "failed"
				delete(attempt.Result, "animations")
				return []taskstore.Task{legacy, attempt}
			},
			want: []string{"legacy", "latest-attempt"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) { t.Error("history must not call upstream") })
			ctx := context.Background()
			tasks := tc.prepare()
			for i := range 25 {
				tasks = append(tasks, animationHistoryTask(fmt.Sprintf("unrelated-%02d", i), "2", "animation", i+10))
			}
			for _, task := range tasks {
				if err := f.tasks.Store.Save(ctx, task); err != nil {
					t.Fatal(err)
				}
			}
			history, err := f.service.AnimationHistory(ctx)
			if err != nil {
				t.Fatal(err)
			}
			ids := map[string]bool{}
			for _, task := range history {
				ids[task.ID] = true
			}
			for _, id := range tc.want {
				if !ids[id] {
					t.Errorf("missing retained task %s", id)
				}
			}
			for _, id := range tc.omit {
				if ids[id] {
					t.Errorf("unexpected superseded task %s", id)
				}
			}
		})
	}
}
