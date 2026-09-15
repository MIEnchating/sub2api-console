package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestOAuthQueueActiveReconnectReturnsOriginalTaskWithoutLaunchingAnotherBrowser(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	view := startRecoverableBatch(t, f, "owner@example.com----private-queue-password")
	defer f.service.CancelOAuthBatch("owner", view.ID)
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 || !list[0].Active || !list[0].CanResume || list[0].Scope != accountworkbench.ScopeManaged || list[0].Pending != 1 {
		t.Fatalf("active queue summary unavailable: %+v %v", list, err)
	}
	resumed, err := f.service.ResumeOAuthQueue(context.Background(), "owner", view.ID, accountworkbench.QueueRecoveryAction{Confirmed: true, Revision: list[0].Revision})
	if err != nil || resumed.ID != view.ID || resumed.Status != "queued" {
		t.Fatalf("active reconnect replaced the original task: %+v %v", resumed, err)
	}
	resumed.Items[0].Email = "modified@example.test"
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Items[0].Email != "owner@example.com" {
		t.Fatal("reconnect returned mutable internal rows")
	}
	close(gate)
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	f.browsers.mu.Lock()
	defer f.browsers.mu.Unlock()
	if f.browsers.opened != 1 {
		t.Fatal("active queue reconnect opened another browser")
	}
}

func TestOAuthQueueActiveResultCannotDeleteItsPrivateRecoveryWhileStillAvailable(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startRecoverableBatch(t, f, "owner@example.com----private-queue-password")
	defer f.service.CancelOAuthBatch("owner", view.ID)
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 || list[0].Succeeded != 1 || list[0].Pending != 0 || list[0].Review != 0 {
		t.Fatalf("completed summary invalid: %+v %v", list, err)
	}
	if err := f.service.DeleteQueueRecovery(context.Background(), "owner", view.ID, accountworkbench.QueueRecoveryAction{Confirmed: true, Revision: list[0].Revision}); err == nil {
		t.Fatal("private recovery was deleted while its credentials remained available")
	}
}

func TestQueueSummaryIncludesOnlyWhitelistedRowsAndSeparatesUncertainLogin(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startRecoverableMixed(t, f, mixedJSON+"\nrt_mixed_private\nowner@example.com----private-queue-password")
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	f.runner.Cancel()
	f.awaitDone(t, view.ID)
	service := resumedQueueService(t, f)
	list, err := service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 || list[0].Active || list[0].Pending != 0 || list[0].Succeeded != 2 || list[0].Review != 1 || len(list[0].Items) != 3 {
		t.Fatalf("mixed summary lost per-item outcomes: %+v %v", list, err)
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"mixed-json-private", "rt_mixed_private", "private-queue-password", "credentials", "access_token", "inputs", "checkpoints"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private queue payload escaped the summary whitelist")
		}
	}
	if list[0].Items[2].Email != "owner@example.com" || list[0].Items[2].Status != "running" {
		t.Fatal("uncertain account identity or original stage was lost")
	}
}
