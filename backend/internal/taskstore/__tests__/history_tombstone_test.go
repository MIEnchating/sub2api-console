package taskstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDeletedHistoryRejectsLateSameStatusSaveBeforeAndAfterRestart(t *testing.T) {
	for _, restart := range []bool{false, true} {
		name := "same-process"
		if restart {
			name = "after-restart"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "tasks.db")
			store, err := taskstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			now := time.Now().UTC().Format(time.RFC3339Nano)
			task := taskstore.Task{ID: "completed-oauth", Skill: "account-workbench", Operation: "account-workbench-oauth", Status: "succeeded", Message: "授权完成", CreatedAt: now, UpdatedAt: now, Result: map[string]any{"phase": "authorized"}}
			if err := store.Save(ctx, task); err != nil {
				t.Fatal(err)
			}
			if err := store.DeleteTerminalBySkill(ctx, task.Skill, []taskstore.HistorySelection{{ID: task.ID, UpdatedAt: task.UpdatedAt}}); err != nil {
				t.Fatal(err)
			}
			if restart {
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				store, err = taskstore.Open(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			task.Message = "短信订单结束状态未确认"
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			task.Result["sms_order_status"] = "review"
			if err := store.Save(ctx, task); !errors.Is(err, taskstore.ErrTaskDeleted) {
				t.Fatalf("late SMS-result save was not rejected as deleted: %v", err)
			}
			if _, err := store.Get(ctx, task.ID); !errors.Is(err, taskstore.ErrNotFound) {
				t.Fatalf("deleted history became visible: %v", err)
			}
		})
	}
}

func TestRejectedHistoryDeletionDoesNotBlockSubsequentUpdatesToItsValidSelection(t *testing.T) {
	ctx := context.Background()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "completed", Skill: "account-workbench", Operation: "account-workbench-oauth", Status: "succeeded", Message: "授权完成", CreatedAt: now, UpdatedAt: now, Result: map[string]any{}}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTerminalBySkill(ctx, task.Skill, []taskstore.HistorySelection{{ID: task.ID, UpdatedAt: now}, {ID: "missing", UpdatedAt: now}}); err == nil {
		t.Fatal("invalid deletion batch succeeded")
	}
	task.Message = "订单核对完成"
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("rolled-back deletion blocked normal finalization: %v", err)
	}
	stored, err := store.Get(ctx, task.ID)
	if err != nil || stored.Message != task.Message {
		t.Fatal("valid history update was discarded after rejected deletion")
	}
}

func TestDeletedStableTaskIDCannotReserveAnOperationAndOtherIDsRemainWritable(t *testing.T) {
	ctx := context.Background()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "deleted", Skill: "account-workbench", Operation: "account-workbench-oauth", Status: "cancelled", Message: "已取消", CreatedAt: now, UpdatedAt: now, Result: map[string]any{}}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTerminalBySkill(ctx, task.Skill, []taskstore.HistorySelection{{ID: task.ID, UpdatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	task.Skill, task.Operation, task.Status = "another-module", "account-rate-sync", "running"
	if err := store.Save(ctx, task); !errors.Is(err, taskstore.ErrTaskDeleted) {
		t.Fatalf("deleted stable ID was not rejected under a different operation: %v", err)
	}
	task.ID = "unrelated-active-task"
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("rejected resurrection reserved an unrelated operation: %v", err)
	}
	task.Status = "cancelled"
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("unrelated running task could not be cancelled: %v", err)
	}
}
