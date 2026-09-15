package accountworkbench_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type queueCommitResponseLost struct {
	*configstore.Store
	failed atomic.Bool
}

func (s *queueCommitResponseLost) SaveWorkbenchQueue(ctx context.Context, input configstore.WorkbenchQueue) (configstore.WorkbenchQueue, error) {
	saved, err := s.Store.SaveWorkbenchQueue(ctx, input)
	if err == nil && !s.failed.Swap(true) {
		return configstore.WorkbenchQueue{}, context.Canceled
	}
	return saved, err
}

func TestOAuthQueueCommittedPrivateSaveReadbackAllowsStartupWithoutDuplicateWrite(t *testing.T) {
	f := newBatchFixture(t, nil)
	store := &queueCommitResponseLost{Store: f.private}
	f.service = accountworkbench.New(store, f.taskBoundary, f.business, nil, f.runBoundary)
	f.service.UseOAuthBrowser(f.browsers)
	f.service.UseOAuthAssistTicks(make(chan time.Time))
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Content: "owner@example.com----private-start-password", RecoveryEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal("committed private queue save was not reconciled", err)
	}
	defer f.service.CancelOAuthBatch("owner", view.ID)
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 || list[0].TaskID != view.ID {
		t.Fatal("startup created a duplicate or lost its private queue")
	}
}
