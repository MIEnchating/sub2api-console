package accountautomation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const (
	Skill               = "sub2api-account-model-automation"
	Operation           = "account-model-automation"
	GroupOperation      = "account-model-automation-group"
	scheduleSettingKey  = "account_model_automation.schedules"
	taskGroupSettingKey = "account_model_automation.task_groups"
)

type AccountModelSource interface {
	Models(context.Context, string) ([]string, error)
	PreviewModels(context.Context, string) ([]string, error)
}

type AccountGroupSource interface {
	AccountIDsByGroups(context.Context, []string) ([]string, error)
}

type PlatformManagement interface {
	ChannelsByID(context.Context, string, []string, int, int) (newapimanagement.ChannelPage, error)
	ChangeChannelModels(context.Context, string, string, newapimanagement.ChannelModelChange) (newapimanagement.Channel, error)
	ModelPriceCatalog(context.Context, string, bool) (newapimanagement.ModelPriceCatalog, error)
	SaveModelPrices(context.Context, string, []newapimanagement.ModelPriceInput) (newapimanagement.RemoteSnapshot, error)
}

type ModeReader interface {
	Mode(context.Context) (string, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
	Get(context.Context, string) (taskstore.Task, error)
	CancelTask(context.Context, string) (bool, error)
}

type ScheduleRepository interface {
	JSONSetting(context.Context, string) (string, error)
	UpdateJSONSettings(context.Context, []string, func(map[string]string) (map[string]string, error)) error
}

type Schedule struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Request         Request  `json:"request"`
	Enabled         bool     `json:"enabled"`
	ScheduleType    string   `json:"schedule_type,omitempty"`
	DailyTimes      []string `json:"daily_times,omitempty"`
	Timezone        string   `json:"timezone,omitempty"`
	IntervalMinutes int      `json:"interval_minutes"`
	LastRunAt       *string  `json:"last_run_at"`
	NextRunAt       *string  `json:"next_run_at"`
}

type ChannelTarget struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type Request struct {
	GroupIDs       []string        `json:"group_ids"`
	PlatformID     string          `json:"platform_id"`
	Channels       []ChannelTarget `json:"channels"`
	SelectedModels []string        `json:"selected_models"`
	PublishMissing bool            `json:"publish_missing"`
	PriceEnabled   bool            `json:"price_enabled"`
	PricingSource  string          `json:"pricing_source"`
	DryRun         bool            `json:"dry_run"`
	ConfigVersion  int             `json:"config_version"`
	PreviewTaskID  string          `json:"preview_task_id,omitempty"`
	Actor          string          `json:"-"`
}

type ResultItem struct {
	AccountID   string   `json:"account_id,omitempty"`
	ChannelID   string   `json:"channel_id,omitempty"`
	Models      []string `json:"models,omitempty"`
	Planned     []string `json:"planned,omitempty"`
	ChannelName string   `json:"channel_name,omitempty"`
	Added       []string `json:"added,omitempty"`
	Existing    []string `json:"existing,omitempty"`
	Status      string   `json:"status"`
	Message     string   `json:"message,omitempty"`
}

type Service struct {
	accounts         AccountModelSource
	groups           AccountGroupSource
	manager          PlatformManagement
	mode             ModeReader
	store            TaskStore
	runner           taskrunner.Runner
	groupRunner      taskrunner.Runner
	scheduleRepo     ScheduleRepository
	scheduleMu       sync.Mutex
	scheduleCancel   context.CancelFunc
	scheduleDone     chan struct{}
	schedulesStopped bool
	runningSchedules map[string]*scheduledRun
	runningTaskGroup bool
}

type recoveryInput struct {
	Request         Request `json:"request"`
	TaskGroupTaskID string  `json:"task_group_task_id"`
}

func (s *Service) UseScheduleRepository(repository ScheduleRepository) { s.scheduleRepo = repository }

// UseGroupRunner isolates the orchestration worker from its child tasks. A
// group waits for each child preview and write task, so sharing the channel
// runner could otherwise consume all workers and deadlock the sequence.
func (s *Service) UseGroupRunner(runner taskrunner.Runner) { s.groupRunner = runner }

