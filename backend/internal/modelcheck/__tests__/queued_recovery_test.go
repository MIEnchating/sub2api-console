package modelcheck_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestQueuedManagedRecoveryCancellationReleasesPlanAndAccounts(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("cancelled recovery reached upstream") })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {}}
	plans, err := f.service.SaveDetectionTask(t.Context(), managedTask(), "test")
	if err != nil {
		t.Fatal(err)
	}
	f.service.UseTaskRunner(&deferredRunner{})
	task, err := f.service.RunDetectionTask(t.Context(), plans[0].ID, plans[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	pool := &deferredRunner{}
	restarted.UseTaskRunner(pool)
	queued, err := f.tasks.RecoveryTask(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.ResumeTask(t.Context(), queued); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.tasks.CancelRecovery(t.Context(), task.ID); err != nil || !ok {
		t.Fatalf("cancel: %v %v", ok, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pool.runs[0](ctx)
	if _, err := restarted.RunDetectionTask(t.Context(), plans[0].ID, plans[0].Version); err != nil {
		t.Fatalf("cancelled recovery still holds plan or accounts: %v", err)
	}
	if _, err := restarted.EnqueueAnimation(t.Context(), modelcheck.AnimationRequest{Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: "test-model"}}, TimeoutSeconds: 5}); err != nil {
		t.Fatalf("cancelled recovery still holds account reservation: %v", err)
	}
}

func TestQueuedManagedRecoveryRejectsMembersChangedAfterReservation(t *testing.T) {
	f := setup(t, 2, "openai", func(http.ResponseWriter, *http.Request) { t.Error("changed scope reached upstream") })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {}}
	plans, err := f.service.SaveDetectionTask(t.Context(), managedTask(), "test")
	if err != nil {
		t.Fatal(err)
	}
	f.service.UseTaskRunner(&deferredRunner{})
	task, err := f.service.RunDetectionTask(t.Context(), plans[0].ID, plans[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	// This task stopped before its first execution and has no bound members yet.
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	pool := &deferredRunner{}
	restarted.UseTaskRunner(pool)
	queued, err := f.tasks.RecoveryTask(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.ResumeTask(t.Context(), queued); err != nil {
		t.Fatal(err)
	}
	f.catalog.groupMembers["7"] = []string{"1", "2"}
	pool.runs[0](t.Context())
	got, err := f.tasks.Get(t.Context(), task.ID)
	if err != nil || got.Status != "failed" || !strings.Contains(got.Message, "检测分组成员在中断后已变化") {
		t.Fatalf("queued scope change accepted: status=%s message=%s err=%v", got.Status, got.Message, err)
	}
}
