package routingwrite_test

import (
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
)

func TestPolicyChangeDefersOldTargetAndAllowsRecalculatedTargetNextRound(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	if err := fixture.store.PersistRoutingRound(t.Context(), nil, nil, nil, nil, nil, nil, nil, true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if pending, err := fixture.store.RoutingWritebackPending(t.Context()); err != nil || pending {
		t.Fatalf("initial calculation must be up to date: pending=%v err=%v", pending, err)
	}
	repository := &policyChangingLeaseStore{
		Store: fixture.store,
		patch: map[string]any{"auto_apply": map[string]any{"concurrency": false}},
	}
	service := routingwrite.New(routingTarget{url: fixture.serverURL}, repository)
	oldConcurrency := int64(6)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &oldConcurrency},
	}, "scheduler")
	if err != nil || result.Failed != 0 || result.RemoteWrite || len(result.Results) != 1 || !result.Results[0].Skipped {
		t.Fatalf("old target must be deferred: %+v err=%v", result, err)
	}
	pending, err := fixture.store.RoutingWritebackPending(t.Context())
	if err != nil || !pending {
		t.Fatalf("deferred policy change must still require recalculation: pending=%v err=%v", pending, err)
	}
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"auto_apply": map[string]any{"concurrency": true}}, "operator"); err != nil {
		t.Fatal(err)
	}
	newConcurrency := int64(5)
	result, err = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &newConcurrency},
	}, "scheduler")
	if err != nil || result.Failed != 0 || !result.RemoteWrite || result.Changed != 1 || result.Results[0].Skipped {
		t.Fatalf("fresh target must remain executable in the next round: %+v err=%v", result, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 5 {
		t.Fatalf("next round did not apply the new target: %v", fixture.states["41"])
	}
}

func TestPolicyChangeSkipDoesNotResolveAnEarlierWriteFailure(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	reason := "远端写入失败：HTTP 503"
	if err := fixture.store.RecordAccountOperation(t.Context(), business.AccountOperation{
		OperationID: "earlier-write-failure", OperationType: "routing.writeback", ObjectID: "41",
		GroupNames: []string{"capacity-group"}, State: "failed", Phase: "remote-write", Writeback: true, Error: &reason,
	}); err != nil {
		t.Fatal(err)
	}
	repository := &policyChangingLeaseStore{
		Store: fixture.store,
		patch: map[string]any{"auto_apply": map[string]any{"concurrency": false}},
	}
	desired := int64(6)
	result, err := routingwrite.New(routingTarget{url: fixture.serverURL}, repository).Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
	}, "scheduler")
	if err != nil || result.Failed != 0 || len(result.Results) != 1 || !result.Results[0].Skipped {
		t.Fatalf("policy change must defer the old target: %+v err=%v", result, err)
	}
	if _, err := fixture.store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	queue, err := fixture.store.NotificationQueueDetails(t.Context(), "test-channel", true)
	if err != nil || len(queue.ProducerFiring) != 1 || len(queue.ProducerRecovered) != 0 {
		t.Fatalf("policy skip resolved an earlier real failure: %+v err=%v", queue, err)
	}
	if queue.ProducerFiring[0].CauseCode != "APPLY_FAILED:"+reason {
		t.Fatalf("real failure cause was replaced: %+v", queue.ProducerFiring[0])
	}
}
