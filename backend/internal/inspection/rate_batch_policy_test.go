package inspection

import (
	"context"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestAutomaticInspectionUsesCurrentPolicyBatchInsteadOfStaleSchedulerConfiguration(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Full, accountRateDue: true, policy: map[string]any{
		"account_rate_sync": map[string]any{"interval_seconds": int64(300), "batch_size": int64(3), "batch_percent": int64(0)},
	}}
	scheduler := &accountRateBatchSchedulerStub{}
	runner := NewRunner(repository, nil, &evidencePlannerStub{}, nil, nil, nil, nil, &countingTaskStore{}, scheduler)
	config := business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15, AccountRateSyncBatchSize: 7}
	result, err := runner.Run(context.Background(), RunRequest{Actor: "auto-inspection", Automatic: true, AutoConfig: &config})
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("run failed: result=%#v err=%v", result, err)
	}
	if scheduler.size != 3 || scheduler.percent != 0 {
		t.Fatalf("scheduled batch does not match current policy: size=%d percent=%d", scheduler.size, scheduler.percent)
	}
}
