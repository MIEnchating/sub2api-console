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

func profileFixture() configstore.WorkbenchLoginProfile {
	return configstore.WorkbenchLoginProfile{ID: "profile-1", TargetURL: "https://target.example", TargetFingerprint: strings.Repeat("a", 64), AccountID: "101", UserID: "user-101", WorkspaceID: "workspace-101", Email: "owner@example.com", HasPassword: true, Login: json.RawMessage(`{"email":"owner@example.com","password":"private-login-password","workspace_id":"workspace-101"}`)}
}

func TestWorkbenchLoginProfileRestartPreservesPrivateCredentialsAndListSelectsOnlyMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	value := profileFixture()
	saved, err := store.SaveWorkbenchLoginProfile(context.Background(), value)
	if err != nil || saved.Revision != 1 || len(saved.Login) != 0 {
		t.Fatalf("save result not metadata: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	private, err := store.WorkbenchLoginProfile(context.Background(), value.TargetFingerprint, value.ID)
	if err != nil || !strings.Contains(string(private.Login), "private-login-password") {
		t.Fatal("private profile did not survive restart")
	}
	raw, _ := json.Marshal(private)
	if strings.Contains(string(raw), "private-login-password") {
		t.Fatal("private record JSON exposed login")
	}
	list, err := store.WorkbenchLoginProfiles(context.Background(), value.TargetFingerprint)
	if err != nil || len(list) != 1 || len(list[0].Login) != 0 {
		t.Fatal("list read credentials")
	}
	other, err := store.WorkbenchLoginProfiles(context.Background(), strings.Repeat("b", 64))
	if err != nil || len(other) != 0 {
		t.Fatal("profiles leaked across target identities")
	}
}

func TestWorkbenchLoginProfileCASRejectsConcurrentUpdatesStaleDeleteAndIdentityRebinding(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	value := profileFixture()
	if _, err := store.SaveWorkbenchLoginProfile(ctx, value); err != nil {
		t.Fatal(err)
	}
	value.Revision = 1
	changed := value
	changed.UserID = "other-user"
	if _, err := store.SaveWorkbenchLoginProfile(ctx, changed); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatal("profile ID rebound to different identity")
	}
	duplicate := value
	duplicate.ID, duplicate.Revision = "profile-2", 0
	if _, err := store.SaveWorkbenchLoginProfile(ctx, duplicate); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatal("second profile claimed same target account")
	}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		group.Go(func() { _, err := store.SaveWorkbenchLoginProfile(ctx, value); results <- err })
	}
	group.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("CAS success=%d conflict=%d", success, conflict)
	}
	if err := store.DeleteWorkbenchLoginProfile(ctx, value.TargetFingerprint, value.ID, 1); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatal("stale delete removed newer record")
	}
	if err := store.DeleteWorkbenchLoginProfile(ctx, value.TargetFingerprint, value.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWorkbenchLoginProfile(ctx, value); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatal("stale update recreated deleted credentials")
	}
}
