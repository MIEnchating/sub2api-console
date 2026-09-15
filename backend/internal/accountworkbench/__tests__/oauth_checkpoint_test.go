package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestOAuthCheckpointSavePersistsBeforeSuspendAndRestoresSameTransactionManuallyAfterRestart(t *testing.T) {
	f := newCheckpointFixture(t)
	f.factory.beforeSuspend = func(options browserlogin.OAuthOptions) {
		list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
		if err != nil || len(list) != 1 || list[0].Status != "saving" || list[0].CanRestore {
			t.Error("worker suspension happened before a private save intent existed")
		}
		if options.Recovery == nil || strings.Contains(options.ProxyURL, "{session}") {
			t.Error("source browser lacks fixed recovery or proxy binding")
		}
	}
	original := f.start(t)
	checkpoint := f.save(t, original.ID)
	f.factory.mu.Lock()
	originalOptions := f.factory.original
	f.factory.mu.Unlock()
	f.configureService()
	f.factory.beforeRestore = func(options browserlogin.OAuthRestoreOptions) {
		list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
		if err != nil || len(list) != 1 || list[0].Status != "restoring" || list[0].CanRestore {
			t.Error("worker restore happened before the one-use claim was persisted")
		}
		if options.Options.AuthorizationURL != originalOptions.AuthorizationURL || options.Options.State != originalOptions.State || options.Options.ProxyURL != originalOptions.ProxyURL || options.Options.RedirectURI != originalOptions.RedirectURI || options.Checkpoint.Lease != originalOptions.Recovery.Lease || options.Lease == options.Checkpoint.Lease || !options.Options.Recovery.ExpiresAt.Equal(originalOptions.Recovery.ExpiresAt) {
			t.Error("restoration changed the reviewed OAuth transaction, proxy, expiry or reused its lease")
		}
	}
	view, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true})
	if err != nil || view.ID == original.ID || view.ExpiresAt != checkpoint.ExpiresAt {
		t.Fatalf("restored authorization = %+v, %v", view, err)
	}
	f.active = append(f.active, view.ID)
	waiting := f.phase(t, "waiting")
	if waiting.Operation != "account-workbench-oauth-recovery" || f.exchanges.Load() != 0 || f.factory.restores.Load() != 1 {
		t.Fatal("restoration did not create a paused new task")
	}
	if err := f.service.FinishOAuth("checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "waiting")
	if f.exchanges.Load() != 0 {
		t.Fatal("restoration automatically exchanged an authorization code")
	}
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", view.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", view.ID); err != nil {
		t.Fatal(err)
	}
	finished := f.phase(t, "authorized")
	if f.exchanges.Load() != 1 || finished.Operation != "account-workbench-oauth-recovery" {
		t.Fatal("manual restoration did not exchange exactly once")
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 || list[0].Status != "restored" || list[0].CanRestore {
		t.Fatalf("consumed recovery metadata = %+v, %v", list, err)
	}
	public, _ := json.Marshal([]any{original, checkpoint, view, list, finished})
	for _, secret := range []string{"private-login-password", "private-proxy", "private-checkpoint-code", "rt_checkpoint_private", originalOptions.State, originalOptions.Recovery.Lease} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("public checkpoint state exposed %s", secret)
		}
	}
}

func TestOAuthCheckpointResavingRestoredSessionKeepsRecoveryTaskOperation(t *testing.T) {
	f := newCheckpointFixture(t)
	checkpoint := f.save(t, f.start(t).ID)
	view, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, view.ID)
	f.phase(t, "waiting")
	resaved, err := f.service.SaveOAuthCheckpoint(context.Background(), "checkpoint-owner", view.ID, true)
	if err != nil || resaved.ID == checkpoint.ID || resaved.SourceTaskID != view.ID || resaved.ExpiresAt != checkpoint.ExpiresAt {
		t.Fatalf("resaved checkpoint = %+v, %v", resaved, err)
	}
	terminal := f.phase(t, "checkpointed")
	if terminal.Operation != "account-workbench-oauth-recovery" || terminal.Result["checkpoint_id"] != resaved.ID {
		t.Fatalf("resaved recovery task = %+v", terminal)
	}
}

func TestOAuthCheckpointRequiresConfirmationOwnerVersionAndOriginalTarget(t *testing.T) {
	f := newCheckpointFixture(t)
	original := f.start(t)
	if _, err := f.service.SaveOAuthCheckpoint(context.Background(), "checkpoint-owner", original.ID, false); err == nil || f.factory.suspends.Load() != 0 {
		t.Fatal("unconfirmed checkpoint suspended authorization")
	}
	if _, err := f.service.SaveOAuthCheckpoint(context.Background(), "another-owner", original.ID, true); err == nil || f.factory.suspends.Load() != 0 {
		t.Fatal("another session suspended authorization")
	}
	checkpoint := f.save(t, original.ID)
	for _, attempt := range []struct {
		owner  string
		action accountworkbench.OAuthCheckpointAction
	}{
		{"another-owner", accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true}},
		{"checkpoint-owner", accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: false}},
		{"checkpoint-owner", accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision + 1, Confirmed: true}},
	} {
		if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), attempt.owner, checkpoint.ID, attempt.action); err == nil || f.factory.restores.Load() != 0 {
			t.Fatal("invalid confirmation or owner restored a checkpoint")
		}
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-target-key", 3); err != nil {
		t.Fatal(err)
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 0 {
		t.Fatal("changed target could enumerate old checkpoints")
	}
	if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true}); err == nil || f.factory.restores.Load() != 0 {
		t.Fatal("changed target restored an old authorization")
	}
}

