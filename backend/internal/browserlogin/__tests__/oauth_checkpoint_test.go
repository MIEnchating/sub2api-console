package browserlogin_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func checkpointOptions() browserlogin.OAuthOptions {
	options := validOAuthOptions("checkpoint-state")
	options.Recovery = &browserlogin.OAuthRecoveryBinding{Owner: strings.Repeat("a", 64), Lease: strings.Repeat("b", 64), ExpiresAt: time.Now().Add(10 * time.Minute).UTC()}
	return options
}

func TestOAuthCheckpointPruneRemovesExpiredAndMalformedStateButKeepsUnexpiredSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	expiredID, activeID, invalidID := strings.Repeat("a", 48), strings.Repeat("b", 48), strings.Repeat("c", 48)
	for _, fixture := range []struct {
		id     string
		expiry time.Time
	}{{expiredID, time.Now().Add(-time.Minute)}, {activeID, time.Now().Add(time.Minute)}} {
		raw, err := json.Marshal(struct {
			Version int                          `json:"version"`
			Meta    browserlogin.OAuthCheckpoint `json:"meta"`
		}{Version: 1, Meta: browserlogin.OAuthCheckpoint{ID: fixture.id, ExpiresAt: fixture.expiry}})
		if err != nil || os.WriteFile(filepath.Join(root, fixture.id+".json"), raw, 0600) != nil {
			t.Fatal("private fixture setup failed")
		}
	}
	if err := os.WriteFile(filepath.Join(root, invalidID+".json"), []byte("incomplete checkpoint"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (browserlogin.Chromium{CheckpointDirectory: root}).PruneOAuthCheckpoints(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{expiredID, invalidID} {
		if _, err := os.Stat(filepath.Join(root, id+".json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("expired or interrupted private checkpoint survived cleanup")
		}
	}
	if _, err := os.Stat(filepath.Join(root, activeID+".json")); err != nil {
		t.Fatal("unexpired checkpoint was deleted")
	}
}

func TestOAuthRecoveryBindingRejectsExpiredExtendedAndNonOpaqueOwnership(t *testing.T) {
	for _, scenario := range []string{"expired", "extended", "owner", "lease", "automatic_without_id", "id_without_consent", "invalid_automatic_id"} {
		t.Run(scenario, func(t *testing.T) {
			options := checkpointOptions()
			switch scenario {
			case "expired":
				options.Recovery.ExpiresAt = time.Now().Add(-time.Second)
			case "extended":
				options.Recovery.ExpiresAt = time.Now().Add(16 * time.Minute)
			case "owner":
				options.Recovery.Owner = "actual-session-cookie"
			case "lease":
				options.Recovery.Lease = "../outside"
			case "automatic_without_id":
				options.Recovery.AutoCheckpoint = true
			case "id_without_consent":
				options.Recovery.CheckpointID = strings.Repeat("a", 48)
			case "invalid_automatic_id":
				options.Recovery.AutoCheckpoint = true
				options.Recovery.CheckpointID = "../outside"
			}
			if !errors.Is(options.Validate(), browserlogin.ErrOAuthCheckpoint) {
				t.Fatal("invalid recovery binding was accepted")
			}
		})
	}
}

func TestOAuthCheckpointStorageRejectsSymlinkDirectoryWithoutReadingTarget(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(private, link); err != nil {
		t.Fatal(err)
	}
	factory := browserlogin.Chromium{CheckpointDirectory: link}
	err := factory.DeleteOAuthCheckpoint(context.Background(), browserlogin.OAuthCheckpointRef{ID: strings.Repeat("a", 48), Owner: strings.Repeat("b", 64), Lease: strings.Repeat("c", 64)})
	if !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("symlink checkpoint directory was accepted")
	}
	entries, err := os.ReadDir(private)
	if err != nil || len(entries) != 0 {
		t.Fatal("rejected storage path was modified")
	}
}

func TestOAuthCheckpointDeleteRejectsTraversalAndWrongDirectoryPermissions(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	factory := browserlogin.Chromium{CheckpointDirectory: root}
	for _, id := range []string{"../outside", strings.Repeat("a", 48)} {
		err := factory.DeleteOAuthCheckpoint(context.Background(), browserlogin.OAuthCheckpointRef{ID: id, Owner: strings.Repeat("b", 64), Lease: strings.Repeat("c", 64)})
		if !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
			t.Fatal("invalid private checkpoint access was accepted")
		}
	}
}