func (s *Service) Schedules(ctx context.Context) ([]Schedule, error) {
	if s.scheduleRepo == nil {
		return []Schedule{}, nil
	}
	raw, err := s.scheduleRepo.JSONSetting(ctx, scheduleSettingKey)
	if err != nil {
		return nil, err
	}
	return decodeSchedules(raw)
}

func decodeSchedules(raw string) ([]Schedule, error) {
	if strings.TrimSpace(raw) == "" {
		return []Schedule{}, nil
	}
	var schedules []Schedule
	if err := json.Unmarshal([]byte(raw), &schedules); err != nil {
		return nil, errors.New("账号模型任务配置损坏")
	}
	for index := range schedules {
		if schedules[index].ScheduleType == "" {
			schedules[index].ScheduleType = "interval"
		}
		// Legacy schedules have no explicit model scope. Keep them editable, but
		// disable publication until the operator chooses models and saves.
		if schedules[index].Request.SelectedModels == nil {
			schedules[index].Request.PublishMissing = false
			schedules[index].Request.PriceEnabled = false
		}
		schedules[index].Request = normalizeRequest(schedules[index].Request)
		if err := validateSchedule(schedules[index]); err != nil {
			return nil, err
		}
	}
	return schedules, nil
}

func (s *Service) SaveSchedule(ctx context.Context, schedule Schedule) (Schedule, error) {
	if schedule.ScheduleType == "" {
		schedule.ScheduleType = "interval"
	}
	schedule.Request = normalizeRequest(schedule.Request)
	if err := validateSchedule(schedule); err != nil {
		return Schedule{}, err
	}
	if schedule.Enabled && schedule.NextRunAt == nil {
		next := schedule.NextRunAfter(time.Now().UTC()).Format(time.RFC3339Nano)
		schedule.NextRunAt = &next
	}
	if schedule.ID == "" {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err != nil {
			return Schedule{}, err
		}
		schedule.ID = hex.EncodeToString(bytes)
	}
	err := s.updateScheduleSettings(ctx, []string{scheduleSettingKey}, func(settings *scheduleSettings) error {
		for index := range settings.schedules {
			if settings.schedules[index].ID == schedule.ID {
				preserveScheduleRunTimes(&schedule, settings.schedules[index], time.Now().UTC())
				settings.schedules[index] = schedule
				return nil
			}
		}
		settings.schedules = append(settings.schedules, schedule)
		return nil
	})
	if err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

func (s *Service) DeleteSchedule(ctx context.Context, id string) error {
	return s.updateScheduleSettings(ctx, []string{scheduleSettingKey, taskGroupSettingKey}, func(settings *scheduleSettings) error {
		filtered := settings.schedules[:0]
		for _, schedule := range settings.schedules {
			if schedule.ID != id {
				filtered = append(filtered, schedule)
			}
		}
		settings.schedules = filtered
		settings.groups = removeScheduleFromTaskGroups(settings.groups, id)
		return nil
	})
}

func (s *Service) RunSchedule(ctx context.Context, id string) (taskstore.Task, error) {
	task, _, err := s.runSchedule(ctx, id, false)
	return task, err
}

func (s *Service) runSchedule(ctx context.Context, id string, fromTaskGroup bool) (taskstore.Task, *scheduledRun, error) {
	if err := ctx.Err(); err != nil {
		return taskstore.Task{}, nil, err
	}
	run, err := s.reserveSchedule(ctx, id, fromTaskGroup)
	if err != nil {
		return taskstore.Task{}, nil, err
	}
	release := func() { s.releaseSchedule(id, run) }
	// Request cancellation can abort startup; accepted standalone work outlives HTTP.
	requestContext := ctx
	stopRequest := context.AfterFunc(requestContext, func() { run.cancel(context.Cause(requestContext)) })
	defer stopRequest()
	ctx = run.ctx
	schedules, err := s.Schedules(ctx)
	if err != nil {
		release()
		return taskstore.Task{}, nil, err
	}
	for index := range schedules {
		if schedules[index].ID != id {
			continue
		}
		original := schedules[index]
		schedule := original
		if len(schedule.Request.Channels) > 0 {
			if s.manager == nil {
				release()
				return taskstore.Task{}, nil, errors.New("账号模型服务尚未就绪")
			}
			channels, err := s.manager.ChannelsByID(ctx, schedule.Request.PlatformID, channelIDs(schedule.Request.Channels), 0, len(schedule.Request.Channels))
			if err != nil {
				release()
				return taskstore.Task{}, nil, fmt.Errorf("渠道读取失败：%w", err)
			}
			latestChannels, err := channelTargets(schedule.Request.Channels, channels.Items)
			if err != nil {
				release()
				return taskstore.Task{}, nil, err
			}
			schedule.Request.Channels = latestChannels
		}
		previewRequest := schedule.Request
		previewRequest.DryRun = true
		previewRequest.PreviewTaskID = ""
		preview, err := s.enqueueOwned(ctx, previewRequest, schedule.ID, run.ctx)
		if err != nil {
			release()
			return taskstore.Task{}, nil, err
		}
		now := time.Now().UTC()
		next := schedule.NextRunAfter(now).Format(time.RFC3339Nano)
		nowText := now.Format(time.RFC3339Nano)
		schedule.LastRunAt, schedule.NextRunAt = &nowText, &next
		// The preview is read-only. Start its write follow-up only after the
		// original configuration is verified and its run metadata is committed.
		if err := s.recordScheduleStart(ctx, original, schedule); err != nil {
			release()
			return taskstore.Task{}, nil, err
		}
		go func() {
			defer release()
			run.outcome = s.completeScheduledRun(run.ctx, schedule, preview.ID)
			if run.outcome.status == "failed" {
				run.cancel(errors.New(run.outcome.message))
			}
		}()
		return preview, run, nil
	}
	release()
	return taskstore.Task{}, nil, errors.New("运营任务不存在")
}

func terminal(status string) bool {
	switch status {
	case "succeeded", "failed", "partial", "cancelled":
		return true
	default:
		return false
	}
}

func validateSchedule(schedule Schedule) error {
	if strings.TrimSpace(schedule.Name) == "" || utf8.RuneCountInString(schedule.Name) > 128 {
		return errors.New("任务名称不能为空且不能超过 128 个字符")
	}
	if err := validateScheduleTiming(schedule); err != nil {
		return err
	}
	return validateRequest(schedule.Request)
}

func New(accounts AccountModelSource, manager PlatformManagement, mode ModeReader, store TaskStore, runner taskrunner.Runner, groups ...AccountGroupSource) *Service {
	var groupSource AccountGroupSource
	if len(groups) > 0 {
		groupSource = groups[0]
	}
	return &Service{accounts: accounts, groups: groupSource, manager: manager, mode: mode, store: store, runner: runner}
}

func (s *Service) Enqueue(ctx context.Context, request Request) (taskstore.Task, error) {
	s.scheduleMu.Lock()
	runningTaskGroup := s.runningTaskGroup
	s.scheduleMu.Unlock()
	if runningTaskGroup {
		return taskstore.Task{}, errors.New("账号模型任务组正在执行")
	}
	return s.enqueue(ctx, request, "")
}

func (s *Service) enqueue(ctx context.Context, request Request, scheduleID string) (taskstore.Task, error) {
	return s.enqueueOwned(ctx, request, scheduleID, nil)
}

func (s *Service) enqueueOwned(ctx context.Context, request Request, scheduleID string, owner context.Context) (taskstore.Task, error) {
	if err := ctx.Err(); err != nil {
		return taskstore.Task{}, err
	}

	request = normalizeRequest(request)
	if err := validateRequest(request); err != nil {
		return taskstore.Task{}, err
	}
	if s.accounts == nil || (len(request.Channels) > 0 && s.manager == nil) || s.store == nil || s.runner == nil {
		return taskstore.Task{}, errors.New("账号模型服务尚未就绪")
	}
	if !request.DryRun && s.mode != nil {
		mode, err := s.mode.Mode(ctx)
		if err != nil {
			return taskstore.Task{}, err
		}
		if mode != runtimepolicy.Full {
			return taskstore.Task{}, errors.New("账号模型写入需要完全模式")
		}
	}
	if !request.DryRun {
		preview, err := s.store.Get(ctx, request.PreviewTaskID)
		if err != nil || preview.Operation != Operation || preview.Status != "succeeded" || preview.Result["dry_run"] != true || preview.Result["scope"] != requestScope(request) {
			return taskstore.Task{}, errors.New("预览已失效，请先重新预览并确认影响范围")
		}
		_, ok := preview.Result["models"].([]any)
		if !ok {
			return taskstore.Task{}, errors.New("预览缺少模型结果，请重新预览")
		}
		created, err := time.Parse(time.RFC3339Nano, preview.CreatedAt)
		if err != nil || time.Since(created) > 30*time.Minute {
			return taskstore.Task{}, errors.New("预览超过 30 分钟，请重新预览")
		}
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return taskstore.Task{}, err
	}
	id := hex.EncodeToString(idBytes)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: id, Skill: Skill, Operation: Operation, Status: "queued", Message: "账号模型任务已排队", CreatedAt: now, UpdatedAt: now}
	if err := taskstore.WithRecovery(&task, recoveryInput{Request: request, TaskGroupTaskID: taskGroupOwnerID(owner)}); err != nil {
		return taskstore.Task{}, err
	}
	task.Result = map[string]any{"phase": "queued", "request_id": id, "platform_id": request.PlatformID, "dry_run": request.DryRun, "scope": requestScope(request), "items": []ResultItem{}, "models": []string{}, "result_version": 2, "selected_models": request.SelectedModels, "pricing_enabled": request.PriceEnabled, "pricing_applied_models": 0}
	if scheduleID != "" {
		task.Result["schedule_id"] = scheduleID
	}
	if err := s.store.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	queued := task
	// The response is serialized while the worker updates its own result map.
	queued.Result = maps.Clone(task.Result)
	if err := s.dispatchOwned(owner, task); err != nil {
		task.Status = "failed"
		task.Message = "任务启动失败：" + err.Error()
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = s.store.Save(context.WithoutCancel(ctx), task)
		return taskstore.Task{}, err
	}
	return queued, nil
}

