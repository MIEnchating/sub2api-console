package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

func startRecoverableBatch(t *testing.T, f *batchFixture, content string) accountworkbench.OAuthBatchView {
	t.Helper()
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Content: content, RecoveryEnabled: true})
	if err != nil || !preview.RecoveryEnabled || len(preview.Errors) > 0 {
		t.Fatalf("recovery preview: %v %v", err, preview.Errors)
	}
	view, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func resumedQueueService(t *testing.T, f *batchFixture) *accountworkbench.Service {
	t.Helper()
	group := taskrunner.New(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = group.Shutdown(ctx)
	})
	service := accountworkbench.New(f.private, f.taskBoundary, f.business, nil, group)
	service.UseOAuthBrowser(f.browsers)
	service.UseOAuthAssistTicks(make(chan time.Time))
	return service
}

func TestOAuthQueueInterruptionPreservesUnstartedCredentialsAndResumeKeepsOriginalExpiry(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	view := startRecoverableBatch(t, f, "owner@example.com----private-queue-password")
	f.runner.Cancel()
	close(gate)
	f.awaitDone(t, view.ID)
	service := resumedQueueService(t, f)
	list, err := service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 || !list[0].CanResume || list[0].Status != "interrupted" {
		t.Fatalf("interrupted queue unavailable: %v %v", err, list)
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "private-queue-password") {
		t.Fatal("recovery list leaked password")
	}
	restored, err := service.ResumeOAuthQueue(context.Background(), "owner", list[0].ID, accountworkbench.QueueRecoveryAction{Revision: list[0].Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	defer service.CancelOAuthBatch("owner", restored.ID)
	a, _ := time.Parse(time.RFC3339Nano, view.ExpiresAt)
	b, _ := time.Parse(time.RFC3339Nano, restored.ExpiresAt)
	if restored.ID == view.ID || !a.Equal(b) || restored.RecoveryID != view.ID || restored.Items[0].Status != "queued" {
		t.Fatal("resume changed scope, extended expiry, or lost pending row")
	}
	if _, err := service.ResumeOAuthQueue(context.Background(), "owner", list[0].ID, accountworkbench.QueueRecoveryAction{Revision: list[0].Revision, Confirmed: true}); err == nil {
		t.Fatal("same recovery reused")
	}
}

func TestOAuthQueueCompletedCredentialsResumeWithoutOpeningAnotherBrowser(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startRecoverableBatch(t, f, "owner@example.com----private-queue-password")
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	service := resumedQueueService(t, f)
	list, err := service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 || list[0].Status != "completed" {
		t.Fatalf("completed result missing: %v", err)
	}
	restored, err := service.ResumeOAuthQueue(context.Background(), "owner", list[0].ID, accountworkbench.QueueRecoveryAction{Revision: list[0].Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	defer service.CancelOAuthBatch("owner", restored.ID)
	for {
		task := f.awaitTask(t, "account-workbench-oauth-batch", "authorized")
		if task.ID == restored.ID {
			break
		}
	}
	result, err := service.ReadOAuthBatch("owner", restored.ID)
	if err != nil || result.Available != 1 {
		t.Fatalf("successful credentials not restored: %v", err)
	}
	f.browsers.mu.Lock()
	defer f.browsers.mu.Unlock()
	if f.browsers.opened != 1 {
		t.Fatal("completed account logged in again")
	}
}

func TestOAuthQueueInflightLoginDoesNotAutomaticallyReplayAfterInterruption(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startRecoverableBatch(t, f, "owner@example.com----private-queue-password")
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	f.runner.Cancel()
	f.awaitDone(t, view.ID)
	service := resumedQueueService(t, f)
	list, err := service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("queue unavailable: %v", err)
	}
	restored, err := service.ResumeOAuthQueue(context.Background(), "owner", list[0].ID, accountworkbench.QueueRecoveryAction{Revision: list[0].Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	defer service.CancelOAuthBatch("owner", restored.ID)
	if restored.Items[0].Status != "failed" || !strings.Contains(restored.Items[0].Message, "未确认") {
		t.Fatal("uncertain login became executable")
	}
	for {
		task := f.awaitTask(t, "account-workbench-oauth-batch", "failed")
		if task.ID == restored.ID {
			break
		}
	}
	f.browsers.mu.Lock()
	defer f.browsers.mu.Unlock()
	if f.browsers.opened != 1 {
		t.Fatal("inflight login replayed")
	}
}

func TestOAuthQueueExplicitCancelDeletesPrivateRecoveryAndDefaultBatchStoresNothing(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	view := startRecoverableBatch(t, f, "owner@example.com----private-queue-password")
	if err := f.service.CancelOAuthBatch("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	close(gate)
	f.awaitDone(t, view.ID)
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 0 {
		t.Fatal("cancelled recovery resurrected")
	}
	defaultView := f.startBatch(t, "owner@example.com")
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	list, err = f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 0 {
		t.Fatal("default batch persisted private credentials")
	}
	_ = f.service.CancelOAuthBatch("owner", defaultView.ID)
}
