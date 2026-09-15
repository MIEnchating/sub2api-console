package configstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func localSourceProfileFixture() configstore.WorkbenchSourceProfile {
	return configstore.WorkbenchSourceProfile{ID: "source-profile-1", OwnerHash: strings.Repeat("a", 64), Scope: "local-export", UserID: "user-1", WorkspaceID: "workspace-1", Email: "owner@example.com", HasPassword: true, Login: json.RawMessage(`{"email":"owner@example.com","password":"private-local-password","workspace_id":"workspace-1"}`)}
}

func TestWorkbenchSourceProfileRestartPreservesPrivateLoginAndOwnerScopedMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	value := localSourceProfileFixture()
	if saved, err := store.SaveWorkbenchSourceProfile(context.Background(), value); err != nil || saved.Revision != 1 || len(saved.Login) != 0 {
		t.Fatalf("save = %+v, %v", saved, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	private, err := store.WorkbenchSourceProfile(context.Background(), value.OwnerHash, value.Scope, value.ID)
	if err != nil || !strings.Contains(string(private.Login), "private-local-password") {
		t.Fatal("private credentials were lost across restart")
	}
	list, err := store.WorkbenchSourceProfiles(context.Background(), value.OwnerHash, value.Scope)
	if err != nil || len(list) != 1 || len(list[0].Login) != 0 {
		t.Fatal("metadata list read private credentials")
	}
	raw, _ := json.Marshal([]any{private, list})
	if strings.Contains(string(raw), "private-local-password") {
		t.Fatal("serialized private profile exposed login payload")
	}
	for _, binding := range [][2]string{{strings.Repeat("b", 64), value.Scope}, {value.OwnerHash, "managed"}} {
		other, err := store.WorkbenchSourceProfiles(context.Background(), binding[0], binding[1])
		if err != nil || len(other) != 0 {
			t.Fatal("local profile crossed its owner or scope")
		}
	}
}

func TestWorkbenchSourceProfileCASProtectsConcurrentChangesAndImmutableIdentity(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	value := localSourceProfileFixture()
	if _, err := store.SaveWorkbenchSourceProfile(ctx, value); err != nil {
		t.Fatal(err)
	}
	value.Revision = 1
	changed := value
	changed.UserID = "other-user"
	if _, err := store.SaveWorkbenchSourceProfile(ctx, changed); !errors.Is(err, configstore.ErrWorkbenchSourceProfile) {
		t.Fatal("stable profile rebound to another identity")
	}
	changed = value
	changed.OwnerHash = strings.Repeat("b", 64)
	if _, err := store.SaveWorkbenchSourceProfile(ctx, changed); !errors.Is(err, configstore.ErrWorkbenchSourceProfile) {
		t.Fatal("stable profile rebound to another owner")
	}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		group.Go(func() { _, err := store.SaveWorkbenchSourceProfile(ctx, value); results <- err })
	}
	group.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, configstore.ErrWorkbenchSourceProfile) {
			t.Fatal(err)
		}
	}
	if succeeded != 1 {
		t.Fatal("concurrent profile replacement did not have exactly one winner")
	}
	if err := store.DeleteWorkbenchSourceProfile(ctx, value.OwnerHash, value.Scope, value.ID, 1); !errors.Is(err, configstore.ErrWorkbenchSourceProfile) {
		t.Fatal("stale delete removed the current profile")
	}
	if err := store.DeleteWorkbenchSourceProfile(ctx, value.OwnerHash, value.Scope, value.ID, 2); err != nil {
		t.Fatal(err)
	}
}
