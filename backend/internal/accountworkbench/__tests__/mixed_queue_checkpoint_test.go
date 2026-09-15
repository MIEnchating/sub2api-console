package accountworkbench_test

import (
	"context"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestMixedQueueRestoresEmbeddedSafeBrowserCheckpointWithoutReplayingAuthorization(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "checkpoint-owner", accountworkbench.WorkbenchRunInput{RecoveryEnabled: true, ExportOnly: true, Content: mixedJSON + "\nowner@example.com----private-mixed-password"})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := f.service.StartWorkbenchRun(context.Background(), "checkpoint-owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.phase(t, "waiting")
	f.factory.mu.Lock()
	original := f.factory.original
	f.factory.mu.Unlock()
	factory.capture(original)
	if !f.runner.CancelTask(batch.ID) {
		t.Fatal("mixed parent could not be interrupted")
	}
	waitCheckpointBatchDone(t, f, batch.ID)
	f.configureService()
	f.service.UseOAuthBrowser(factory)
	queues, err := f.service.QueueRecoveries(context.Background(), "checkpoint-owner", "")
	if err != nil || len(queues) != 1 || queues[0].Kind != "mixed" {
		t.Fatalf("mixed recovery did not own the private child: %+v %v", queues, err)
	}
	restored, err := f.service.ResumeWorkbenchQueue(context.Background(), "checkpoint-owner", queues[0].ID, accountworkbench.QueueRecoveryAction{Revision: queues[0].Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	defer f.service.CancelWorkbenchRun("checkpoint-owner", restored.ID)
	child := f.phase(t, "waiting")
	factory.mu.Lock()
	restores := factory.restores
	factory.mu.Unlock()
	f.factory.mu.Lock()
	resumed := f.factory.original
	f.factory.mu.Unlock()
	if restores != 1 || f.exchanges.Load() != 0 || resumed.State != original.State || resumed.AuthorizationURL != original.AuthorizationURL || !resumed.Recovery.ExpiresAt.Equal(original.Recovery.ExpiresAt) {
		t.Fatal("mixed checkpoint recovery restarted or advanced the original authorization")
	}
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", child.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", child.ID); err != nil {
		t.Fatal(err)
	}
	waitCheckpointBatchDone(t, f, restored.ID)
	view, err := f.service.ReadWorkbenchRun(context.Background(), "checkpoint-owner", restored.ID)
	if err != nil || view.Available != 2 || view.Items[1].Status != "succeeded" || f.exchanges.Load() != 1 {
		t.Fatalf("restored mixed account not merged once: %+v %v", view, err)
	}
}
