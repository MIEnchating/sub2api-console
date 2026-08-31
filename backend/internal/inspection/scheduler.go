package inspection

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

const (
	leaseTTL             = 120 * time.Second
	renewEvery           = 30 * time.Second
	queuePreviewCacheTTL = 15 * time.Second
	queuePreviewTimeout  = 30 * time.Second
)

type Repository interface {
	AutoInspectionConfig(context.Context) (business.AutoInspectionConfig, error)
	UpdateAutoInspectionConfig(context.Context, business.AutoInspectionConfig) (business.AutoInspectionConfig, error)
	InspectionHeartbeats(context.Context, int) ([]business.InspectionHeartbeat, error)
	ClearInspectionHeartbeats(context.Context) (int64, error)
	RecordInspectionHeartbeat(context.Context, business.InspectionHeartbeat) error
	AcquireInspectionLease(context.Context, string, int, string, time.Time, time.Time, time.Duration) (bool, error)
	RenewInspectionLease(context.Context, string, time.Time, time.Duration) (bool, error)
	ReleaseInspectionLease(context.Context, string) error
	ActiveInspectionCheckedAt(context.Context, time.Time) (*string, error)
	ReconcileInterruptedInspections(context.Context, time.Time) (int, error)
}

type ExecutionResult struct {
	TaskID            *string
	Status            string
	Error             *string
	Operations        []string
	OperationTiming   []business.OperationTiming
	Skipped           bool
	MonitoringEnabled *bool
	Summary           *InspectionSummary
}

type InspectionSummary = business.InspectionRoundSummary

type Executor interface {
	Execute(context.Context, business.AutoInspectionConfig) (ExecutionResult, error)
}

type executionTaskReporterKey struct{}

func reportExecutionTask(ctx context.Context, taskID string) {
	reporter, ok := ctx.Value(executionTaskReporterKey{}).(func(string))
	if ok {
		reporter(taskID)
	}
}

type QueueItem struct {
	TaskType     string           `json:"task_type"`
	Label        string           `json:"label"`
	State        string           `json:"state"`
	ScheduledFor *string          `json:"scheduled_for"`
	Detail       string           `json:"detail"`
	TargetCount  *int             `json:"target_count"`
	Operations   []QueueOperation `json:"operations"`
}

type QueueOperation struct {
	Operation   string `json:"operation"`
	Label       string `json:"label"`
	TargetCount *int   `json:"target_count"`
	Cycle       string `json:"cycle"`
	Due         bool   `json:"due"`
}

type QueuePreviewer interface {
	Preview(context.Context, time.Time) (QueueItem, error)
}

type MonitoringConfiguration interface {
	MonitoringConfigured(context.Context) (bool, error)
}

type Status struct {
	business.AutoInspectionConfig
	Running              bool                           `json:"running"`
	LastRunAt            *string                        `json:"last_run_at"`
	NextRunAt            *string                        `json:"next_run_at"`
	LastStatus           *string                        `json:"last_status"`
	LastError            *string                        `json:"last_error"`
	LastTaskID           *string                        `json:"last_task_id"`
	LastRunDurationMS    int64                          `json:"last_run_duration_ms"`
	LastSummary          InspectionSummary              `json:"last_summary"`
	Queue                []QueueItem                    `json:"queue"`
	HeartbeatHistory     []business.InspectionHeartbeat `json:"heartbeat_history"`
	MonitoringConfigured bool                           `json:"monitoring_configured"`
	MonitoringEnabled    bool                           `json:"monitoring_enabled"`
	MonitoringCheckedAt  *string                        `json:"monitoring_checked_at"`
}

type Scheduler struct {
	repository Repository
	executor   Executor
	ownerID    string
	ownerHost  string
	now        func() time.Time

	mu                  sync.Mutex
	running             bool
	lastRunAt           *string
	nextRunAt           *string
	lastStatus          *string
	lastError           *string
	lastTaskID          *string
	lastRunDurationMS   int64
	lastSummary         InspectionSummary
	loopCancel          context.CancelFunc
	currentCancel       context.CancelFunc
	cancelRequested     bool
	monitoringEnabled   bool
	monitoringKnown     bool
	monitoringCheckedAt *string
	reconfigure         chan struct{}
	subscribers         map[chan struct{}]struct{}
	loopDone            chan struct{}
	previewMu           sync.Mutex
	previewItem         *QueueItem
	previewedAt         time.Time
	previewLoading      bool
}

