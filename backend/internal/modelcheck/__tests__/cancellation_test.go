package modelcheck_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestCancellingBehaviorCheckWithQueuedCombinationsCompletesTask(t *testing.T) {
	started := make(chan struct{}, 100)
	f := setup(t, 3, "openai", func(_ http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		started <- struct{}{}
		<-request.Context().Done()
	})
	queued, err := f.service.Enqueue(context.Background(), modelcheck.Request{
		AccountIDs: []string{"1", "2", "3"}, Models: f.service.Capabilities().SolModels[:1], TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("running task was not cancellable")
	}
	if task := finished(t, f); task.ID != queued.ID || task.Status != "cancelled" {
		t.Fatalf("cancelled task did not reach its terminal state: %+v", task)
	}
}
