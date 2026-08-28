package logs

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type cleanupSettingsStub struct {
	value configstore.LogCleanupSettings
	marks []time.Time
}

func (s *cleanupSettingsStub) LogCleanupSettings(context.Context) (configstore.LogCleanupSettings, error) {
	return s.value, nil
}
func (s *cleanupSettingsStub) ConfigureLogCleanup(_ context.Context, enabled bool, days int) (configstore.LogCleanupSettings, error) {
	s.value.Enabled, s.value.RetentionDays = enabled, days
	return s.value, nil
}
func (s *cleanupSettingsStub) MarkLogCleanupRun(_ context.Context, completed time.Time) error {
	s.marks = append(s.marks, completed)
	value := completed.UTC().Format(time.RFC3339Nano)
	s.value.LastRunAt = &value
	return nil
}

type businessCleanerStub struct {
	calls  int
	cutoff *time.Time
	result business.LogCleanupResult
}

func (s *businessCleanerStub) ClearLogRecords(_ context.Context, before *time.Time) (business.LogCleanupResult, error) {
	s.calls++
	s.cutoff = before
	return s.result, nil
}

type taskCleanerStub struct {
	calls, deleted, protected int64
	cutoff                    *time.Time
}

type cancellableTaskCleaner struct {
	started chan struct{}
	stopped chan struct{}
}

func (s *cancellableTaskCleaner) ClearLogs(ctx context.Context, _ *time.Time) (int64, int64, error) {
	close(s.started)
	<-ctx.Done()
	close(s.stopped)
	return 0, 0, ctx.Err()
}

func (s *taskCleanerStub) ClearLogs(_ context.Context, before *time.Time) (int64, int64, error) {
	s.calls++
	s.cutoff = before
	return s.deleted, s.protected, nil
}

func TestAutomaticCleanupRunsOncePerDayAndUsesRetentionCutoff(t *testing.T) {
	now := time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC)
	settings := &cleanupSettingsStub{value: configstore.LogCleanupSettings{Enabled: true, RetentionDays: 30}}
	businessCleaner := &businessCleanerStub{result: business.LogCleanupResult{Runs: 2, Events: 3, Changes: 4}}
	taskCleaner := &taskCleanerStub{deleted: 1, protected: 2}
	maintenance := NewMaintenance(settings, businessCleaner, taskCleaner)
	maintenance.now = func() time.Time { return now }

	ran, result, err := maintenance.RunDue(context.Background())
	if err != nil || !ran || result.Total != 10 || result.ProtectedTasks != 2 {
		t.Fatalf("ran=%v result=%#v err=%v", ran, result, err)
	}
	expectedCutoff := now.AddDate(0, 0, -30)
	if businessCleaner.cutoff == nil || !businessCleaner.cutoff.Equal(expectedCutoff) || taskCleaner.cutoff == nil || !taskCleaner.cutoff.Equal(expectedCutoff) {
		t.Fatalf("wrong cutoffs: business=%v tasks=%v", businessCleaner.cutoff, taskCleaner.cutoff)
	}
	ran, _, err = maintenance.RunDue(context.Background())
	if err != nil || ran || businessCleaner.calls != 1 || taskCleaner.calls != 1 {
		t.Fatalf("cleanup repeated inside 24 hours: ran=%v business=%d tasks=%d err=%v", ran, businessCleaner.calls, taskCleaner.calls, err)
	}
}

func TestManualCleanupUsesRequestedRetentionCutoff(t *testing.T) {
	now := time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC)
	settings := &cleanupSettingsStub{}
	businessCleaner := &businessCleanerStub{result: business.LogCleanupResult{Events: 2}}
	taskCleaner := &taskCleanerStub{deleted: 1}
	maintenance := NewMaintenance(settings, businessCleaner, taskCleaner)
	maintenance.now = func() time.Time { return now }

	result, err := maintenance.ClearExpired(context.Background(), 7)
	if err != nil || result.Total != 3 || result.RetentionDays != 7 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	expectedCutoff := now.AddDate(0, 0, -7)
	if result.CutoffAt != expectedCutoff.Format(time.RFC3339Nano) || businessCleaner.cutoff == nil || !businessCleaner.cutoff.Equal(expectedCutoff) || taskCleaner.cutoff == nil || !taskCleaner.cutoff.Equal(expectedCutoff) {
		t.Fatalf("wrong cutoff: result=%q business=%v tasks=%v", result.CutoffAt, businessCleaner.cutoff, taskCleaner.cutoff)
	}
}

func TestManualCleanupRejectsInvalidRetentionWithoutDeleting(t *testing.T) {
	businessCleaner := &businessCleanerStub{}
	taskCleaner := &taskCleanerStub{}
	maintenance := NewMaintenance(&cleanupSettingsStub{}, businessCleaner, taskCleaner)

	if _, err := maintenance.ClearExpired(context.Background(), 0); err == nil {
		t.Fatal("expected invalid retention error")
	}
	if businessCleaner.calls != 0 || taskCleaner.calls != 0 {
		t.Fatalf("invalid cleanup reached stores: business=%d tasks=%d", businessCleaner.calls, taskCleaner.calls)
	}
}

func TestDisabledAutomaticCleanupDoesNotDeleteLogs(t *testing.T) {
	settings := &cleanupSettingsStub{value: configstore.LogCleanupSettings{Enabled: false, RetentionDays: 30}}
	businessCleaner := &businessCleanerStub{}
	taskCleaner := &taskCleanerStub{}
	maintenance := NewMaintenance(settings, businessCleaner, taskCleaner)
	ran, _, err := maintenance.RunDue(context.Background())
	if err != nil || ran || businessCleaner.calls != 0 || taskCleaner.calls != 0 {
		t.Fatalf("disabled cleanup executed: ran=%v business=%d tasks=%d err=%v", ran, businessCleaner.calls, taskCleaner.calls, err)
	}
}

func TestStopWaitsForAutomaticCleanupLoopToExit(t *testing.T) {
	settings := &cleanupSettingsStub{value: configstore.LogCleanupSettings{Enabled: true, RetentionDays: 30}}
	taskCleaner := &cancellableTaskCleaner{started: make(chan struct{}), stopped: make(chan struct{})}
	maintenance := NewMaintenance(settings, &businessCleanerStub{}, taskCleaner)
	if err := maintenance.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-taskCleaner.started:
	case <-time.After(time.Second):
		t.Fatal("automatic cleanup did not start")
	}
	maintenance.Stop()
	select {
	case <-taskCleaner.stopped:
	default:
		t.Fatal("maintenance returned before cleanup observed cancellation")
	}
	maintenance.mu.Lock()
	done := maintenance.done
	maintenance.mu.Unlock()
	if done != nil {
		t.Fatal("maintenance loop remained registered after Stop")
	}
}
