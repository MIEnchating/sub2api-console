package taskrecovery_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestRecoveryDispatchReadsLatestProgressAndRecoveryParameters(t *testing.T) {
	store, pending := pendingTask(t)
	ctx := context.Background()
	current, err := store.Get(ctx, pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Result["completed"] = 2
	if err := taskstore.WithRecovery(&current, []string{"42"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, current); err != nil {
		t.Fatal(err)
	}
	var received *taskstore.Task
	taskrecovery.Run(ctx, store, pending, map[string]taskrecovery.Handler{"test-recovery": func(_ context.Context, task taskstore.Task) error {
		received = &task
		return nil
	}})
	if received == nil || received.Result["completed"] != json.Number("2") {
		t.Fatal("recovery dispatched an outdated result")
	}
	var scope []string
	if err := taskstore.RecoveryInput(*received, &scope); err != nil || len(scope) != 1 || scope[0] != "42" {
		t.Fatalf("recovery dispatched outdated scope: %#v, %v", scope, err)
	}
}
