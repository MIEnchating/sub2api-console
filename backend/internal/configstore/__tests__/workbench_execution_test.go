package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestWorkbenchExecutionCompetingClaimPreservesFirstRetryTask(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	input := configstore.WorkbenchExecution{ID: "source-task", TargetURL: "https://isolated.invalid", TargetFingerprint: "test-fingerprint", Items: []configstore.WorkbenchExecutionItem{{Index: 0, Credentials: json.RawMessage(`{"access_token":"private-test-token"}`), Phase: "prepared", Status: "failed"}}}
	if err := store.SaveWorkbenchExecution(ctx, input); err != nil {
		t.Fatal(err)
	}
	first, err := store.WorkbenchExecution(ctx, input.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.WorkbenchExecution(ctx, input.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first-retry", "second-retry"} {
		child := input
		child.ID, child.SourceID = id, input.ID
		if err := store.SaveWorkbenchExecution(ctx, child); err != nil {
			t.Fatal(err)
		}
	}
	first.Items[0].RetryTaskID = "first-retry"
	if err := store.SaveWorkbenchExecution(ctx, first); err != nil {
		t.Fatal(err)
	}
	second.Items[0].RetryTaskID = "second-retry"
	if err := store.SaveWorkbenchExecution(ctx, second); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatalf("stale claim unexpectedly overwrote execution: %v", err)
	}
	stored, err := store.WorkbenchExecution(ctx, input.ID)
	if err != nil || stored.Items[0].RetryTaskID != "first-retry" || stored.Revision != 2 {
		t.Fatalf("stored execution = %+v, %v", stored, err)
	}
	for _, id := range []string{"first-retry", "second-retry"} {
		child, err := store.WorkbenchExecution(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		err = store.ValidateWorkbenchExecutionClaim(ctx, child)
		if (id == "first-retry" && err != nil) || (id == "second-retry" && !errors.Is(err, configstore.ErrWorkbenchExecution)) {
			t.Fatalf("retry claim authorization = %s, %v", id, err)
		}
	}
}

func TestWorkbenchExecutionClearedCredentialRemainsClearedAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	input := configstore.WorkbenchExecution{ID: "source-task", TargetURL: "https://isolated.invalid", TargetFingerprint: "test-fingerprint", Items: []configstore.WorkbenchExecutionItem{{Index: 0, Credentials: json.RawMessage(`{"access_token":"private-test-token"}`), Phase: "prepared", Status: "failed"}}}
	if err := store.SaveWorkbenchExecution(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.Revision = 1
	input.Items[0].Credentials = nil
	input.Items[0].AccountID, input.Items[0].Phase = "101", "staged"
	if err := store.SaveWorkbenchExecution(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	stored, err := store.WorkbenchExecution(ctx, input.ID)
	if err != nil || len(stored.Items[0].Credentials) != 0 || stored.Items[0].AccountID != "101" {
		t.Fatalf("reopened execution = %+v, %v", stored, err)
	}
}
