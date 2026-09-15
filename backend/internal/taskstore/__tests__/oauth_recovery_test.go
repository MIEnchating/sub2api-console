package taskstore_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestInterruptedOAuthBatchPreservesStableRowsForFreshReauthorization(t *testing.T) {
	s, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "batch", Skill: "account-workbench", Operation: "account-workbench-oauth-batch", Status: "waiting_input", Message: "等待授权", CreatedAt: now, UpdatedAt: now, Result: map[string]any{"items": []map[string]any{{"account_id": "101", "user_id": "user-101", "workspace_id": "workspace-101", "profile_id": "profile-101", "status": "running"}}, "phase": "running"}}
	if err := s.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecoverInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := recovered.Result["items"].([]any)
	if !ok || len(rows) != 1 || rows[0].(map[string]any)["account_id"] != "101" {
		t.Fatal("restart discarded stable source needed for fresh authorization")
	}
	if rows[0].(map[string]any)["status"] != "cancelled" {
		t.Fatal("interrupted row still appears running")
	}
	if recovered.Status != "failed" || recovered.Result["interrupted"] != true {
		t.Fatal("interrupted task remained executable")
	}
}
