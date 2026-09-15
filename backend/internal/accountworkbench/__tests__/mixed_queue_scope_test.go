package accountworkbench_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestMixedQueueActiveReconnectReturnsSameTaskWithoutLaunchingDuplicateRunner(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	view := startRecoverableMixed(t, f, mixedJSON)
	defer close(gate)
	defer f.service.CancelWorkbenchRun("owner", view.ID)
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("active parent not listed: %+v %v", list, err)
	}
	input := accountworkbench.QueueRecoveryAction{Revision: list[0].Revision, Confirmed: true}
	attached, err := f.service.ResumeWorkbenchQueue(context.Background(), "owner", list[0].ID, input)
	if err != nil || attached.ID != view.ID || attached.TaskID != view.TaskID {
		t.Fatalf("active reconnect started a different task: %+v %v", attached, err)
	}
	if f.runBoundary.calls.Load() != 1 {
		t.Fatal("active reconnect launched another runner")
	}
	input.Revision++
	if _, err := f.service.ResumeWorkbenchQueue(context.Background(), "owner", list[0].ID, input); err == nil {
		t.Fatal("stale reconnect revision accepted")
	}
	input.Revision--
	if _, err := f.service.ResumeWorkbenchQueue(context.Background(), "other", list[0].ID, input); err == nil {
		t.Fatal("foreign login session reconnected parent")
	}
}

func TestMixedQueueLocalExportResumesWithoutManagementTargetOrNetwork(t *testing.T) {
	f := newBatchFixture(t, nil)
	if err := f.private.Close(); err != nil {
		t.Fatal(err)
	}
	f.private = unconfiguredWorkbenchStore(t)
	f.service = accountworkbench.New(f.private, f.taskBoundary, f.business, nil, f.runBoundary)
	var requests atomic.Int64
	transport := oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("local export cannot call management")
	})
	f.service.UseTransport(transport)
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true, RecoveryEnabled: true, Content: mixedJSON})
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	restartMixedFixture(t, f)
	f.service.UseTransport(transport)
	list, err := f.service.QueueRecoveries(context.Background(), "owner", accountworkbench.ScopeLocalExport)
	if err != nil || len(list) != 1 {
		t.Fatalf("local recovery missing: %+v %v", list, err)
	}
	input := accountworkbench.QueueRecoveryAction{Scope: accountworkbench.ScopeLocalExport, Revision: list[0].Revision, Confirmed: true}
	restored, err := f.service.ResumeWorkbenchQueue(context.Background(), "owner", list[0].ID, input)
	if err != nil {
		t.Fatal(err)
	}
	defer f.service.CancelWorkbenchRun("owner", restored.ID)
	f.awaitDone(t, restored.ID)
	result, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", restored.ID)
	if err != nil || result.Target != "" || !result.ExportOnly || len(result.Items) != 1 {
		t.Fatalf("local restored result invalid: %+v %v", result, err)
	}
	if requests.Load() != 0 {
		t.Fatal("local mixed recovery accessed a management endpoint")
	}
}

func TestMixedQueueConcurrentStartCreatesOnlyOnePrivateParent(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: mixedJSON, RecoveryEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		view accountworkbench.WorkbenchRunView
		err  error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			view, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true)
			results <- outcome{view, err}
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			successes++
			defer f.service.CancelWorkbenchRun("owner", result.view.ID)
		}
	}
	defer close(gate)
	list, err := f.service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || successes != 1 || len(list) != 1 || f.runBoundary.calls.Load() != 1 {
		t.Fatalf("one preview created duplicate private parents: successes=%d queues=%d err=%v", successes, len(list), err)
	}
}
