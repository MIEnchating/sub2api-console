package accountworkbench_test

import (
	"context"
	"testing"
	"time"
)

func TestQueueSchedulerPurgesPrivatePayloadEvenWhenMaintenanceIsDisabled(t *testing.T) {
	gate := make(chan struct{})
	f := newBatchFixture(t, gate)
	view := startRecoverableBatch(t, f, "owner@example.com----private-expiring-password")
	f.runner.Cancel()
	close(gate)
	f.awaitDone(t, view.ID)
	service := resumedQueueService(t, f)
	ticks := make(chan time.Time, 1)
	service.UseMaintenanceTicks(ticks)
	ticks <- time.Now().Add(3 * time.Hour)
	close(ticks)
	service.RunScheduler(context.Background())
	list, err := service.QueueRecoveries(context.Background(), "owner", "")
	if err != nil || len(list) != 0 {
		t.Fatal("scheduler left expired private queue credentials behind")
	}
}
