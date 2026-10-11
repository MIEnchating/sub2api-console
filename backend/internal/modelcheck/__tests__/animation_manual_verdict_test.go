package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func manualVerdictTask(id string) taskstore.Task {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return taskstore.Task{ID: id, Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "succeeded", Message: "动画检测完成", CreatedAt: now, UpdatedAt: now,
		Result: map[string]any{"remote_write": false, "precise_value": json.Number("9007199254740993"), "animations": []any{
			map[string]any{"account_id": "1", "account_name": "同名账号", "model": "test-model", "request_id": "request-1", "status": "succeeded", "svg": fixtureSVG, "completed_at": now},
			map[string]any{"account_id": "2", "account_name": "同名账号", "model": "test-model", "request_id": "request-2", "status": "succeeded", "svg": fixtureSVG, "completed_at": now},
		}}}
}

func manualVerdictFixture(t *testing.T) *fixture {
	t.Helper()
	return setup(t, 2, "openai", func(http.ResponseWriter, *http.Request) { t.Error("manual marking must not call upstream") })
}

func TestAnimationManualVerdictPersistsChangesAndClearWithoutChangingOtherEvidence(t *testing.T) {
	f := manualVerdictFixture(t)
	ctx := context.Background()
	task := manualVerdictTask("manual-verdict")
	if err := f.tasks.Store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	before, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, verdict := range []string{"not_passed", "passed", "unmarked"} {
		request := modelcheck.AnimationManualVerdictRequest{AccountID: "1", RequestID: "request-1", Verdict: verdict}
		if _, err := f.service.SetAnimationManualVerdict(ctx, task.ID, request, "operator"); err != nil {
			t.Fatal(err)
		}
		stored, err := f.tasks.Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		row := stored.Result["animations"].([]any)[0].(map[string]any)
		mark := row["manual_verdict"].(map[string]any)
		if mark["verdict"] != verdict || mark["updated_by"] != "operator" {
			t.Fatalf("wrong mark: %#v", mark)
		}
		if _, err := time.Parse(time.RFC3339Nano, mark["updated_at"].(string)); err != nil {
			t.Fatal(err)
		}
		delete(row, "manual_verdict")
		if !reflect.DeepEqual(stored.Result, before.Result) {
			t.Fatal("mark changed generated content, other accounts, precision or scheduling evidence")
		}
	}
	if len(f.catalog.raw) != 0 || len(f.catalog.plans) != 0 {
		t.Fatal("manual judgment modified automatic detection configuration")
	}
}

func TestAnimationManualVerdictRejectsInvalidOrUnfinishedTargets(t *testing.T) {
	for _, name := range []string{"invalid verdict", "wrong account", "wrong request", "wrong skill", "precheck", "legacy precheck", "duplicate result", "unfinished result", "active", "missing task"} {
		t.Run(name, func(t *testing.T) {
			f := manualVerdictFixture(t)
			ctx := context.Background()
			task := manualVerdictTask("invalid-mark")
			request := modelcheck.AnimationManualVerdictRequest{AccountID: "1", RequestID: "request-1", Verdict: "passed"}
			expected := modelcheck.ErrInvalidAnimationVerdict
			row := task.Result["animations"].([]any)[0].(map[string]any)
			switch name {
			case "invalid verdict":
				request.Verdict = "healthy"
			case "wrong account":
				request.AccountID = "2"
			case "wrong request":
				request.RequestID = "different-generation"
			case "wrong skill":
				task.Skill = "other"
			case "precheck":
				row["mode"] = "precheck"
			case "legacy precheck":
				task.Result["mode"] = "precheck"
			case "duplicate result":
				task.Result["animations"] = []any{row, row}
			case "unfinished result":
				delete(row, "completed_at")
			case "active":
				task.Status = "running"
				expected = taskstore.ErrTaskStillActive
			case "missing task":
				expected = taskstore.ErrNotFound
			}
			if name != "missing task" {
				if err := f.tasks.Store.Save(ctx, task); err != nil {
					t.Fatal(err)
				}
			}
			_, err := f.service.SetAnimationManualVerdict(ctx, task.ID, request, "operator")
			if !errors.Is(err, expected) {
				t.Fatalf("got %v; want %v", err, expected)
			}
			if name != "missing task" {
				stored, err := f.tasks.Get(ctx, task.ID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.Result["animations"].([]any)[0].(map[string]any)["manual_verdict"] != nil {
					t.Fatal("rejected write persisted")
				}
			}
		})
	}
}

func TestAnimationManualVerdictConcurrentAccountsSurviveReopenAndDoNotTransferToRetest(t *testing.T) {
	f := manualVerdictFixture(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "marks.sqlite3")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	service, err := modelcheck.New(store, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	task := manualVerdictTask("concurrent-marks")
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for _, id := range []string{"1", "2"} {
		workers.Go(func() {
			_, err := service.SetAnimationManualVerdict(ctx, task.ID, modelcheck.AnimationManualVerdictRequest{AccountID: id, RequestID: "request-" + id, Verdict: "not_passed"}, "operator")
			if err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range stored.Result["animations"].([]any) {
		if value.(map[string]any)["manual_verdict"].(map[string]any)["verdict"] != "not_passed" {
			t.Fatal("concurrent mark lost after restart")
		}
	}
	retest := manualVerdictTask("retest")
	if err := store.Save(ctx, retest); err != nil {
		t.Fatal(err)
	}
	stored, err = store.Get(ctx, retest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Result["animations"].([]any)[0].(map[string]any)["manual_verdict"] != nil {
		t.Fatal("retest inherited previous mark")
	}
}
