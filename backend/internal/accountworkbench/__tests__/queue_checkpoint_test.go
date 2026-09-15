package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func waitCheckpointBatchDone(t *testing.T, f *checkpointFixture, id string) {
	t.Helper()
	for {
		select {
		case completed := <-f.completed:
			if completed == id {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("batch did not finish")
		}
	}
}

func TestOAuthQueueSafeCheckpointResumesOriginalTransactionWithoutLoginReplay(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "checkpoint-owner", accountworkbench.OAuthBatchPreviewInput{RecoveryEnabled: true, Content: "owner@example.com----private-login-password"})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := f.service.StartOAuthBatch(context.Background(), "checkpoint-owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.phase(t, "waiting")
	f.factory.mu.Lock()
	original := f.factory.original
	f.factory.mu.Unlock()
	factory.capture(original)
	checkpoints, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(checkpoints) != 1 || checkpoints[0].CanRestore {
		t.Fatal("batch checkpoint must require parent recovery")
	}
	if !f.runner.CancelTask(batch.ID) {
		t.Fatal("parent could not be interrupted")
	}
	waitCheckpointBatchDone(t, f, batch.ID)
	f.configureService()
	f.service.UseOAuthBrowser(factory)
	queues, err := f.service.QueueRecoveries(context.Background(), "checkpoint-owner", "")
	if err != nil || len(queues) != 1 {
		t.Fatal("interrupted queue missing")
	}
	restored, err := f.service.ResumeOAuthQueue(context.Background(), "checkpoint-owner", queues[0].ID, accountworkbench.QueueRecoveryAction{Revision: queues[0].Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	defer f.service.CancelOAuthBatch("checkpoint-owner", restored.ID)
	child := f.phase(t, "waiting")
	f.factory.mu.Lock()
	resumed := f.factory.original
	f.factory.mu.Unlock()
	if resumed.State != original.State || resumed.AuthorizationURL != original.AuthorizationURL || !resumed.Recovery.ExpiresAt.Equal(original.Recovery.ExpiresAt) || resumed.Recovery.Lease == original.Recovery.Lease {
		t.Fatal("checkpoint recovery replaced or extended original authorization transaction")
	}
	factory.mu.Lock()
	restores := factory.restores
	factory.mu.Unlock()
	if restores != 1 || f.exchanges.Load() != 0 {
		t.Fatal("restoration replayed login or exchanged a code before manual input")
	}
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", child.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", child.ID); err != nil {
		t.Fatal(err)
	}
	waitCheckpointBatchDone(t, f, restored.ID)
	view, err := f.service.ReadOAuthBatch("checkpoint-owner", restored.ID)
	if err != nil || view.Available != 1 || view.Items[0].Status != "succeeded" || f.exchanges.Load() != 1 {
		t.Fatal("restored account did not become one durable result")
	}
}

func TestOAuthQueueBindsCheckpointBeforeBrowserOpensAndPersistsSuccessBeforePublicCompletion(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	ownerDigest := sha256.Sum256([]byte("checkpoint-owner"))
	owner := hex.EncodeToString(ownerDigest[:])
	factory.beforeOpen = func(options browserlogin.OAuthOptions) {
		checkpoints, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
		if err != nil || len(checkpoints) != 1 {
			t.Error("checkpoint missing before open")
			return
		}
		queueViews, err := f.service.QueueRecoveries(context.Background(), "checkpoint-owner", "")
		if err != nil || len(queueViews) != 1 {
			t.Error("queue missing before open")
			return
		}
		records, err := f.private.WorkbenchQueues(context.Background(), owner, queueTargetHash(t, f.importFixture))
		if err != nil || len(records) != 1 {
			t.Error("private queue missing")
			return
		}
		record, err := f.private.WorkbenchQueue(context.Background(), owner, records[0].Target, records[0].ID)
		var payload struct {
			Checkpoints map[int]struct {
				ID string `json:"id"`
			} `json:"checkpoints"`
		}
		if err != nil || json.Unmarshal(record.Payload, &payload) != nil || payload.Checkpoints[0].ID != checkpoints[0].ID {
			t.Error("browser launched before parent checkpoint binding was durable")
		}
	}
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "checkpoint-owner", accountworkbench.OAuthBatchPreviewInput{RecoveryEnabled: true, Content: "owner@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := f.service.StartOAuthBatch(context.Background(), "checkpoint-owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer f.service.CancelOAuthBatch("checkpoint-owner", batch.ID)
	child := f.phase(t, "waiting")
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", child.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "authorized")
	records, err := f.private.WorkbenchQueues(context.Background(), owner, queueTargetHash(t, f.importFixture))
	if err != nil || len(records) != 1 {
		t.Fatal("queue missing after authorized event")
	}
	record, err := f.private.WorkbenchQueue(context.Background(), owner, records[0].Target, records[0].ID)
	var payload struct {
		Results []json.RawMessage `json:"results"`
	}
	if err != nil || json.Unmarshal(record.Payload, &payload) != nil || len(payload.Results) != 1 {
		t.Fatal("public completion preceded durable successful credentials")
	}
	waitCheckpointBatchDone(t, f, batch.ID)
}

func queueTargetHash(t *testing.T, f *importFixture) string {
	t.Helper()
	target, err := f.private.TargetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]string{target.BaseURL, target.AdminKey})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
