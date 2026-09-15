package configstore_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestOAuthAutomaticCheckpointRestartRetainsWatchingTransactionWithoutExtendingExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	input := checkpointRecord()
	input.Automatic, input.Status = true, "watching"
	input.WorkerID, input.WorkerLease, input.ParentID = strings.Repeat("a", 48), strings.Repeat("b", 32), strings.Repeat("c", 32)
	saved, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	current, err := store.WorkbenchOAuthCheckpoint(context.Background(), saved.OwnerHash, saved.TargetFingerprint, saved.ID)
	if err != nil || current.Status != "watching" || current.ExpiresAt != saved.ExpiresAt || string(current.Payload) != string(input.Payload) || current.ParentID != input.ParentID {
		t.Fatalf("watching checkpoint after restart = %+v, %v", current, err)
	}
	changed := current
	changed.Status, changed.ParentID = "restoring", strings.Repeat("d", 32)
	if _, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), changed); err == nil {
		t.Fatal("watching transaction could be reassigned to a different parent task")
	}
	current.Status, current.WorkerRevision = "restoring", 4
	claimed, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), current)
	if err != nil || claimed.WorkerRevision != 4 {
		t.Fatalf("versioned watching claim = %+v, %v", claimed, err)
	}
	if _, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), current); err == nil {
		t.Fatal("watching transaction could be claimed twice")
	}
}
