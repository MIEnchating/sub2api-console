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

func securityCheckpointRequest(options browserlogin.OAuthOptions, meta browserlogin.OAuthCheckpoint) browserlogin.SecurityCheckpointOptions {
	return browserlogin.SecurityCheckpointOptions{Checkpoint: meta, SessionID: strings.Repeat("d", 48), Options: options}
}

func securityCheckpointFixture(t *testing.T) (browserlogin.Chromium, browserlogin.SecurityCheckpointOptions, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	options := checkpointOptions()
	meta := browserlogin.OAuthCheckpoint{ID: strings.Repeat("e", 48), Owner: options.Recovery.Owner, Lease: options.Recovery.Lease, ExpiresAt: options.Recovery.ExpiresAt, Revision: 1, Stage: "sms_code"}
	raw, err := json.Marshal(struct {
		Version int                          `json:"version"`
		Meta    browserlogin.OAuthCheckpoint `json:"meta"`
		Options browserlogin.OAuthOptions    `json:"options"`
		URL     string                       `json:"url"`
		Storage map[string]any               `json:"storage"`
	}{Version: 1, Meta: meta, Options: options, URL: "https://auth.openai.com/phone-verification", Storage: map[string]any{"url": "https://auth.openai.com/phone-verification", "local": map[string]string{}, "session": map[string]string{}}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, meta.ID+".json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return browserlogin.Chromium{Executable: filepath.Join(root, "missing-chromium"), CheckpointDirectory: root}, securityCheckpointRequest(options, meta), path
}

func TestSecurityCheckpointRejectsChangedBindingWithoutConsumingOriginal(t *testing.T) {
	for _, scenario := range []string{"owner", "lease", "revision", "missing_revision", "stage", "expiry", "proxy", "state", "session_id"} {
		t.Run(scenario, func(t *testing.T) {
			factory, request, path := securityCheckpointFixture(t)
			switch scenario {
			case "owner":
				request.Checkpoint.Owner = strings.Repeat("f", 64)
			case "lease":
				request.Checkpoint.Lease = strings.Repeat("f", 64)
			case "revision":
				request.Checkpoint.Revision++
			case "missing_revision":
				request.Checkpoint.Revision = 0
			case "stage":
				request.Checkpoint.Stage = "workspace"
			case "expiry":
				request.Checkpoint.ExpiresAt = request.Checkpoint.ExpiresAt.Add(time.Minute)
				request.Options.Recovery.ExpiresAt = request.Checkpoint.ExpiresAt
			case "proxy":
				request.Options.ProxyURL = "http://proxy.example:8080"
			case "state":
				changed := validOAuthOptions("different-transaction")
				changed.Recovery = request.Options.Recovery
				request.Options = changed
			case "session_id":
				request.SessionID = request.Checkpoint.ID
			}
			if _, err := factory.OpenSecurityFromCheckpoint(context.Background(), request); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
				t.Fatalf("changed binding accepted: %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("rejected transfer consumed the original checkpoint")
			}
		})
	}
}

func TestSecurityCheckpointConsumesBeforeBrowserLaunchAndCannotReplayAfterFailure(t *testing.T) {
	factory, request, path := securityCheckpointFixture(t)
	if _, err := factory.OpenSecurityFromCheckpoint(context.Background(), request); err == nil || errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatalf("expected isolated missing browser startup failure: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("startup failure retained an executable checkpoint")
	}
	if _, err := factory.OpenSecurityFromCheckpoint(context.Background(), request); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("failed transfer replayed the consumed checkpoint")
	}
}

func TestSecurityCheckpointCanceledRequestDoesNotConsumeSavedState(t *testing.T) {
	factory, request, path := securityCheckpointFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := factory.OpenSecurityFromCheckpoint(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled request attempted transfer")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("canceled request consumed checkpoint")
	}
}