func (s *Service) ResumeTask(ctx context.Context, task taskstore.Task) error {
	if task.Operation != Operation {
		return errors.New("此账号模型任务不支持自动恢复")
	}
	var input recoveryInput
	if err := taskstore.RecoveryInput(task, &input); err != nil {
		return err
	}
	input.Request = normalizeRequest(input.Request)
	if err := validateRequest(input.Request); err != nil {
		return err
	}
	if allowed, err := s.allowOwnedTask(ctx, task); err != nil || !allowed {
		return err
	}
	if !input.Request.DryRun {
		return errors.New("远端写入可能已经发生，请核对渠道与价格后重新预览")
	}
	return taskrecovery.Enqueue(s.runner, s.store, task, nil, func(run context.Context, current taskstore.Task) {
		s.runStored(run, current)
	})
}

func (s *Service) runStored(ctx context.Context, task taskstore.Task) {
	var input recoveryInput
	if err := taskstore.RecoveryInput(task, &input); err != nil {
		task.Status = "failed"
		task.Message = "任务恢复参数读取失败：" + err.Error()
		task.Result["error"] = task.Message
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistFinal(s.store, task)
		return
	}
	s.run(ctx, task, normalizeRequest(input.Request))
}

func (s *Service) run(ctx context.Context, task taskstore.Task, request Request) {
	if taskstore.MarkCancelled(ctx, &task, "任务在启动前已取消") {
		taskstore.PersistFinal(s.store, task)
		return
	}
	if allowed, err := s.allowOwnedTask(ctx, task); err != nil || !allowed {
		if err != nil {
			task.Status, task.Message = "failed", err.Error()
			task.Result["error"] = err.Error()
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			taskstore.PersistFinal(s.store, task)
		}
		return
	}
	task.Status = "running"
	task.Result["phase"] = "discovering"
	if !s.persist(ctx, task) {
		return
	}
	if s.groups == nil {
		s.finish(ctx, &task, "failed", "分组目录服务尚未就绪", nil)
		return
	}
	accountIDs, err := s.groups.AccountIDsByGroups(ctx, request.GroupIDs)
	if err == nil {
		accountIDs, err = s.managedAccountIDs(ctx, accountIDs)
	}
	if err != nil {
		s.finish(ctx, &task, "failed", "分组账号读取失败："+err.Error(), nil)
		return
	}
	if len(accountIDs) == 0 {
		task.Result["account_ids"], task.Result["account_count"] = []string{}, 0
		s.finish(ctx, &task, "succeeded", "所选分组没有参与管控的账号，本次无需执行", []ResultItem{})
		return
	}
	if !request.DryRun {
		preview, err := s.store.Get(ctx, request.PreviewTaskID)
		matches := false
		if err == nil {
			matches, err = s.sameManagedAccountIDs(ctx, preview.Result["account_ids"], accountIDs)
		}
		if err != nil || !matches {
			s.finish(ctx, &task, "failed", "账号范围与预览不一致，请重新预览", nil)
			return
		}
	}
	task.Result["group_ids"] = append([]string{}, request.GroupIDs...)
	task.Result["account_ids"] = append([]string{}, accountIDs...)
	task.Result["account_count"] = len(accountIDs)
	modelSet := map[string]string{}
	items := []ResultItem{}
	failed := 0
	for _, accountID := range accountIDs {
		if ctx.Err() != nil {
			break
		}
		var models []string
		var err error
		if checkErr := business.CheckAccountUpstreamManaged(ctx, s.groups, accountID); checkErr != nil {
			if errors.Is(checkErr, business.ErrUpstreamExcluded) {
				continue
			}
			s.finish(ctx, &task, "failed", "账号管控范围读取失败："+checkErr.Error(), items)
			return
		}
		if request.DryRun {
			models, err = s.accounts.PreviewModels(ctx, accountID)
		} else {
			models, err = s.accounts.Models(ctx, accountID)
		}
		if errors.Is(err, business.ErrUpstreamExcluded) {
			continue
		}
		item := ResultItem{AccountID: accountID, Status: "succeeded", Models: append([]string{}, models...)}
		item.Message = fmt.Sprintf("已刷新模型目录，共 %d 个模型", len(models))
		if request.DryRun {
			item.Message = fmt.Sprintf("已读取模型目录，共 %d 个模型；尚未同步或上架", len(models))
		}
		if err != nil {
			item.Status, item.Message = "failed", err.Error()
			failed++
		} else {
			for _, model := range models {
				model = strings.TrimSpace(model)
				if model != "" {
					modelSet[strings.ToLower(model)] = model
				}
			}
		}
		items = append(items, item)
		task.Result["items"] = items
		task.Progress = len(items) * 35 / len(accountIDs)
		task.Message = fmt.Sprintf("正在刷新账号模型目录（%d/%d）", len(items), len(accountIDs))
		if request.DryRun {
			task.Message = fmt.Sprintf("正在读取账号模型目录（%d/%d）", len(items), len(accountIDs))
		}
		if !s.persist(ctx, task) {
			return
		}
	}
	participating := make([]string, 0, len(items))
	for _, item := range items {
		participating = append(participating, item.AccountID)
	}
	task.Result["account_ids"], task.Result["account_count"] = participating, len(participating)
	models := make([]string, 0, len(modelSet))
	for _, model := range modelSet {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return strings.ToLower(models[i]) < strings.ToLower(models[j]) })
	task.Result["models"] = models
	if ctx.Err() != nil {
		s.finish(ctx, &task, "cancelled", "任务已取消", items)
		return
	}
	if len(items) == 0 {
		s.finish(ctx, &task, "succeeded", "账号均已排除管控，本次无需执行", items)
		return
	}
	if len(models) == 0 && failed > 0 {
		s.finish(ctx, &task, "failed", "账号模型目录读取失败，请查看明细后重试", items)
		return
	}
	if failed > 0 && !request.DryRun {
		s.finish(ctx, &task, "failed", "部分账号模型同步失败，请重新预览", items)
		return
	}
	selectedModels := []string{}
	skippedModels := []string{}
	if request.PublishMissing {
		for _, selected := range request.SelectedModels {
			if _, exists := modelSet[modelKey(selected)]; !exists {
				skippedModels = append(skippedModels, selected)
				continue
			}
			selectedModels = append(selectedModels, selected)
		}
	}
	task.Result["skipped_models"] = skippedModels
	var channels newapimanagement.ChannelPage
	if len(request.Channels) > 0 {
		channels, err = s.manager.ChannelsByID(ctx, request.PlatformID, channelIDs(request.Channels), 0, len(request.Channels))
		if err != nil {
			s.finish(ctx, &task, "failed", "渠道读取失败："+err.Error(), items)
			return
		}
	}
	if _, err := channelTargets(request.Channels, channels.Items); err != nil {
		s.finish(ctx, &task, "failed", err.Error(), items)
		return
	}
	expectedVersions := make(map[string]string, len(request.Channels))
	for _, target := range request.Channels {
		expectedVersions[target.ID] = target.Version
	}
	plannedModelSet := map[string]bool{}
	for _, channel := range channels.Items {
		if expectedVersions[channel.ID] != channel.Version {
			continue
		}
		current := map[string]bool{}
		for _, model := range channel.Models {
			current[modelKey(model)] = true
		}
		for _, model := range selectedModels {
			if !current[modelKey(model)] && request.PublishMissing {
				plannedModelSet[modelKey(model)] = true
			}
		}
	}
	plannedModels := make([]string, 0, len(plannedModelSet))
	for _, model := range selectedModels {
		if plannedModelSet[modelKey(model)] {
			plannedModels = append(plannedModels, model)
		}
	}
	var plannedPrices []newapimanagement.ModelPriceInput
	if request.PriceEnabled && len(plannedModels) > 0 {
		catalog, catalogErr := s.manager.ModelPriceCatalog(ctx, request.PlatformID, request.PricingSource == "official")
		if catalogErr != nil {
			s.finish(ctx, &task, "failed", "模型价格目录读取失败："+catalogErr.Error(), items)
			return
		}
		plannedPrices = BuildPriceInputs(plannedModels, catalog.Models)
		digest := pricePlanDigest(plannedPrices)
		task.Result["price_plan_digest"] = digest
		task.Result["pricing_plan"] = plannedPrices
		task.Result["pricing_missing"] = len(plannedModels) - len(plannedPrices)
		if !request.DryRun {
			preview, err := s.store.Get(ctx, request.PreviewTaskID)
			if err != nil || !matchesPreviewPrices(preview, plannedPrices, plannedModels) {
				s.finish(ctx, &task, "failed", "模型价格与预览不一致，请重新预览", items)
				return
			}
		}
	}
	task.Result["phase"] = "publishing"
	if !s.persist(ctx, task) {
		return
	}
	verifiedModels := map[string]bool{}
	for index, channel := range channels.Items {
		if ctx.Err() != nil {
			break
		}
		if expectedVersions[channel.ID] != channel.Version {
			failed++
			items = append(items, ResultItem{ChannelID: channel.ID, ChannelName: channel.Name, Status: "failed", Message: "渠道配置已变化，请刷新后重试"})
			task.Result["items"] = items
			if !s.persist(ctx, task) {
				return
			}
			continue
		}
		current := map[string]bool{}
		for _, model := range channel.Models {
			current[modelKey(model)] = true
		}
		missing := make([]string, 0)
		item := ResultItem{ChannelID: channel.ID, ChannelName: channel.Name, Status: "succeeded", Message: "渠道模型已核对"}
		for _, model := range selectedModels {
			if !current[modelKey(model)] {
				missing = append(missing, model)
			} else {
				item.Existing = append(item.Existing, model)
			}
		}
		item.Planned = missing
		if len(missing) > 0 && request.PublishMissing && !request.DryRun {
			task.Result["phase"] = "publishing"
			task.Result["pending_channel_id"] = channel.ID
			if !s.persist(ctx, task) {
				return
			}
			var updated newapimanagement.Channel
			updated, err = s.manager.ChangeChannelModels(ctx, request.PlatformID, channel.ID, newapimanagement.ChannelModelChange{Action: "add", Models: missing, Version: channel.Version})
			if err != nil {
				item.Status, item.Message = "failed", err.Error()
			} else {
				savedModels := map[string]bool{}
				for _, saved := range updated.Models {
					savedModels[modelKey(saved)] = true
				}
				for _, model := range missing {
					if savedModels[modelKey(model)] {
						item.Added = append(item.Added, model)
					}
				}
				if len(item.Added) != len(missing) {
					item.Status, item.Message = "failed", "上架结果未完整确认，请核对渠道模型后重试"
				} else {
					item.Message = fmt.Sprintf("已上架 %d 个所选模型", len(item.Added))
				}
				channel = updated
			}
		} else if len(missing) > 0 && request.DryRun {
			item.Message = fmt.Sprintf("待上架 %d 个所选模型，尚未写入", len(missing))
		} else if !request.PublishMissing {
			item.Status, item.Message = "skipped", "未启用自动上架"
		}
		if item.Status == "failed" {
			failed++
		} else if item.Status == "succeeded" && !request.DryRun {
			for _, model := range missing {
				for _, saved := range channel.Models {
					if modelKey(saved) == modelKey(model) {
						verifiedModels[modelKey(model)] = true
					}
				}
			}
		}
		items = append(items, item)
		task.Result["items"] = items
		delete(task.Result, "pending_channel_id")
		task.Progress = 40 + (index+1)*35/len(channels.Items)
		if !s.persist(ctx, task) {
			return
		}
	}
	if request.PriceEnabled && len(plannedModels) > 0 && ctx.Err() == nil {
		task.Result["phase"] = "pricing"
		if !s.persist(ctx, task) {
			return
		}
		priceModels := plannedModels
		if !request.DryRun {
			priceModels = []string{}
			for _, model := range selectedModels {
				if verifiedModels[modelKey(model)] {
					priceModels = append(priceModels, model)
				}
			}
		}
		prices := filterPrices(plannedPrices, priceModels)
		task.Result["pricing_models"] = len(prices)
		if len(prices) > 0 && !request.DryRun {
			task.Result["pending_pricing"] = true
			if !s.persist(ctx, task) {
				return
			}
			if _, err = s.manager.SaveModelPrices(ctx, request.PlatformID, prices); err != nil {
				failed++
				task.Result["pricing_error"] = err.Error()
			} else {
				task.Result["pricing_applied_models"] = len(prices)
			}
			delete(task.Result, "pending_pricing")
		}
		if len(prices) < len(priceModels) {
			task.Result["pricing_missing"] = len(priceModels) - len(prices)
		}
	}
	plannedCount, addedCount := 0, 0
	for _, item := range items {
		plannedCount += len(item.Planned)
		addedCount += len(item.Added)
	}
	status := "succeeded"
	message := fmt.Sprintf("执行完成：已刷新账号模型目录，渠道共新增 %d 项模型；价格已同步 %v 个模型", addedCount, task.Result["pricing_applied_models"])
	if request.DryRun {
		message = fmt.Sprintf("预览完成：读取到 %d 个模型，渠道待上架 %d 项；未写入任何配置", len(models), plannedCount)
	}
	if len(skippedModels) > 0 {
		message += "；已跳过当前账号目录中不存在的所选模型：" + strings.Join(skippedModels, "、")
	}
	if failed > 0 {
		status = "partial"
		message += fmt.Sprintf("；有 %d 项失败，请查看明细", failed)
	}
	if ctx.Err() != nil {
		status, message = "cancelled", "任务已取消"
	}
	s.finish(ctx, &task, status, message, items)
}

