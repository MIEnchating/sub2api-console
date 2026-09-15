package routingwrite_test

import (
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestCapacityWaitSkipsRepeatedRecoveryWithoutFailureAlertAndRetriesWhenCapacityReturns(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 10, false)
	enabled := true
	targets := map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: &enabled},
	}
	for range 2 {
		result, err := service.Apply(t.Context(), targets, "test")
		if err != nil || result.Failed != 0 || result.RemoteWrite || len(result.Results) != 1 {
			t.Fatalf("capacity wait must not fail or write: %+v err=%v", result, err)
		}
		item := result.Results[0]
		if !item.Skipped || item.Changed || item.Error != nil || item.Reason == nil || !strings.Contains(*item.Reason, "等待并发额度") {
			t.Fatalf("missing explicit waiting outcome: %+v", item)
		}
		if _, err := fixture.store.EvaluateAlertIncidents(t.Context()); err != nil {
			t.Fatal(err)
		}
		queue, err := fixture.store.NotificationQueueDetails(t.Context(), "test-channel", true)
		if err != nil || len(queue.ProducerFiring) != 0 || len(queue.ConsumerItems) != 0 {
			t.Fatalf("normal capacity wait generated notifications: %+v err=%v", queue, err)
		}
	}
	audit, err := fixture.store.AuditEvents(t.Context(), nil, false)
	if err != nil || len(audit) != 2 {
		t.Fatalf("missing waiting audit: %+v err=%v", audit, err)
	}
	for _, record := range audit {
		if record.State != "skipped" || record.Writeback || record.Error != nil {
			t.Fatalf("waiting audit falsely reports failure or mutation: %+v", record)
		}
	}
	fixture.mu.Lock()
	fixture.states["42"]["concurrency"] = 6
	fixture.mu.Unlock()
	result, err := service.Apply(t.Context(), targets, "test")
	if err != nil || result.Failed != 0 || result.Changed != 1 || !result.RemoteWrite || result.Results[0].Skipped {
		t.Fatalf("next scheduling round did not recover after capacity returned: %+v err=%v", result, err)
	}
}

func TestCapacityReadFailureStillReportsExecutionFailure(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 6, false)
	fixture.failSnapshot = true
	enabled := true
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: &enabled},
	}, "test")
	if err != nil || result.Failed != 1 || result.Results[0].Error == nil || result.Results[0].Skipped {
		t.Fatalf("snapshot failure was incorrectly silenced: %+v err=%v", result, err)
	}
}
