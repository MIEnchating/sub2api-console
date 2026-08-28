package logs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const cleanupCheckInterval = time.Hour

type BusinessCleaner interface {
	ClearLogRecords(context.Context, *time.Time) (business.LogCleanupResult, error)
}

type TaskCleaner interface {
	ClearLogs(context.Context, *time.Time) (int64, int64, error)
}

type CleanupSettingsStore interface {
	LogCleanupSettings(context.Context) (configstore.LogCleanupSettings, error)
	ConfigureLogCleanup(context.Context, bool, int) (configstore.LogCleanupSettings, error)
	MarkLogCleanupRun(context.Context, time.Time) error
}

type CleanupResult struct {
	Tasks          int64  `json:"tasks"`
	Runs           int64  `json:"runs"`
	Events         int64  `json:"events"`
	Changes        int64  `json:"changes"`
	ProtectedTasks int64  `json:"protected_tasks"`
	Total          int64  `json:"total"`
	RetentionDays  int    `json:"retention_days"`
	CutoffAt       string `json:"cutoff_at"`
	CompletedAt    string `json:"completed_at"`
}

type CleanupStatus struct {
	configstore.LogCleanupSettings
	NextRunAt *string `json:"next_run_at"`
}

type Maintenance struct {
	settings CleanupSettingsStore
	business BusinessCleaner
	tasks    TaskCleaner
	now      func() time.Time

	runMu  sync.Mutex
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}
}

func NewMaintenance(settings CleanupSettingsStore, businessCleaner BusinessCleaner, taskCleaner TaskCleaner) *Maintenance {
	return &Maintenance{
		settings: settings, business: businessCleaner, tasks: taskCleaner,
		now: time.Now, wake: make(chan struct{}, 1),
	}
}

func (m *Maintenance) Status(ctx context.Context) (CleanupStatus, error) {
	settings, err := m.settings.LogCleanupSettings(ctx)
	if err != nil {
		return CleanupStatus{}, err
	}
	return cleanupStatus(settings, m.now().UTC()), nil
}

func (m *Maintenance) Update(ctx context.Context, enabled bool, retentionDays int) (CleanupStatus, error) {
	settings, err := m.settings.ConfigureLogCleanup(ctx, enabled, retentionDays)
	if err != nil {
		return CleanupStatus{}, err
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return cleanupStatus(settings, m.now().UTC()), nil
}

func (m *Maintenance) ClearExpired(ctx context.Context, retentionDays int) (CleanupResult, error) {
	if retentionDays < 1 || retentionDays > 3650 {
		return CleanupResult{}, errors.New("日志保留天数必须在 1 到 3650 之间")
	}
	cutoff := m.now().UTC().AddDate(0, 0, -retentionDays)
	return m.clear(ctx, cutoff, retentionDays)
}

func (m *Maintenance) RunDue(ctx context.Context) (bool, CleanupResult, error) {
	settings, err := m.settings.LogCleanupSettings(ctx)
	if err != nil {
		return false, CleanupResult{}, err
	}
	now := m.now().UTC()
	if !settings.Enabled || !cleanupDue(settings, now) {
		return false, CleanupResult{}, nil
	}
	cutoff := now.AddDate(0, 0, -settings.RetentionDays)
	result, err := m.clear(ctx, cutoff, settings.RetentionDays)
	return true, result, err
}

func (m *Maintenance) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return nil
	}
	loopContext, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	m.cancel = cancel
	m.done = done
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			if m.done == done {
				m.cancel = nil
				m.done = nil
			}
			m.mu.Unlock()
			close(done)
		}()
		m.loop(loopContext)
	}()
	return nil
}

func (m *Maintenance) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	done := m.done
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (m *Maintenance) loop(ctx context.Context) {
	ticker := time.NewTicker(cleanupCheckInterval)
	defer ticker.Stop()
	for {
		if _, _, err := m.RunDue(ctx); err != nil && ctx.Err() == nil {
			slog.Error("定时日志清理失败", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.wake:
		}
	}
}

func (m *Maintenance) clear(ctx context.Context, cutoff time.Time, retentionDays int) (CleanupResult, error) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	if m.business == nil || m.tasks == nil || m.settings == nil {
		return CleanupResult{}, errors.New("日志清理服务尚未就绪")
	}
	tasks, protected, err := m.tasks.ClearLogs(ctx, &cutoff)
	if err != nil {
		return CleanupResult{}, err
	}
	businessResult, err := m.business.ClearLogRecords(ctx, &cutoff)
	if err != nil {
		return CleanupResult{}, err
	}
	completed := m.now().UTC()
	if err := m.settings.MarkLogCleanupRun(ctx, completed); err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{
		Tasks: tasks, Runs: businessResult.Runs, Events: businessResult.Events,
		Changes: businessResult.Changes, ProtectedTasks: protected,
		RetentionDays: retentionDays, CutoffAt: cutoff.UTC().Format(time.RFC3339Nano),
		CompletedAt: completed.Format(time.RFC3339Nano),
	}
	result.Total = result.Tasks + result.Runs + result.Events + result.Changes
	return result, nil
}

func cleanupDue(settings configstore.LogCleanupSettings, now time.Time) bool {
	if settings.LastRunAt == nil {
		return true
	}
	last, err := time.Parse(time.RFC3339Nano, *settings.LastRunAt)
	return err != nil || !last.Add(24*time.Hour).After(now)
}

func cleanupStatus(settings configstore.LogCleanupSettings, now time.Time) CleanupStatus {
	status := CleanupStatus{LogCleanupSettings: settings}
	if !settings.Enabled {
		return status
	}
	next := now
	if settings.LastRunAt != nil {
		if last, err := time.Parse(time.RFC3339Nano, *settings.LastRunAt); err == nil {
			next = last.Add(24 * time.Hour)
		}
	}
	normalized := next.UTC().Format(time.RFC3339Nano)
	status.NextRunAt = &normalized
	return status
}