func sameAccountIDs(saved any, current []string) bool {
	var ids []string
	switch values := saved.(type) {
	case []string:
		ids = append(ids, values...)
	case []any:
		for _, value := range values {
			id, ok := value.(string)
			if !ok {
				return false
			}
			ids = append(ids, id)
		}
	default:
		return false
	}
	if len(ids) != len(current) {
		return false
	}
	other := append([]string(nil), current...)
	slices.Sort(ids)
	slices.Sort(other)
	return slices.Equal(ids, other)
}

func (s *Service) finish(ctx context.Context, task *taskstore.Task, status, message string, items []ResultItem) {
	if taskstore.Interrupted(ctx) {
		return
	}
	task.Status, task.Message = status, message
	task.Result["phase"] = "complete"
	task.Result["items"] = items
	task.Progress = 100
	taskstore.MarkCancelled(ctx, task, message)
	_ = s.persist(ctx, *task)
}

func (s *Service) persist(ctx context.Context, task taskstore.Task) bool {
	copy := task
	copy.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.store.Save(context.WithoutCancel(ctx), copy); err != nil {
		return false
	}
	return true
}

func channelIDs(channels []ChannelTarget) []string {
	ids := make([]string, len(channels))
	for i := range channels {
		ids[i] = channels[i].ID
	}
	return ids
}

