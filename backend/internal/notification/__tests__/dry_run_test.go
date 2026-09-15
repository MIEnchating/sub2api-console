package notification_test

import (
	"context"
	"testing"
)

func TestDryRunLeavesPendingAlertsAvailableForRealDelivery(t *testing.T) {
	fixture := newUpstreamDeliveryFixture(t)
	ctx := context.Background()
	preview, err := fixture.service.Deliver(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || preview.Sent != 3 || preview.Batches != 1 || len(fixture.sender.messages) != 0 {
		t.Fatalf("unexpected simulated result: %+v messages=%v", preview, fixture.sender.messages)
	}
	var attempted int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM alert_deliveries WHERE attempts > 0`).Scan(&attempted); err != nil {
		t.Fatal(err)
	}
	if attempted != 0 {
		t.Fatalf("simulated delivery consumed live alerts: attempted=%d", attempted)
	}
	actual, err := fixture.service.Deliver(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Sent != 3 || len(fixture.sender.messages) != 1 {
		t.Fatalf("real delivery skipped simulated alerts: %+v messages=%v", actual, fixture.sender.messages)
	}
}
