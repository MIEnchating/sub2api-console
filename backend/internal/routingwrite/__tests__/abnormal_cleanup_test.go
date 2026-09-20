package routingwrite_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"testing"
)

func TestLongAbnormalPausePreservesReasonWhenAlreadyUnschedulable(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 2, 2, false)
	action, schedulable := "pause", false
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": {
		AccountID: "41", GroupNames: []string{"capacity-group"}, DesiredHealth: "fused", Schedulable: &schedulable, CleanupAction: &action, CleanupReason: "长期异常",
	}}, "test")
	if err != nil || result.Failed != 0 || result.Succeeded != 1 {
		t.Fatalf("pause failed: %+v %v", result, err)
	}
	account, err := fixture.store.Account(t.Context(), "41")
	if err != nil || account.Paused == nil || !*account.Paused || account.PausedReason == nil || *account.PausedReason != "长期异常，已自动暂停" {
		t.Fatalf("wrong visible pause reason: %+v %v", account, err)
	}
}
