package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAutomaticOAuthCheckpointPersistsBeforeBrowserAndRestoresOnlyReviewedSnapshotAfterInterruption(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	factory.beforeOpen = func(options browserlogin.OAuthOptions) {
		list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
		if err != nil || len(list) == 0 {
			t.Error("browser started before private authorization transaction was persisted")
		}
		if !options.Recovery.AutoCheckpoint || len(options.Recovery.CheckpointID) != 48 {
			t.Error("browser did not receive preallocated automatic recovery binding")
		}
	}
	view := startAutomaticCheckpoint(t, f)
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 || !list[0].CanRestore || !list[0].Active || list[0].CheckpointRevision != 0 || list[0].Status != "watching" || !list[0].Automatic {
		t.Fatalf("empty automatic checkpoint = %+v, %v", list, err)
	}
	f.factory.mu.Lock()
	options := f.factory.original
	f.factory.mu.Unlock()
	first := factory.capture(options)
	list, err = f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 || !list[0].CanRestore || list[0].CheckpointRevision != first.Revision {
		t.Fatalf("available automatic checkpoint = %+v, %v", list, err)
	}
	stale := list[0]
	latest := factory.capture(options)
	if !f.runner.CancelTask(view.ID) {
		t.Fatal("could not simulate runner interruption")
	}
	f.phase(t, "checkpoint-review")
	select {
	case <-f.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("interrupted authorization did not close")
	}
	f.configureService()
	f.service.UseOAuthBrowser(factory)
	if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", stale.ID, accountworkbench.OAuthCheckpointAction{Revision: stale.Revision, CheckpointRevision: stale.CheckpointRevision, Confirmed: true}); err == nil {
		t.Fatal("automatic restore accepted an obsolete browser snapshot")
	}
	restored, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", stale.ID, accountworkbench.OAuthCheckpointAction{Revision: stale.Revision, CheckpointRevision: latest.Revision, Confirmed: true})
	originalExpiry, _ := time.Parse(time.RFC3339Nano, view.ExpiresAt)
	restoredExpiry, expiryErr := time.Parse(time.RFC3339Nano, restored.ExpiresAt)
	if err != nil || restored.ID == view.ID || restored.CheckpointID == view.CheckpointID || expiryErr != nil || !restoredExpiry.Equal(originalExpiry) || !restored.RecoveryEnabled {
		t.Fatalf("automatic restore = %+v, %v", restored, err)
	}
	f.active = append(f.active, restored.ID)
	f.phase(t, "waiting")
	if f.exchanges.Load() != 0 {
		t.Fatal("restoration submitted an authorization code without manual action")
	}
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", restored.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", restored.ID); err != nil {
		t.Fatal(err)
	}
	terminal := f.phase(t, "authorized")
	select {
	case <-f.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("restored authorization did not close")
	}
	remaining, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(remaining) != 1 || remaining[0].Status != "restored" || remaining[0].CanRestore || f.exchanges.Load() != 1 {
		t.Fatalf("completed automatic checkpoints = %+v, %v", remaining, err)
	}
	public, _ := json.Marshal([]any{view, list, restored, terminal, remaining})
	for _, secret := range []string{options.State, options.Recovery.Lease, "rt_checkpoint_private", "private-checkpoint-code"} {
		if strings.Contains(string(public), secret) {
			t.Fatal("automatic checkpoint metadata exposed a private authorization value")
		}
	}
}

func TestAutomaticOAuthCheckpointExplicitCancelRevokesSnapshotAndPrivateTransaction(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	view := startAutomaticCheckpoint(t, f)
	f.factory.mu.Lock()
	options := f.factory.original
	f.factory.mu.Unlock()
	factory.capture(options)
	if err := f.service.CancelOAuth("checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "cancelled")
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 0 {
		t.Fatalf("cancelled automatic checkpoints = %+v, %v", list, err)
	}
	if _, err := factory.ReadOAuthCheckpoint(context.Background(), browserlogin.OAuthCheckpointRef{ID: options.Recovery.CheckpointID, Owner: options.Recovery.Owner, Lease: options.Recovery.Lease}); err == nil {
		t.Fatal("explicit cancellation left browser snapshot available")
	}
}

func TestAutomaticOAuthCheckpointRequiresExplicitOptInAndCapableBrowser(t *testing.T) {
	f := newCheckpointFixture(t)
	if _, err := f.service.StartOAuthWithInput(context.Background(), "checkpoint-owner", accountworkbench.OAuthStartInput{RecoveryEnabled: true}); err == nil {
		t.Fatal("automatic recovery started without a checkpoint reader")
	}
	view := f.start(t)
	if view.RecoveryEnabled || view.CheckpointID != "" {
		t.Fatal("ordinary OAuth implicitly enabled persistent automatic recovery")
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 0 {
		t.Fatal("ordinary OAuth persisted an automatic checkpoint transaction")
	}
}

func TestAutomaticOAuthCheckpointHistoryCancellationRevokesSnapshot(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	view := startAutomaticCheckpoint(t, f)
	f.factory.mu.Lock()
	options := f.factory.original
	f.factory.mu.Unlock()
	factory.capture(options)
	result, err := f.service.CancelHistory(context.Background(), accountworkbench.HistoryActionInput{Confirmed: true, Items: []taskstore.HistorySelection{{ID: view.ID}}})
	if err != nil || len(result) != 1 || !result[0].Cancelled {
		t.Fatalf("history cancellation = %+v, %v", result, err)
	}
	select {
	case <-f.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled authorization did not finish")
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 0 {
		t.Fatalf("history cancellation retained automatic checkpoint = %+v, %v", list, err)
	}
}
