package accountops_test

import (
	"context"
	"testing"
)

func TestQueuedModelDiscoveryStopsWhenAccountEntersManualPriority(t *testing.T) {
	f := newSettingsFixture(t)
	if _, err := f.service.EnqueueModelDiscovery(context.Background(), []string{"41"}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repository.AssignManualPriority(context.Background(), "41", 3, "100", 100, false, "test"); err != nil {
		t.Fatal(err)
	}
	f.runner.run(context.Background())
	if f.requests.Load() != 0 {
		t.Fatalf("queued model discovery accessed a manually protected account: requests=%d", f.requests.Load())
	}
}
