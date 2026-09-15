package configstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestOAuthCheckpointLocalScopePersistsWithoutTargetAndCannotChangeScope(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	input := checkpointRecord()
	input.Scope, input.TargetURL = "local-export", ""
	saved, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), input)
	if err != nil || saved.Scope != "local-export" || saved.TargetURL != "" {
		t.Fatalf("local checkpoint save = %+v, %v", saved, err)
	}
	stored, err := store.WorkbenchOAuthCheckpoint(context.Background(), input.OwnerHash, input.TargetFingerprint, input.ID)
	if err != nil || stored.Scope != "local-export" || stored.TargetURL != "" {
		t.Fatalf("local checkpoint read = %+v, %v", stored, err)
	}
	stored.Status, stored.Payload = "failed", nil
	stored.Scope, stored.TargetURL = "managed", "https://changed.example"
	if _, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), stored); err == nil {
		t.Fatal("persisted local checkpoint could be rebound to management scope")
	}
}

func TestOAuthCheckpointRejectsAmbiguousScopeAndTarget(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, pair := range [][2]string{{"managed", ""}, {"local-export", "https://target.example"}, {"other", ""}} {
		input := checkpointRecord()
		input.Scope, input.TargetURL = pair[0], pair[1]
		if _, err := store.SaveWorkbenchOAuthCheckpoint(context.Background(), input); err == nil {
			t.Fatalf("checkpoint accepted ambiguous scope %q and target %q", input.Scope, input.TargetURL)
		}
	}
}
