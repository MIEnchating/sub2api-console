package accountops_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
)

func TestOrdinarySettingsCannotQueueReservedManualPriority(t *testing.T) {
	f := newSettingsFixture(t)
	_, err := f.service.EnqueueSettings(context.Background(), "41", accountops.SettingsInput{
		Priority: 3, LoadFactor: "1", Concurrency: 3,
	}, "test")
	if err == nil || !strings.Contains(err.Error(), "手动控制") || f.runner.run != nil {
		t.Fatalf("ordinary settings queued a reserved manual priority: err=%v queued=%t", err, f.runner.run != nil)
	}
}

func TestQueuedSettingsRecheckExpandedManualPriorityReservationBeforeRemoteAccess(t *testing.T) {
	f := newSettingsFixture(t)
	_, err := f.service.EnqueueSettings(context.Background(), "41", accountops.SettingsInput{
		Priority: 20, LoadFactor: "1", Concurrency: 3,
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repository.UpdatePolicy(context.Background(), map[string]any{
		"advanced_policy": map[string]any{"manual_priority": map[string]any{"reserved_max": 20}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	f.runner.run(context.Background())
	if f.tasks.last.Status != "failed" || !strings.Contains(fmt.Sprint(f.tasks.last.Result["error"]), "手动控制") || f.requests.Load() != 0 {
		t.Fatalf("queued settings bypassed expanded reservation: task=%+v requests=%d", f.tasks.last, f.requests.Load())
	}
}
