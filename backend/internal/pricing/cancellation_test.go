package pricing

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"testing"
)

type cancelAfterTargetRead struct {
	*fakeTargets
	cancel context.CancelFunc
}

func (targets *cancelAfterTargetRead) TargetSettings(ctx context.Context) (configstore.TargetSettings, error) {
	settings, err := targets.fakeTargets.TargetSettings(ctx)
	targets.cancel()
	return settings, err
}

func TestApplyPlanDoesNotReportCancelledChangesAsUnchangedSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	targets := &cancelAfterTargetRead{fakeTargets: &fakeTargets{settings: configstore.TargetSettings{BaseURL: "http://127.0.0.1:1", AdminKey: "test", TimeoutSeconds: 1}}, cancel: cancel}
	service := New(&fakeRepository{}, targets, nil)
	decision := Decision{AccountID: "41", CurrentGroupIDs: []string{"6"}, DesiredGroupIDs: []string{"7"}, Changed: true}
	result, err := service.applyPlan(ctx, plan{snapshot: Snapshot{Accounts: 1, Changes: 1, Decisions: []Decision{decision}}}, Config{WriteConcurrency: 1}, "operator")
	if !errors.Is(err, context.Canceled) || result.Unchanged != 0 {
		t.Fatalf("cancelled change counted as successful unchanged: result=%#v error=%v", result, err)
	}
}
