package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

func startRecoverableMixed(t *testing.T, f *batchFixture, content string) accountworkbench.WorkbenchRunView {
	t.Helper()
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: content, RecoveryEnabled: true, ExportOnly: true})
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("mixed recovery preview: %v %+v", err, preview.Errors)
	}
	view, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func restartMixedFixture(t *testing.T, f *batchFixture) {
	t.Helper()
	group := taskrunner.New(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = group.Shutdown(ctx)
	})
	f.runBoundary = &batchRunner{group: group, done: make(chan string, 30)}
	f.service = accountworkbench.New(f.private, f.taskBoundary, f.business, nil, f.runBoundary)
	f.service.UseOAuthBrowser(f.browsers)
	f.service.UseOAuthAssistTicks(make(chan time.Time))
}

func resumeMixedFixture(t *testing.T, f *batchFixture) accountworkbench.WorkbenchRunView {
	t.Helper()
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 || list[0].Kind != "mixed" {
		t.Fatalf("single parent recovery missing: %+v %v", list, err)
	}
	view, err := f.service.ResumeWorkbenchQueue(context.Background(), "owner", list[0].ID, accountworkbench.QueueRecoveryAction{Revision: list[0].Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelWorkbenchRun("owner", view.ID) })
	return view
}

func TestMixedQueueResumePreservesOriginalExpiryAndDoesNotExposeCredentials(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: mixedJSON, RecoveryEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.runner.Cancel()
	close(gate)
	f.awaitDone(t, view.ID)
	service := resumedQueueService(t, f)
	list, err := service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 || list[0].Kind != "mixed" {
		t.Fatalf("mixed queue missing: %v %+v", err, list)
	}
	raw := list[0]
	if raw.Status != "interrupted" || !raw.CanResume {
		t.Fatalf("mixed queue not resumable: %+v", raw)
	}
	encoded, _ := json.Marshal(list)
	if strings.Contains(string(encoded), "mixed-json-private") {
		t.Fatal("mixed queue id leaked credentials")
	}
	restored, err := service.ResumeWorkbenchQueue(context.Background(), "owner", raw.ID, accountworkbench.QueueRecoveryAction{Revision: raw.Revision, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	defer service.CancelWorkbenchRun("owner", restored.ID)
	a, _ := time.Parse(time.RFC3339Nano, view.ExpiresAt)
	b, _ := time.Parse(time.RFC3339Nano, restored.ExpiresAt)
	if !a.Equal(b) || restored.RecoveryID != raw.ID || restored.Available != 0 {
		t.Fatalf("mixed resume changed expiry/recovery: %+v", restored)
	}
	if _, err := service.ResumeWorkbenchQueue(context.Background(), "owner", raw.ID, accountworkbench.QueueRecoveryAction{Revision: raw.Revision, Confirmed: true}); !errors.Is(err, configstore.ErrWorkbenchQueue) {
		t.Fatal("mixed queue replayed")
	}
}

func TestMixedQueueInterruptionAfterJSONAndRTKeepsResultsWithoutRepeatingRefresh(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startRecoverableMixed(t, f, mixedJSON+"\nrt_mixed_private\nowner@example.com----private-login")
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	f.runner.Cancel()
	f.awaitDone(t, view.ID)
	restartMixedFixture(t, f)
	restored := resumeMixedFixture(t, f)
	f.awaitDone(t, restored.ID)
	result, err := f.service.ReadWorkbenchRun(context.Background(), "owner", restored.ID)
	if err != nil || result.Status != "ready" || result.Available != 2 || result.Items[2].Status != "failed" {
		t.Fatalf("partial mixed recovery lost completed sources: %+v %v", result, err)
	}
	preview, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", restored.ID)
	if err != nil || len(preview.Items) != 2 {
		t.Fatalf("recovered mixed results unavailable: %+v %v", preview, err)
	}
	f.remote.mu.Lock()
	refreshes := f.remote.refreshes
	f.remote.mu.Unlock()
	f.browsers.mu.Lock()
	opened := f.browsers.opened
	f.browsers.mu.Unlock()
	if refreshes != 1 || opened != 1 {
		t.Fatal("recovery repeated refresh or uncertain authorization")
	}
}

func TestMixedQueueInterruptedChildPreservesSuccessfulLoginAndSkipsUncertainLogin(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startRecoverableMixed(t, f, mixedJSON+"\nowner@example.com----private-first\nother@example.com----private-second")
	first := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", first.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	f.runner.Cancel()
	f.awaitDone(t, view.ID)
	restartMixedFixture(t, f)
	restored := resumeMixedFixture(t, f)
	f.awaitDone(t, restored.ID)
	result, err := f.service.ReadWorkbenchRun(context.Background(), "owner", restored.ID)
	if err != nil || result.Status != "ready" || result.Available != 2 || result.Items[1].Status != "succeeded" || result.Items[2].Status != "failed" {
		t.Fatalf("child login receipt not preserved: %+v %v", result, err)
	}
	f.browsers.mu.Lock()
	defer f.browsers.mu.Unlock()
	if f.browsers.opened != 2 {
		t.Fatal("child recovery opened a duplicate login")
	}
}

func TestMixedQueueCompletedResultsResumeWithoutRefreshAndChangedTemplatesAreRejected(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startRecoverableMixed(t, f, mixedJSON+"\nrt_mixed_private")
	f.awaitDone(t, view.ID)
	restartMixedFixture(t, f)
	restored := resumeMixedFixture(t, f)
	f.awaitDone(t, restored.ID)
	result, err := f.service.ReadWorkbenchRun(context.Background(), "owner", restored.ID)
	if err != nil || result.Available != 2 {
		t.Fatalf("completed queue lost results: %+v %v", result, err)
	}
	f.remote.mu.Lock()
	refreshes := f.remote.refreshes
	f.remote.mu.Unlock()
	if refreshes != 1 {
		t.Fatal("completed recovery repeated RT exchange")
	}
	restartMixedFixture(t, f)
	if _, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Changed", Config: configstore.WorkbenchTemplateConfig{"concurrency": json.RawMessage(`2`)}}); err != nil {
		t.Fatal(err)
	}
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 {
		t.Fatal("completed parent recovery not listed")
	}
	if _, err := f.service.ResumeWorkbenchQueue(context.Background(), "owner", list[0].ID, accountworkbench.QueueRecoveryAction{Revision: list[0].Revision, Confirmed: true}); err == nil {
		t.Fatal("template changes were ignored by mixed recovery")
	}
}

func TestMixedQueueExplicitCancellationDeletesPrivateStateWithoutResurrection(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	view := startRecoverableMixed(t, f, mixedJSON+"\nowner@example.com----private-login")
	if err := f.service.CancelWorkbenchRun("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	close(gate)
	f.awaitDone(t, view.ID)
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 0 {
		t.Fatal("cancelled mixed recovery remained available")
	}
}
