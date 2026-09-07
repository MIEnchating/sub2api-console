package upstreamdelete

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type modeDeleteRepository struct {
	*deleteRepository
	mode string
}

func (repository *modeDeleteRepository) Mode(context.Context) (string, error) {
	return repository.mode, nil
}

func TestQueuedDeleteRejectsRestrictedModeBeforePrivateOrProjectionMutation(t *testing.T) {
	repository := &modeDeleteRepository{deleteRepository: &deleteRepository{
		preview: business.UpstreamDeletePreview{Host: "api.example", AccountIDs: []string{}},
	}, mode: runtimepolicy.Full}
	private := &deletePrivateStore{target: configstore.TargetSettings{BaseURL: "https://unused.example", AdminKey: "test"}}
	tasks := &deleteTaskObserver{updates: make(chan taskstore.Task, 1)}
	runner := &deferredDeleteRunner{}
	service := New(repository, private, tasks)
	service.UseTaskRunner(runner)
	if _, err := service.Enqueue(context.Background(), "api.example", []string{}, "tester"); err != nil {
		t.Fatal(err)
	}
	repository.mode = runtimepolicy.Monitoring
	runner.Run(context.Background())
	if private.deleteCalls != 0 || repository.deleteCalls != 0 {
		t.Fatalf("restricted runtime mode allowed upstream deletion: auth=%d projection=%d", private.deleteCalls, repository.deleteCalls)
	}
	finished := <-tasks.updates
	if finished.Status != "failed" || !strings.Contains(fmt.Sprint(finished.Result["error"]), "完全模式") {
		t.Fatalf("mode change was not persisted as a failed task: %#v", finished)
	}
}