func NewScheduler(repository Repository, executor Executor) (*Scheduler, error) {
	ownerID, err := randomOwnerID()
	if err != nil {
		return nil, err
	}
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	return &Scheduler{
		repository:  repository,
		executor:    executor,
		ownerID:     ownerID,
		ownerHost:   host,
		now:         time.Now,
		reconfigure: make(chan struct{}, 1),
		subscribers: make(map[chan struct{}]struct{}),
	}, nil
}

func (s *Scheduler) Status(ctx context.Context) (Status, error) {
	config, err := s.repository.AutoInspectionConfig(ctx)
	if err != nil {
		return Status{}, err
	}
	now := s.now().UTC()
	if _, err := s.repository.ReconcileInterruptedInspections(ctx, now); err != nil {
		return Status{}, err
	}
	activeCheckedAt, err := s.repository.ActiveInspectionCheckedAt(ctx, now)
	if err != nil {
		return Status{}, err
	}
	history, err := s.repository.InspectionHeartbeats(ctx, 20)
	if err != nil {
		return Status{}, err
	}
	completedAt := now.Format(time.RFC3339Nano)
	for index := range history {
		if history[index].Status == "running" && (activeCheckedAt == nil || history[index].CheckedAt != *activeCheckedAt) {
			history[index].Status = "failed"
			history[index].CompletedAt = &completedAt
			history[index].Error = stringPointer(business.InspectionInterruptedText())
		}
	}
	s.mu.Lock()
	result := Status{
		AutoInspectionConfig: config,
		Running:              s.running || activeCheckedAt != nil,
		LastRunAt:            cloneString(s.lastRunAt), NextRunAt: cloneString(s.nextRunAt),
		LastStatus: cloneString(s.lastStatus), LastError: cloneString(s.lastError),
		LastTaskID: cloneString(s.lastTaskID), Queue: []QueueItem{}, HeartbeatHistory: history,
		LastRunDurationMS: s.lastRunDurationMS, LastSummary: s.lastSummary,
		MonitoringEnabled:   s.monitoringKnown && s.monitoringEnabled,
		MonitoringCheckedAt: cloneString(s.monitoringCheckedAt),
	}
	s.mu.Unlock()
	monitoringConfigurationKnown := false
	if provider, available := s.executor.(MonitoringConfiguration); available {
		configured, configErr := provider.MonitoringConfigured(ctx)
		if configErr == nil {
			monitoringConfigurationKnown = true
			result.MonitoringConfigured = configured
		}
	}
	if !config.Enabled {
		result.NextRunAt = nil
	}
	result.Queue = s.queuePreview(config, now, result.NextRunAt, result.Running)
	if latest := latestCompleted(history); latest != nil {
		if result.LastRunAt == nil {
			result.LastRunAt = latest.CompletedAt
		}
		if result.LastStatus == nil {
			result.LastStatus = stringPointer(latest.Status)
		}
		if result.LastError == nil {
			result.LastError = latest.Error
		}
		if result.LastTaskID == nil {
			result.LastTaskID = latest.TaskID
		}
		if result.LastRunDurationMS == 0 && latest.CompletedAt != nil {
			started := parseTimeOrZero(latest.CheckedAt)
			completed := parseTimeOrZero(*latest.CompletedAt)
			if !started.IsZero() && !completed.IsZero() && !completed.Before(started) {
				result.LastRunDurationMS = completed.Sub(started).Milliseconds()
			}
		}
	}
	if summary := latestSummary(history); summary != nil && result.LastSummary == (InspectionSummary{}) {
		result.LastSummary = *summary
	}
	if enabled, checkedAt := latestMonitoringResult(history); enabled != nil && result.MonitoringCheckedAt == nil {
		result.MonitoringEnabled = *enabled
		result.MonitoringCheckedAt = checkedAt
	}
	if monitoringConfigurationKnown && !result.MonitoringConfigured {
		result.MonitoringEnabled = false
	}
	return result, nil
}

func (s *Scheduler) queuePreview(
	config business.AutoInspectionConfig,
	now time.Time,
	scheduledFor *string,
	running bool,
) []QueueItem {
	if !config.Enabled {
		return []QueueItem{{
			TaskType: "inspection", Label: "巡检计划未启用", State: "disabled",
			Detail: "自动巡检未启用，启用后才会计算到期操作", TargetCount: nil, Operations: []QueueOperation{},
		}}
	}
	if running {
		return []QueueItem{{
			TaskType: "inspection", Label: "当前轮次执行中", State: "waiting",
			Detail: "当前巡检完成后更新下一轮计划", Operations: []QueueOperation{},
		}}
	}
	previewer, available := s.executor.(QueuePreviewer)
	if !available {
		return []QueueItem{}
	}

	s.previewMu.Lock()
	item := cloneQueueItem(s.previewItem)
	fresh := item != nil && now.Sub(s.previewedAt) < queuePreviewCacheTTL
	if !fresh && !s.previewLoading {
		s.previewLoading = true
		go s.refreshQueuePreview(previewer)
	}
	s.previewMu.Unlock()

	if item == nil {
		return []QueueItem{{
			TaskType: "inspection", Label: "正在读取巡检计划", State: "waiting",
			ScheduledFor: cloneString(scheduledFor), Detail: "计划更新后会自动显示本轮安排", Operations: []QueueOperation{},
		}}
	}
	item.ScheduledFor = cloneString(scheduledFor)
	return []QueueItem{*item}
}