func TestOAuthCheckpointSaveFailureNeverCallsWorkerAndUnsafePageRemainsManual(t *testing.T) {
	f := newCheckpointFixture(t)
	original := f.start(t)
	f.state.failStatus = "saving"
	if _, err := f.service.SaveOAuthCheckpoint(context.Background(), "checkpoint-owner", original.ID, true); err == nil || f.factory.suspends.Load() != 0 {
		t.Fatal("failed durable intent still called worker suspend")
	}
	f.state.failStatus = ""
	f.factory.suspendErr = browserlogin.ErrOAuthCheckpointUnsafe
	if _, err := f.service.SaveOAuthCheckpoint(context.Background(), "checkpoint-owner", original.ID, true); !errors.Is(err, browserlogin.ErrOAuthCheckpointUnsafe) {
		t.Fatalf("unsafe page = %v", err)
	}
	view, err := f.service.ReadOAuth(context.Background(), "checkpoint-owner", original.ID)
	if err != nil || view.Status != "waiting" {
		t.Fatalf("unsafe page did not remain available for manual input: %+v, %v", view, err)
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 || list[0].Status != "failed" || list[0].CanRestore {
		t.Fatal("unsafe page produced a restorable checkpoint")
	}
}

func TestOAuthCheckpointFailedRestoreCannotReplayOldCheckpoint(t *testing.T) {
	f := newCheckpointFixture(t)
	checkpoint := f.save(t, f.start(t).ID)
	f.factory.restoreErr = errors.New("isolated response lost after restore")
	view, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, view.ID)
	f.phase(t, "failed")
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 || list[0].Status != "failed" || list[0].CanRestore {
		t.Fatalf("failed restoration = %+v, %v", list, err)
	}
	if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: list[0].Revision, Confirmed: true}); err == nil || f.factory.restores.Load() != 1 {
		t.Fatal("uncertain worker restoration was replayed")
	}
}

func TestOAuthCheckpointDeletionRevokesRestoreAndExpiryRemovesRecords(t *testing.T) {
	f := newCheckpointFixture(t)
	checkpoint := f.save(t, f.start(t).ID)
	if err := f.service.DeleteOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: false}); err == nil || f.factory.deletes.Load() != 0 {
		t.Fatal("unconfirmed deletion removed worker checkpoint")
	}
	if err := f.service.DeleteOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true}); err == nil || f.factory.deletes.Load() != 1 {
		t.Fatal("deleted checkpoint remained restorable")
	}
	next := f.save(t, f.start(t).ID)
	if err := f.private.PurgeExpiredWorkbenchOAuthCheckpoints(context.Background(), time.Now().Add(16*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", next.ID, accountworkbench.OAuthCheckpointAction{Revision: next.Revision, Confirmed: true}); err == nil || f.factory.restores.Load() != 0 {
		t.Fatal("expired checkpoint still restored")
	}
}

func TestOAuthCheckpointClaimPersistenceFailureLeavesReadyRecordAndDoesNotRestoreWorker(t *testing.T) {
	f := newCheckpointFixture(t)
	checkpoint := f.save(t, f.start(t).ID)
	f.state.failStatus = "restoring"
	if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true}); err == nil || f.factory.restores.Load() != 0 {
		t.Fatal("worker restored without a durable claim")
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 || !list[0].CanRestore || list[0].Revision != checkpoint.Revision {
		t.Fatalf("unclaimed checkpoint = %+v, %v", list, err)
	}
}

func TestOAuthCheckpointReceiptPersistenceFailureStopsOriginalWithoutAdvertisingReadyCheckpoint(t *testing.T) {
	f := newCheckpointFixture(t)
	original := f.start(t)
	f.state.failStatus = "ready"
	if _, err := f.service.SaveOAuthCheckpoint(context.Background(), "checkpoint-owner", original.ID, true); err == nil {
		t.Fatal("unsaved worker receipt was advertised as restorable")
	}
	terminal := f.phase(t, "checkpoint-review")
	if terminal.Result["checkpoint_id"] != nil {
		t.Fatal("failed receipt save advertised checkpoint ID in task")
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 || list[0].Status != "failed" || list[0].CanRestore {
		t.Fatalf("uncommitted receipt = %+v, %v", list, err)
	}
}