func BuildPriceInputs(models []string, catalog []newapimanagement.Sub2APIModelPrice) []newapimanagement.ModelPriceInput {
	byModel := make(map[string]newapimanagement.Sub2APIModelPrice, len(catalog))
	for _, item := range catalog {
		byModel[modelKey(item.Model)] = item
	}
	result := make([]newapimanagement.ModelPriceInput, 0, len(models))
	for _, model := range models {
		item, ok := byModel[modelKey(model)]
		if !ok || strings.TrimSpace(item.ModelRatio) == "" || strings.TrimSpace(item.CompletionRatio) == "" || item.SyncError != "" {
			continue
		}
		billingMode := ""
		if strings.TrimSpace(item.BillingExpr) != "" {
			billingMode = "tiered_expr"
		}
		result = append(result, newapimanagement.ModelPriceInput{Model: model, InputRatio: item.ModelRatio, CompletionRatio: item.CompletionRatio, BillingMode: billingMode, BillingExpr: item.BillingExpr, CacheRatio: item.CacheRatio, CreateCacheRatio: item.CreateCacheRatio, ImageRatio: item.ImageRatio})
	}
	return result
}

func modelKey(model string) string { return strings.ToLower(strings.TrimSpace(model)) }

func requestScope(request Request) string {
	selected := append([]string{}, request.SelectedModels...)
	sort.Strings(selected)
	groups := append([]string(nil), request.GroupIDs...)
	channels := append([]ChannelTarget(nil), request.Channels...)
	sort.Strings(groups)
	sort.Slice(channels, func(i, j int) bool { return channels[i].ID < channels[j].ID })
	raw, _ := json.Marshal(struct {
		Models   []string
		Groups   []string
		Platform string
		Channels []ChannelTarget
		Publish  bool
		Price    bool
		Source   string
	}{selected, groups, request.PlatformID, channels, request.PublishMissing, request.PriceEnabled, request.PricingSource})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// Compare only prices that will actually be written. Missing selected models
// shrink the write scope without invalidating the remaining confirmed prices.
func matchesPreviewPrices(preview taskstore.Task, current []newapimanagement.ModelPriceInput, models []string) bool {
	raw, err := json.Marshal(preview.Result["pricing_plan"])
	if err != nil {
		return false
	}
	var prices []newapimanagement.ModelPriceInput
	if err := json.Unmarshal(raw, &prices); err != nil {
		return false
	}
	if preview.Result["price_plan_digest"] != pricePlanDigest(prices) {
		return false
	}
	return pricePlanDigest(filterPrices(prices, models)) == pricePlanDigest(current)
}

func pricePlanDigest(prices []newapimanagement.ModelPriceInput) string {
	raw, _ := json.Marshal(prices)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func filterPrices(prices []newapimanagement.ModelPriceInput, models []string) []newapimanagement.ModelPriceInput {
	allowed := make(map[string]bool, len(models))
	for _, model := range models {
		allowed[strings.ToLower(model)] = true
	}
	selected := make([]newapimanagement.ModelPriceInput, 0, len(prices))
	for _, price := range prices {
		if allowed[strings.ToLower(price.Model)] {
			selected = append(selected, price)
		}
	}
	return selected
}