func (s *Scheduler) refreshQueuePreview(previewer QueuePreviewer) {
	ctx, cancel := context.WithTimeout(context.Background(), queuePreviewTimeout)
	defer cancel()
	item, err := previewer.Preview(ctx, s.now().UTC())
	if err != nil {
		item = QueueItem{
			TaskType: "inspection", Label: "巡检计划读取失败", State: "blocked",
			Detail: "巡检计划读取失败：" + stringsOrType(err), Operations: []QueueOperation{},
		}
	}
	if item.Operations == nil {
		item.Operations = []QueueOperation{}
	}
	item.ScheduledFor = nil
	s.previewMu.Lock()
	s.previewItem = &item
	s.previewedAt = s.now().UTC()
	s.previewLoading = false
	s.previewMu.Unlock()
	s.notify()
}

func cloneQueueItem(item *QueueItem) *QueueItem {
	if item == nil {
		return nil
	}
	result := *item
	result.ScheduledFor = cloneString(item.ScheduledFor)
	result.TargetCount = cloneInt(item.TargetCount)
	result.Operations = append([]QueueOperation(nil), item.Operations...)
	return &result
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func (s *Scheduler) UpdateConfig(ctx context.Context, config business.AutoInspectionConfig) (Status, error) {
	stored, err := s.repository.UpdateAutoInspectionConfig(ctx, config)
	if err != nil {
		return Status{}, err
	}
	now := s.now().UTC()
	s.mu.Lock()
	s.nextRunAt = scheduledAfter(now, stored)
	cancel := s.currentCancel
	if !stored.Enabled && cancel != nil {
		s.cancelRequested = true
	}
	s.mu.Unlock()
	if !stored.Enabled && cancel != nil {
		cancel()
	}
	select {
	case s.reconfigure <- struct{}{}:
	default:
	}
	s.notify()
	return s.Status(ctx)
}

func (s *Scheduler) ClearHistory(ctx context.Context) (int64, error) {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if running {
		return 0, errors.New("当前巡检正在执行，请停止或等待完成后再清空心跳记录")
	}
	active, err := s.repository.ActiveInspectionCheckedAt(ctx, s.now().UTC())
	if err != nil {
		return 0, err
	}
	if active != nil {
		return 0, errors.New("其他实例正在执行巡检，请等待完成后再清空心跳记录")
	}
	deleted, err := s.repository.ClearInspectionHeartbeats(ctx)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.lastRunAt = nil
	s.lastStatus = nil
	s.lastError = nil
	s.lastTaskID = nil
	s.lastRunDurationMS = 0
	s.lastSummary = InspectionSummary{}
	s.mu.Unlock()
	s.notify()
	return deleted, nil
}

func (s *Scheduler) Cancel(ctx context.Context) (Status, bool, error) {
	config, err := s.repository.AutoInspectionConfig(ctx)
	if err != nil {
		return Status{}, false, err
	}
	s.mu.Lock()
	canceled := s.running && s.currentCancel != nil
	s.mu.Unlock()
	config.Enabled = false
	status, err := s.UpdateConfig(ctx, config)
	return status, canceled, err
}

func (s *Scheduler) Resume(ctx context.Context) (Status, error) {
	config, err := s.repository.AutoInspectionConfig(ctx)
	if err != nil {
		return Status{}, err
	}
	config.Enabled = true
	return s.UpdateConfig(ctx, config)
}

func (s *Scheduler) Subscribe() (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	s.mu.Lock()
	s.subscribers[updates] = struct{}{}
	s.mu.Unlock()
	return updates, func() {
		s.mu.Lock()
		delete(s.subscribers, updates)
		s.mu.Unlock()
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.loopCancel != nil {
		s.mu.Unlock()
		return nil
	}
	loopContext, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.loopCancel = cancel
	s.loopDone = done
	s.mu.Unlock()
	if _, err := s.repository.ReconcileInterruptedInspections(loopContext, s.now().UTC()); err != nil {
		cancel()
		s.finishLoop(done)
		return err
	}
	config, err := s.repository.AutoInspectionConfig(loopContext)
	if err != nil {
		cancel()
		s.finishLoop(done)
		return err
	}
	s.mu.Lock()
	s.nextRunAt = scheduledAfter(s.now().UTC(), config)
	s.mu.Unlock()
	go func() {
		defer s.finishLoop(done)
		s.loop(loopContext)
	}()
	return nil
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	cancel := s.loopCancel
	done := s.loopDone
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *Scheduler) finishLoop(done chan struct{}) {
	s.mu.Lock()
	if s.loopDone == done {
		s.loopCancel = nil
		s.loopDone = nil
	}
	s.mu.Unlock()
	close(done)
}

func (s *Scheduler) RunDue(ctx context.Context, at time.Time, force bool) (bool, error) {
	return s.runDue(ctx, at, force, s.executor)
}

func (s *Scheduler) RunWithExecutor(ctx context.Context, at time.Time, executor Executor) (bool, error) {
	if executor == nil {
		return false, errors.New("巡检执行器不能为空")
	}
	return s.runDue(ctx, at, true, executor)
}

func (s *Scheduler) runDue(ctx context.Context, at time.Time, force bool, executor Executor) (bool, error) {
	current := at.UTC()
	config, err := s.repository.AutoInspectionConfig(ctx)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	if (!config.Enabled && !force) || s.running || (!force && s.nextRunAt != nil && current.Before(parseTimeOrZero(*s.nextRunAt))) {
		if !config.Enabled && !force {
			s.nextRunAt = nil
		}
		s.mu.Unlock()
		return false, nil
	}
	executionContext, cancelExecution := context.WithCancel(ctx)
	s.running = true
	s.cancelRequested = false
	s.currentCancel = cancelExecution
	s.mu.Unlock()
	s.notify()

	acquired, err := s.repository.AcquireInspectionLease(
		executionContext, s.ownerID, os.Getpid(), s.ownerHost, current, s.now().UTC(), leaseTTL,
	)
	if err != nil {
		cancelExecution()
		s.setNotRunning()
		return false, err
	}
	if !acquired {
		cancelExecution()
		s.mu.Lock()
		s.nextRunAt = scheduledAfter(current, config)
		s.mu.Unlock()
		s.setNotRunning()
		return false, nil
	}

	s.mu.Lock()
	s.nextRunAt = nil
	s.lastTaskID = nil
	s.lastStatus = nil
	s.lastError = nil
	s.mu.Unlock()
	if err := s.repository.RecordInspectionHeartbeat(executionContext, business.InspectionHeartbeat{
		CheckedAt: current.Format(time.RFC3339Nano), Status: "running",
		Operations: []string{}, OperationTiming: []business.OperationTiming{},
	}); err != nil {
		cancelExecution()
		if releaseErr := s.releaseLease(); releaseErr != nil {
			slog.Error("巡检启动失败后释放租约失败", "error", releaseErr)
		}
		s.setNotRunning()
		return false, err
	}

	var reportOnce sync.Once
	executorContext := context.WithValue(executionContext, executionTaskReporterKey{}, func(taskID string) {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			return
		}
		reportOnce.Do(func() {
			if err := s.repository.RecordInspectionHeartbeat(context.Background(), business.InspectionHeartbeat{
				CheckedAt: current.Format(time.RFC3339Nano), Status: "running", TaskID: stringPointer(taskID),
				Operations: []string{}, OperationTiming: []business.OperationTiming{},
			}); err != nil {
				return
			}
			s.mu.Lock()
			s.lastTaskID = stringPointer(taskID)
			s.mu.Unlock()
			s.notify()
		})
	})
	renewalDone := make(chan struct{})
	go s.renewLease(executionContext, renewalDone)
	result, executionErr := executor.Execute(executorContext, config)
	cancelExecution()
	<-renewalDone
	completed := s.now().UTC()
	status := result.Status
	if status == "" {
		status = "failed"
	}
	errorText := result.Error
	if executionErr != nil {
		status = "failed"
		message := stringsOrType(executionErr)
		errorText = &message
	}
	s.mu.Lock()
	wasCanceled := s.cancelRequested
	s.mu.Unlock()
	if wasCanceled {
		status = "cancelled"
		errorText = nil
	}
	completedText := completed.Format(time.RFC3339Nano)
	heartbeat := business.InspectionHeartbeat{
		CheckedAt: current.Format(time.RFC3339Nano), CompletedAt: &completedText, Status: status,
		Operations: result.Operations, OperationTiming: result.OperationTiming,
		TaskID: result.TaskID, Error: errorText, Skipped: result.Skipped,
		Summary: result.Summary, MonitoringEnabled: result.MonitoringEnabled,
	}
	historyErr := s.repository.RecordInspectionHeartbeat(context.Background(), heartbeat)
	releaseErr := s.releaseLease()
	s.mu.Lock()
	s.running = false
	s.currentCancel = nil
	s.cancelRequested = false
	s.lastRunAt = &completedText
	s.lastStatus = stringPointer(status)
	s.lastError = cloneString(errorText)
	s.lastTaskID = cloneString(result.TaskID)
	s.lastRunDurationMS = completed.Sub(current).Milliseconds()
	if result.Summary != nil {
		s.lastSummary = *result.Summary
	}
	s.nextRunAt = scheduledAfter(completed, config)
	if result.MonitoringEnabled != nil {
		s.monitoringKnown = true
		s.monitoringEnabled = *result.MonitoringEnabled
		s.monitoringCheckedAt = &completedText
	}
	s.mu.Unlock()
	s.notify()
	if historyErr != nil {
		return true, historyErr
	}
	if releaseErr != nil {
		return true, releaseErr
	}
	return true, nil
}

func (s *Scheduler) renewLease(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(renewEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			renewed, err := s.repository.RenewInspectionLease(ctx, s.ownerID, now.UTC(), leaseTTL)
			if err == nil && !renewed {
				return
			}
		}
	}
}

