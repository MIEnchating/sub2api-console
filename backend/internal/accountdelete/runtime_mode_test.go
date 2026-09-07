package accountdelete

import (
	"context"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type modeDeleteRepository struct {
	*deleteRepository
	mode string
}

func (repository *modeDeleteRepository) Mode(context.Context) (string, error) {
	return repository.mode, nil
}

func TestQueuedDeleteRejectsRestrictedModeBeforeRemoteMutation(t *testing.T) {
	repository := &modeDeleteRepository{deleteRepository: &deleteRepository{account: boundAccount()}, mode: runtimepolicy.Full}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	admin := &deleteAdmin{}
	tasks, runner := &deleteTaskStore{}, &heldDeleteRunner{}
	service := New(repository, configuredDeletePrivate(), keys, tasks)
	service.adminFactory = func(configstore.TargetSettings) (Admin, error) { return admin, nil }
	service.UseTaskRunner(runner)
	preview, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Enqueue(context.Background(), "37", preview.Binding, preview.ManagementBaseURL, "tester"); err != nil {
		t.Fatal(err)
	}
	repository.mode = runtimepolicy.Monitoring
	runner.run(context.Background())
	if len(keys.deleteCalls) != 0 || len(admin.calls) != 0 || repository.deleted {
		t.Fatalf("restricted runtime mode allowed deletion: keys=%v accounts=%v local=%v", keys.deleteCalls, admin.calls, repository.deleted)
	}
	finished := tasks.tasks[len(tasks.tasks)-1]
	if finished.Status != "failed" || !strings.Contains(finished.Message, "完全模式") {
		t.Fatalf("mode change was not persisted as a failed task: %#v", finished)
	}
}