func (s *Scheduler) releaseLease() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.repository.ReleaseInspectionLease(ctx, s.ownerID)
}

func (s *Scheduler) setNotRunning() {
	s.mu.Lock()
	s.running = false
	s.currentCancel = nil
	s.cancelRequested = false
	s.mu.Unlock()
	s.notify()
}

func (s *Scheduler) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for subscriber := range s.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

func (s *Scheduler) loop(ctx context.Context) {
	for {
		config, err := s.repository.AutoInspectionConfig(ctx)
		if err != nil {
			if !waitContext(ctx, time.Second) {
				return
			}
			continue
		}
		if !config.Enabled {
			s.mu.Lock()
			s.nextRunAt = nil
			s.mu.Unlock()
			if !s.waitForReconfigure(ctx, -1) {
				return
			}
			continue
		}
		s.mu.Lock()
		if s.nextRunAt == nil {
			s.nextRunAt = scheduledAfter(s.now().UTC(), config)
		}
		next := parseTimeOrZero(*s.nextRunAt)
		s.mu.Unlock()
		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}
		if s.waitForReconfigure(ctx, delay) {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if _, err := s.RunDue(ctx, s.now().UTC(), false); err != nil && ctx.Err() == nil {
			slog.Error("自动巡检执行失败", "error", err)
		}
	}
}

func (s *Scheduler) waitForReconfigure(ctx context.Context, delay time.Duration) bool {
	if delay < 0 {
		select {
		case <-ctx.Done():
			return false
		case <-s.reconfigure:
			return true
		}
	}
	if delay == 0 {
		return false
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-s.reconfigure:
		return true
	case <-timer.C:
		return false
	}
}

func scheduledAfter(now time.Time, config business.AutoInspectionConfig) *string {
	if !config.Enabled {
		return nil
	}
	result := now.UTC().Add(time.Duration(config.IntervalSeconds) * time.Second).Format(time.RFC3339Nano)
	return &result
}

func latestCompleted(history []business.InspectionHeartbeat) *business.InspectionHeartbeat {
	for index := range history {
		if history[index].Status == "succeeded" || history[index].Status == "failed" || history[index].Status == "cancelled" {
			return &history[index]
		}
	}
	return nil
}

func latestSummary(history []business.InspectionHeartbeat) *InspectionSummary {
	for index := range history {
		if history[index].Summary != nil {
			return history[index].Summary
		}
	}
	return nil
}

func latestMonitoringResult(history []business.InspectionHeartbeat) (*bool, *string) {
	for index := range history {
		if history[index].MonitoringEnabled != nil {
			return history[index].MonitoringEnabled, cloneString(history[index].CompletedAt)
		}
	}
	return nil, nil
}

func parseTimeOrZero(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func randomOwnerID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func stringPointer(value string) *string {
	result := value
	return &result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func stringsOrType(err error) string {
	if err == nil {
		return ""
	}
	if value := err.Error(); value != "" {
		return value
	}
	return errors.New("巡检执行失败").Error()
}
