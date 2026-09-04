package inspection

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/alerting"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type RunnerRepository interface {
	ControlPolicy(context.Context) (map[string]any, error)
	Mode(context.Context) (string, error)
	AlertPolicy(context.Context) (business.AlertPolicy, error)
	InspectionTaskDue(context.Context, string, int, time.Time) (bool, error)
	MarkInspectionTask(context.Context, string, time.Time) error
	ResetInspectionTask(context.Context, string, time.Time) error
	RoutingWritebackPending(context.Context) (bool, error)
}

type EvidenceCollector interface {
	Plan(context.Context, map[string]any, *string, *string, time.Time) (evidence.Plan, error)
	Collect(context.Context, map[string]any, evidence.Admin, evidence.Options) (evidence.Result, error)
}

type Router interface {
	Calculate(context.Context, routing.Scope, bool) (routing.Result, error)
}

type RoutingWriter interface {
	Apply(context.Context, map[string]business.AccountRoutingTarget, string) (routingwrite.Result, error)
}

type AlertEvaluator interface {
	Evaluate(context.Context) (alerting.Result, error)
}

type UpstreamSynchronizer interface {
	SyncAllNow(context.Context, upstreamsync.Scope, string) (upstreamsync.BatchResult, error)
}

type AccountRateSynchronizer interface {
	SyncAllAccountRates(context.Context, string) (map[string]any, error)
}

type AccountRateSynchronizerWithCatalog interface {
	SyncAllAccountRatesWithCatalog(context.Context, string, map[string]business.UpstreamCatalogSnapshot) (map[string]any, error)
}

type AccountRateSyncScheduler interface {
	EnqueueAccountRateSyncBatch(context.Context, int, int, string) (string, error)
}

type PriceAllocator interface {
	ApplyNow(context.Context, string) (pricing.Result, error)
}

type InvalidAuthRecoverer interface {
	RecoverInvalid(context.Context, []string, string) (business.AuthRecoverySummary, error)
}

type InvalidAuthHostSource interface {
	AuthRecoveryRequiredHosts(context.Context, time.Time) ([]string, error)
}

type InspectionTaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type InspectionTargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type RunRequest struct {
	AccountID  *string
	GroupName  *string
	Actor      string
	Automatic  bool
	AutoConfig *business.AutoInspectionConfig
}

type Runner struct {
	repository    RunnerRepository
	targets       InspectionTargetStore
	evidence      EvidenceCollector
	router        Router
	writer        RoutingWriter
	alerts        AlertEvaluator
	upstreams     UpstreamSynchronizer
	accountRates  AccountRateSynchronizer
	rateScheduler AccountRateSyncScheduler
	pricing       PriceAllocator
	authRecovery  InvalidAuthRecoverer
	tasks         InspectionTaskStore
	now           func() time.Time
}

type duePlan struct {
	traffic                 bool
	probes                  bool
	upstreams               bool
	alert                   bool
	routing                 bool
	accountRates            bool
	pricing                 bool
	policy                  map[string]any
	mode                    string
	probeIDs                []string
	accountRateInterval     int
	accountRateBatchSize    int
	accountRateBatchPercent int
	authRecoveryHosts       []string
}

type upstreamSyncOutcome struct {
	batch     upstreamsync.BatchResult
	err       error
	startedAt time.Time
	duration  time.Duration
}

type evidenceCollectionOutcome struct {
	result    evidence.Result
	err       error
	startedAt time.Time
	duration  time.Duration
}

const (
	operationUpstreamSync       = "upstream_sync"
	operationAuthRecovery       = "auth_recovery"
	operationAccountRateSync    = "account_rate_sync"
	operationPriceManagement    = "price_management"
	operationTrafficRefresh     = "traffic_refresh"
	operationActiveProbe        = "active_probe"
	operationRoutingCalculation = "routing_calculation"
	operationRoutingWriteback   = "routing_writeback"
	operationAlertEvaluation    = "alert_evaluation"
)

const authRecoveryRetryInterval = 5 * time.Minute

func NewRunner(
	repository RunnerRepository,
	targets InspectionTargetStore,
	evidenceCollector EvidenceCollector,
	router Router,
	writer RoutingWriter,
	alerts AlertEvaluator,
	upstreams UpstreamSynchronizer,
	tasks InspectionTaskStore,
	extensions ...any,
) *Runner {
	runner := &Runner{
		repository: repository, targets: targets, evidence: evidenceCollector, router: router,
		writer: writer, alerts: alerts, upstreams: upstreams, tasks: tasks, now: time.Now,
	}
	for _, extension := range extensions {
		if value, ok := extension.(AccountRateSynchronizer); ok {
			runner.accountRates = value
		}
		if value, ok := extension.(AccountRateSyncScheduler); ok {
			runner.rateScheduler = value
		}
		if value, ok := extension.(PriceAllocator); ok {
			runner.pricing = value
		}
		if value, ok := extension.(InvalidAuthRecoverer); ok {
			runner.authRecovery = value
		}
	}
	return runner
}

func (r *Runner) Execute(ctx context.Context, config business.AutoInspectionConfig) (ExecutionResult, error) {
	return r.Run(ctx, RunRequest{Actor: "自动巡检", Automatic: true, AutoConfig: &config})
}

func (r *Runner) Preview(ctx context.Context, now time.Time) (QueueItem, error) {
	config, err := r.repositoryAutoInspectionConfig(ctx)
	if err != nil {
		return QueueItem{}, err
	}
	plan, err := r.plan(ctx, RunRequest{Automatic: true, AutoConfig: &config}, now.UTC())
	if err != nil {
		return QueueItem{}, err
	}
	item := QueueItem{
		TaskType: "inspection", Label: "下一轮巡检计划", State: "waiting",
		Detail: "当前没有操作到期，下次心跳会重新检查", Operations: []QueueOperation{},
	}
	operations, err := previewOperations(plan)
	if err != nil {
		return QueueItem{}, err
	}
	item.Operations = operations
	dueCount := 0
	dueLabels := make([]string, 0, len(operations))
	for _, operation := range operations {
		if operation.Due {
			dueCount++
			dueLabels = append(dueLabels, operation.Label)
		}
		if operation.Operation == "active_probe" && operation.TargetCount != nil {
			item.TargetCount = operation.TargetCount
		}
	}
	if dueCount == 0 {
		return item, nil
	}
	item.State = "ready"
	item.Label = fmt.Sprintf("本轮巡检（%d 项）", dueCount)
	item.Detail = "包含到期操作：" + strings.Join(dueLabels, "、")
	return item, nil
}

func previewOperations(plan duePlan) ([]QueueOperation, error) {
	operations := make([]QueueOperation, 0, 6)
	capabilities, valid := runtimepolicy.For(plan.mode)
	if !valid {
		return nil, fmt.Errorf("运行模式无效：%s", plan.mode)
	}
	if capabilities.AutomaticUpstreamSync {
		section, err := inspectionSection(plan.policy, "upstream_multiplier")
		if err != nil {
			return nil, err
		}
		interval, err := boundedSetting(section, "interval_seconds", 120, 30)
		if err != nil {
			return nil, err
		}
		operations = append(operations, QueueOperation{
			Operation: operationUpstreamSync, Label: "上游数据同步",
			Cycle: periodicCycle(interval), Due: plan.upstreams,
		})
	}
	if plan.accountRates {
		cycle := "每2分钟"
		if plan.accountRateInterval > 0 {
			cycle = periodicCycle(plan.accountRateInterval)
		}
		if plan.accountRateBatchSize > 0 {
			cycle += fmt.Sprintf("，每轮 %d 个账号", plan.accountRateBatchSize)
		} else if plan.accountRateBatchPercent > 0 {
			cycle += fmt.Sprintf("，每轮 %d%% 账号", plan.accountRateBatchPercent)
		} else {
			cycle += "，每轮全量账号"
		}
		var targetCount *int
		if plan.accountRateBatchSize > 0 {
			target := plan.accountRateBatchSize
			targetCount = &target
		}
		operations = append(operations, QueueOperation{
			Operation: operationAccountRateSync, Label: "账号倍率与名称同步",
			TargetCount: targetCount, Cycle: cycle, Due: true,
		})
	}
	if len(plan.authRecoveryHosts) > 0 {
		targets := len(plan.authRecoveryHosts)
		operations = append(operations, QueueOperation{
			Operation: operationAuthRecovery, Label: "鉴权自动恢复", TargetCount: &targets,
			Cycle: "发现鉴权失效后尝试，失败后每5分钟重试", Due: true,
		})
	}
	if plan.pricing {
		section, err := inspectionSection(plan.policy, "price_management")
		if err != nil {
			return nil, err
		}
		interval, err := boundedSetting(section, "interval_seconds", 120, 30)
		if err != nil {
			return nil, err
		}
		operations = append(operations, QueueOperation{
			Operation: operationPriceManagement, Label: "价格分组调整",
			Cycle: periodicCycle(interval), Due: true,
		})
	}
	traffic, err := inspectionSection(plan.policy, "traffic")
	if err != nil {
		return nil, err
	}
	trafficEnabled, err := trafficSourceEnabled(plan.policy)
	if err != nil {
		return nil, err
	}
	if trafficEnabled {
		interval, intervalErr := positiveSetting(traffic, "refresh_seconds", 60)
		if intervalErr != nil {
			return nil, intervalErr
		}
		operations = append(operations, QueueOperation{
			Operation: operationTrafficRefresh, Label: "真实流量同步",
			Cycle: periodicCycle(interval), Due: plan.traffic,
		})
	}
	probe, err := inspectionSection(plan.policy, "probe")
	if err != nil {
		return nil, err
	}
	probeEnabled, err := boolSetting(probe, "enabled", true)
	if err != nil {
		return nil, err
	}
	recovery, err := inspectionSection(plan.policy, "recovery")
	if err != nil {
		return nil, err
	}
	recoveryEnabled, err := boolSetting(recovery, "enabled", true)
	if err != nil {
		return nil, err
	}
	if capabilities.AutomaticActiveProbe && (probeEnabled || recoveryEnabled) {
		probeInterval, intervalErr := boundedSetting(probe, "interval_seconds", 300, 30)
		if intervalErr != nil {
			return nil, intervalErr
		}
		cycle := "按账号策略（常规" + periodicCycle(probeInterval) + "）"
		if !probeEnabled {
			cycle = "按账号策略（仅回池探测）"
		}
		if recoveryEnabled {
			recoveryInterval, intervalErr := positiveSetting(recovery, "probe_interval_seconds", 180)
			if intervalErr != nil {
				return nil, intervalErr
			}
			if probeEnabled {
				cycle = "按账号策略（常规" + periodicCycle(probeInterval) + "；回池" + periodicCycle(recoveryInterval) + "）"
			} else {
				cycle = "按账号策略（回池" + periodicCycle(recoveryInterval) + "）"
			}
		}
		var targets *int
		if plan.probes {
			count := len(plan.probeIDs)
			targets = &count
		}
		operations = append(operations, QueueOperation{
			Operation: operationActiveProbe, Label: "主动探测", TargetCount: targets,
			Cycle: cycle, Due: plan.probes,
		})
	}
	due := plan.upstreams || plan.traffic || plan.probes || plan.routing
	operations = append(operations, QueueOperation{
		Operation: operationRoutingCalculation, Label: "调度计算", Cycle: "有采集、同步或待写回目标时", Due: due,
	})
	if capabilities.AutomaticRemoteApply {
		operations = append(operations, QueueOperation{
			Operation: operationRoutingWriteback, Label: "自动执行", Cycle: "调度计算完成且目标发生变化时", Due: due,
		})
	}
	if plan.alert {
		operations = append(operations, QueueOperation{
			Operation: operationAlertEvaluation, Label: "告警检测", Cycle: "每次自动巡检心跳", Due: true,
		})
	}
	return operations, nil
}

func dueQueueOperations(plan duePlan) ([]QueueOperation, error) {
	operations, err := previewOperations(plan)
	if err != nil {
		return nil, err
	}
	result := make([]QueueOperation, 0, len(operations))
	for _, operation := range operations {
		if operation.Due {
			result = append(result, operation)
		}
	}
	return result, nil
}

func collectionStageMessage(upstreams, traffic, probes bool) string {
	switch {
	case upstreams && traffic && probes:
		return "正在并行同步上游数据、读取真实请求记录并执行主动探测"
	case upstreams && traffic:
		return "正在并行同步上游数据并读取真实请求记录"
	case upstreams && probes:
		return "正在并行同步上游数据并执行主动探测"
	case traffic && probes:
		return "正在读取真实请求记录并执行主动探测"
	case upstreams:
		return "正在同步上游数据"
	case traffic:
		return "正在读取真实请求记录"
	case probes:
		return "正在执行主动探测"
	default:
		return "正在准备本轮巡检任务"
	}
}

func periodicCycle(seconds int) string {
	if seconds%3600 == 0 {
		return fmt.Sprintf("每%d小时", seconds/3600)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("每%d分钟", seconds/60)
	}
	return fmt.Sprintf("每%d秒", seconds)
}

func (r *Runner) MonitoringConfigured(ctx context.Context) (bool, error) {
	policy, err := r.repository.ControlPolicy(ctx)
	if err != nil {
		return false, err
	}
	return trafficSourceEnabled(policy)
}

func (r *Runner) Run(ctx context.Context, request RunRequest) (ExecutionResult, error) {
	if strings.TrimSpace(request.Actor) == "" {
		request.Actor = "控制台"
	}
	now := r.now().UTC()
	plan, err := r.plan(ctx, request, now)
	if err != nil {
		return ExecutionResult{}, err
	}
	if !plan.traffic && !plan.probes && !plan.upstreams && !plan.routing && !plan.alert && !plan.pricing && !plan.accountRates && len(plan.authRecoveryHosts) == 0 {
		return ExecutionResult{Status: "succeeded", Operations: []string{}, OperationTiming: []business.OperationTiming{}, Skipped: true}, nil
	}
	task, err := r.QueueTask(ctx, request.Automatic)
	if err != nil {
		return ExecutionResult{}, err
	}
	reportExecutionTask(ctx, task.ID)
	result := r.executeTask(ctx, task, request, plan, now)
	return result, nil
}

func (r *Runner) QueueTask(ctx context.Context, automatic bool) (taskstore.Task, error) {
	task, err := newInspectionTask(r.now().UTC(), automatic)
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := r.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	return task, nil
}

func (r *Runner) RunTask(ctx context.Context, task taskstore.Task, request RunRequest) ExecutionResult {
	reportExecutionTask(ctx, task.ID)
	now := r.now().UTC()
	plan, err := r.plan(ctx, request, now)
	if err != nil {
		return r.failQueuedTask(ctx, task, err)
	}
	return r.executeTask(ctx, task, request, plan, now)
}

func (r *Runner) failQueuedTask(ctx context.Context, task taskstore.Task, err error) ExecutionResult {
	if cause := taskstore.ContextFailureCause(ctx); cause != nil {
		err = cause
	}
	task.Status, task.Progress, task.Message, task.UpdatedAt = "failed", 100, "巡检启动失败："+err.Error(), r.now().UTC().Format(time.RFC3339Nano)
	task.Result = map[string]any{"error": err.Error(), "remote_write": false}
	cancelled := taskstore.MarkCancelled(ctx, &task, "巡检已取消")
	taskstore.PersistFinal(r.tasks, task)
	if cancelled {
		return ExecutionResult{TaskID: &task.ID, Status: "cancelled", Operations: []string{}, OperationTiming: []business.OperationTiming{}}
	}
	return failedExecution(&task.ID, nil, nil, err)
}

func (r *Runner) plan(ctx context.Context, request RunRequest, now time.Time) (duePlan, error) {
	policy, err := r.repository.ControlPolicy(ctx)
	if err != nil {
		return duePlan{}, err
	}
	mode, err := r.repository.Mode(ctx)
	if err != nil {
		return duePlan{}, err
	}
	capabilities, valid := runtimepolicy.For(mode)
	if !valid {
		return duePlan{}, fmt.Errorf("运行模式无效：%s", mode)
	}
	evidencePlan, err := r.evidence.Plan(ctx, policy, request.AccountID, request.GroupName, now)
	if err != nil {
		return duePlan{}, err
	}
	alertPolicy, err := r.repository.AlertPolicy(ctx)
	if err != nil {
		return duePlan{}, err
	}
	result := duePlan{policy: policy, mode: mode, alert: alertPolicy.Enabled}
	scopedDiagnostic := request.AccountID != nil || request.GroupName != nil
	if !request.Automatic && scopedDiagnostic {
		result.probeIDs = evidencePlan.ProbeAccountIDs
		result.traffic, result.probes = evidencePlan.RequestedSource == "traffic", len(evidencePlan.ProbeAccountIDs) > 0
		return result, nil
	}
	if request.Automatic && r.authRecovery != nil {
		if source, ok := r.repository.(InvalidAuthHostSource); ok {
			result.authRecoveryHosts, err = source.AuthRecoveryRequiredHosts(ctx, now.Add(-authRecoveryRetryInterval))
			if err != nil {
				return duePlan{}, err
			}
		}
	}
	traffic, err := inspectionSection(policy, "traffic")
	if err != nil {
		return duePlan{}, err
	}
	refreshSeconds, err := positiveSetting(traffic, "refresh_seconds", 60)
	if err != nil {
		return duePlan{}, err
	}
	if evidencePlan.RequestedSource == "traffic" {
		result.traffic, err = r.repository.InspectionTaskDue(ctx, "traffic", refreshSeconds, now)
		if err != nil {
			return duePlan{}, err
		}
	}
	if capabilities.AutomaticActiveProbe {
		result.probeIDs = evidencePlan.ProbeAccountIDs
		result.probes = len(evidencePlan.ProbeAccountIDs) > 0
	}
	if capabilities.AutomaticUpstreamSync {
		multiplier, sectionErr := inspectionSection(policy, "upstream_multiplier")
		if sectionErr != nil {
			return duePlan{}, sectionErr
		}
		interval, intervalErr := boundedSetting(multiplier, "interval_seconds", 120, 30)
		if intervalErr != nil {
			return duePlan{}, intervalErr
		}
		result.upstreams, err = r.repository.InspectionTaskDue(ctx, "upstream-sync", interval, now)
		if err != nil {
			return duePlan{}, err
		}
	}
	if capabilities.AutomaticRemoteApply {
		result.routing, err = r.repository.RoutingWritebackPending(ctx)
		if err != nil {
			return duePlan{}, err
		}
		if r.accountRates != nil || r.rateScheduler != nil {
			interval := 120
			batchSize, batchPercent := 0, 0
			if configured, ok := accountRateSyncPolicy(result.policy); ok {
				interval, batchSize, batchPercent = configured.interval, configured.batchSize, configured.percent
			} else if request.AutoConfig != nil {
				if request.AutoConfig.AccountRateSyncIntervalSeconds > 0 {
					interval = request.AutoConfig.AccountRateSyncIntervalSeconds
				}
				batchSize, batchPercent = request.AutoConfig.AccountRateSyncBatchSize, request.AutoConfig.AccountRateSyncBatchPercent
			}
			result.accountRateInterval, result.accountRateBatchSize, result.accountRateBatchPercent = interval, batchSize, batchPercent
			if request.AutoConfig == nil {
				result.accountRates = result.upstreams
			} else {
				result.accountRates, err = r.repository.InspectionTaskDue(ctx, "account-rate-sync", interval, now)
				if err != nil {
					return duePlan{}, err
				}
			}
		}
		priceConfig, configErr := pricing.ConfigFromPolicy(policy)
		if configErr != nil {
			return duePlan{}, configErr
		}
		if priceConfig.Enabled && r.pricing != nil {
			result.pricing, err = r.repository.InspectionTaskDue(ctx, "price-management", priceConfig.IntervalSeconds, now)
			if err != nil {
				return duePlan{}, err
			}
		}
	}
	return result, nil
}

type accountRateSyncPolicySettings struct {
	interval  int
	batchSize int
	percent   int
}

func accountRateSyncPolicy(policy map[string]any) (accountRateSyncPolicySettings, bool) {
	raw, ok := policy["account_rate_sync"]
	section, ok := raw.(map[string]any)
	if !ok {
		return accountRateSyncPolicySettings{}, false
	}
	read := func(key string, fallback int) int {
		switch value := section[key].(type) {
		case int64:
			return int(value)
		case int:
			return value
		case float64:
			return int(value)
		default:
			return fallback
		}
	}
	return accountRateSyncPolicySettings{interval: read("interval_seconds", 120), batchSize: read("batch_size", 0), percent: read("batch_percent", 0)}, true
}

func (r *Runner) repositoryAutoInspectionConfig(ctx context.Context) (business.AutoInspectionConfig, error) {
	reader, ok := r.repository.(interface {
		AutoInspectionConfig(context.Context) (business.AutoInspectionConfig, error)
	})
	if !ok {
		return business.DefaultAutoInspectionConfig(), nil
	}
	return reader.AutoInspectionConfig(ctx)
}

func (r *Runner) enqueueAccountRateSync(
	ctx context.Context,
	request RunRequest,
	plan duePlan,
	resultPayload map[string]any,
	operations *[]string,
	failures *[]string,
	partialFailures *[]string,
) {
	if !plan.accountRates {
		return
	}
	*operations = append(*operations, operationAccountRateSync)
	if r.rateScheduler != nil {
		batchSize, batchPercent := 0, 0
		if request.AutoConfig != nil {
			batchSize = request.AutoConfig.AccountRateSyncBatchSize
			batchPercent = request.AutoConfig.AccountRateSyncBatchPercent
		}
		taskID, err := r.rateScheduler.EnqueueAccountRateSyncBatch(ctx, batchSize, batchPercent, request.Actor)
		if err != nil {
			if errors.Is(err, taskstore.ErrOperationActive) {
				resultPayload["account_rate_sync"] = map[string]any{"queued": false, "already_running": true}
				return
			}
			*failures = append(*failures, "账号倍率与名称同步排队失败："+err.Error())
			return
		}
		resultPayload["account_rate_sync"] = map[string]any{"queued": true, "task_id": taskID, "batch_size": batchSize, "batch_percent": batchPercent}
		return
	}
	if r.accountRates == nil {
		return
	}
	rateResult, err := r.accountRates.SyncAllAccountRates(ctx, request.Actor)
	resultPayload["account_rate_sync"] = rateResult
	if err != nil {
		*failures = append(*failures, "账号倍率与名称同步："+err.Error())
		return
	}
	missing := integerResultValue(rateResult, "missing")
	failed := integerResultValue(rateResult, "failed")
	if missing+failed > 0 {
		*partialFailures = append(*partialFailures, fmt.Sprintf("账号倍率与名称同步部分失败：缺失 %d，失败 %d", missing, failed))
	}
}

func trafficSourceEnabled(policy map[string]any) (bool, error) {
	if policy == nil {
		return false, nil
	}
	traffic, err := inspectionSection(policy, "traffic")
	if err != nil {
		return false, err
	}
	enabled, err := boolSetting(traffic, "enabled", true)
	if err != nil {
		return false, errors.New("traffic.enabled 配置无效")
	}
	return enabled, nil
}

func (r *Runner) executeTask(ctx context.Context, task taskstore.Task, request RunRequest, plan duePlan, started time.Time) ExecutionResult {
	if request.Automatic {
		ctx = mutationguard.WithAutomaticInspection(ctx)
	}
	if r.targets != nil && (plan.traffic || plan.accountRates || plan.pricing || plan.routing) {
		var err error
		ctx, err = targetguard.Capture(ctx, r.targets)
		if err != nil {
			return r.failQueuedTask(ctx, task, err)
		}
	}
	operations := []string{}
	timings := []business.OperationTiming{}
	plannedOperations, err := dueQueueOperations(plan)
	if err != nil {
		return r.failQueuedTask(ctx, task, err)
	}
	resultPayload := map[string]any{
		"planned_operations":   plannedOperations,
		"active_operations":    []string{},
		"completed_operations": []string{},
	}
	failures := []string{}
	partialFailures := []string{}
	summary := InspectionSummary{}
	var monitoringEnabled *bool
	persistStage := func(progress int, message string, active []string) {
		resultPayload["active_operations"] = append([]string{}, active...)
		resultPayload["completed_operations"] = append([]string{}, operations...)
		task.Status, task.Progress, task.Message = "running", progress, message
		task.UpdatedAt, task.Result = r.now().UTC().Format(time.RFC3339Nano), resultPayload
		taskstore.PersistProgress(r.tasks, task)
	}
	finish := func(currentFailures, currentPartialFailures []string) ExecutionResult {
		result := r.finishTask(ctx, task, operations, timings, resultPayload, currentFailures, currentPartialFailures)
		result.MonitoringEnabled = monitoringEnabled
		result.Summary = &summary
		return result
	}
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 5, "正在准备本轮巡检任务", r.now().UTC().Format(time.RFC3339Nano)
	task.Result = resultPayload
	if !taskstore.SaveRunning(ctx, r.tasks, task) {
		if cause := taskstore.ContextFailureCause(ctx); cause != nil {
			return failedExecution(&task.ID, operations, timings, cause)
		}
		if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
			return ExecutionResult{TaskID: &task.ID, Status: "cancelled", Operations: operations, OperationTiming: timings}
		}
		return failedExecution(&task.ID, operations, timings, errors.New("巡检任务启动状态保存失败"))
	}
	if plan.upstreams {
		if err := r.repository.MarkInspectionTask(ctx, "upstream-sync", started); err != nil {
			failures = append(failures, "上游同步到期状态保存失败："+err.Error())
			plan.upstreams = false
		}
	}
	if plan.traffic {
		if err := r.repository.MarkInspectionTask(ctx, "traffic", started); err != nil {
			failures = append(failures, "真实流量同步到期状态保存失败："+err.Error())
			plan.traffic = false
			plan.probes = false
		}
	}
	if plan.pricing {
		if err := r.repository.MarkInspectionTask(ctx, "price-management", started); err != nil {
			failures = append(failures, "价格分组调整到期状态保存失败："+err.Error())
			plan.pricing = false
		}
	}
	if plan.accountRates {
		if err := r.repository.MarkInspectionTask(ctx, "account-rate-sync", started); err != nil {
			failures = append(failures, "账号倍率同步到期状态保存失败："+err.Error())
			plan.accountRates = false
		}
	}
	runInspection := plan.traffic || plan.probes
	collectionOperations := make([]string, 0, 3)
	if plan.upstreams {
		collectionOperations = append(collectionOperations, operationUpstreamSync)
	}
	if plan.traffic {
		collectionOperations = append(collectionOperations, operationTrafficRefresh)
	}
	if plan.probes {
		collectionOperations = append(collectionOperations, operationActiveProbe)
	}
	if len(collectionOperations) > 0 {
		persistStage(35, collectionStageMessage(plan.upstreams, plan.traffic, plan.probes), collectionOperations)
	}
	var upstreamResults chan upstreamSyncOutcome
	if plan.upstreams {
		upstreamResults = make(chan upstreamSyncOutcome, 1)
		upstreamStartedAt := time.Now()
		go func() {
			batch, syncErr := r.upstreams.SyncAllNow(ctx, upstreamsync.Scope{Catalog: true, Balance: true}, request.Actor)
			upstreamResults <- upstreamSyncOutcome{batch: batch, err: syncErr, startedAt: upstreamStartedAt, duration: time.Since(upstreamStartedAt)}
		}()
	}
	var evidenceResults chan evidenceCollectionOutcome
	if runInspection {
		admin := r.evidenceAdmin(ctx)
		evidenceStarted := time.Now()
		evidenceResults = make(chan evidenceCollectionOutcome, 1)
		go func() {
			evidenceResult, collectErr := r.evidence.Collect(ctx, plan.policy, admin, evidence.Options{
				AccountID: request.AccountID, GroupName: request.GroupName, FetchTraffic: plan.traffic,
				StrictFallback: strictEvidenceFallback(request),
				ProbesAllowed:  plan.probes, Now: started,
			})
			evidenceResults <- evidenceCollectionOutcome{result: evidenceResult, err: collectErr, startedAt: evidenceStarted, duration: time.Since(evidenceStarted)}
		}()
	}
	if plan.upstreams {
		outcome := <-upstreamResults
		timings = append(timings, operationTimingDuration(operationUpstreamSync, outcome.startedAt, outcome.duration))
		operations = append(operations, operationUpstreamSync)
		resultPayload["upstream_sync"] = outcome.batch
		if outcome.err != nil {
			failures = append(failures, "上游同步："+outcome.err.Error())
		} else if outcome.batch.Failed > 0 {
			partialFailures = append(partialFailures, fmt.Sprintf("上游同步部分失败：其他 %d", outcome.batch.Failed))
		} else if outcome.batch.AuthFailed > 0 && (!request.Automatic || r.authRecovery == nil) {
			partialFailures = append(partialFailures, fmt.Sprintf("上游同步部分失败：鉴权 %d", outcome.batch.AuthFailed))
		}
		if request.Automatic {
			plan.authRecoveryHosts = uniqueHostList(append(plan.authRecoveryHosts, authenticationFailedHosts(outcome.batch)...))
		}
		if plan.accountRates {
			r.enqueueAccountRateSync(ctx, request, plan, resultPayload, &operations, &failures, &partialFailures)
		}
		if runInspection {
			active := make([]string, 0, 2)
			if plan.traffic {
				active = append(active, operationTrafficRefresh)
			}
			if plan.probes {
				active = append(active, operationActiveProbe)
			}
			persistStage(45, collectionStageMessage(false, plan.traffic, plan.probes), active)
		}
	}
	if request.Automatic && r.authRecovery != nil && len(plan.authRecoveryHosts) > 0 {
		recoveryStarted := time.Now()
		recoverySummary, recoveryErr := r.authRecovery.RecoverInvalid(ctx, plan.authRecoveryHosts, request.Actor)
		timings = append(timings, operationTiming(operationAuthRecovery, recoveryStarted))
		operations = append(operations, operationAuthRecovery)
		resultPayload["auth_recovery"] = recoverySummary
		summary.AuthRecovered = recoverySummary.Recovered
		if recoveryErr != nil {
			failures = append(failures, "鉴权自动恢复："+recoveryErr.Error())
		} else if recoverySummary.Failed > 0 {
			partialFailures = append(partialFailures, fmt.Sprintf("鉴权自动恢复部分失败：成功 %d，失败 %d", recoverySummary.Recovered, recoverySummary.Failed))
		}
	}
	if plan.accountRates && !plan.upstreams {
		r.enqueueAccountRateSync(ctx, request, plan, resultPayload, &operations, &failures, &partialFailures)
	}
	if plan.pricing && r.pricing != nil {
		persistStage(55, "正在按盈利比例动态调整账号分组", []string{operationPriceManagement})
		priceStarted := time.Now()
		priceResult, priceErr := r.pricing.ApplyNow(ctx, request.Actor)
		timings = append(timings, operationTiming(operationPriceManagement, priceStarted))
		operations = append(operations, operationPriceManagement)
		resultPayload["price_management"] = priceResult
		if priceErr != nil {
			if errors.Is(priceErr, context.Canceled) {
				resetCtx := context.WithoutCancel(ctx)
				if resetErr := r.repository.ResetInspectionTask(resetCtx, "price-management", started); resetErr != nil {
					failures = append(failures, "价格分组调整取消后恢复到期状态失败："+resetErr.Error())
				}
			}
			failures = append(failures, "价格分组调整："+priceErr.Error())
		}
	}
	if runInspection {
		outcome := <-evidenceResults
		evidenceResult, err := outcome.result, outcome.err
		monitoringEnabled = evidenceResult.MonitoringAvailable
		summary.Channels = evidenceResult.MonitoredAccounts
		summary.Probed = evidenceResult.ProbesPersisted
		summary.Samples = evidenceResult.TrafficPersisted + evidenceResult.ProbesPersisted
		if plan.traffic {
			operations = append(operations, operationTrafficRefresh)
		}
		if evidenceResult.ProbeDurationSecond > 0 || plan.probes {
			operations = append(operations, operationActiveProbe)
		}
		timings = append(timings, operationTimingDuration("evidence_collection", outcome.startedAt, outcome.duration))
		if err != nil {
			return finish(append(failures, "请求记录与探针："+err.Error()), partialFailures)
		}
		resultPayload["evidence"] = evidenceResult
	}
	if plan.upstreams || runInspection || plan.routing || plan.pricing {
		persistStage(65, "正在计算健康状态与调度目标", []string{operationRoutingCalculation})
		routingStarted := time.Now()
		capabilities, valid := runtimepolicy.For(plan.mode)
		if !valid {
			return finish(append(failures, "运行模式无效："+plan.mode), partialFailures)
		}
		scopedDiagnostic := request.AccountID != nil || request.GroupName != nil
		if scopedDiagnostic {
			resultPayload["diagnostic_only"] = true
			resultPayload["diagnostic_detail"] = "仅更新健康评估；不保存调度结果，不自动执行远程变更"
		}
		persistDecisions := capabilities.PersistRoutingDecisions && !scopedDiagnostic
		routingResult, err := r.router.Calculate(ctx, routing.Scope{AccountID: request.AccountID, GroupName: request.GroupName}, persistDecisions)
		timings = append(timings, operationTiming(operationRoutingCalculation, routingStarted))
		operations = append(operations, operationRoutingCalculation)
		if err != nil {
			return finish(append(failures, "调度计算："+err.Error()), partialFailures)
		}
		summary.Channels = routingResult.Accounts
		resultPayload["routing"] = routingResult
		if capabilities.AutomaticRemoteApply && !scopedDiagnostic && r.writer != nil {
			persistStage(78, "正在应用调度目标", []string{operationRoutingWriteback})
			writeStarted := time.Now()
			writeResult, writeErr := r.writer.Apply(ctx, routingResult.AccountTargets, request.Actor)
			timings = append(timings, operationTiming(operationRoutingWriteback, writeStarted))
			operations = append(operations, operationRoutingWriteback)
			resultPayload["writeback"] = writeResult
			summary.Applied = writeResult.Changed
			summary.Fused, summary.Recovered = appliedRoutingTransitions(routingResult.AccountTargets, writeResult)
			summary.CleanedUp = appliedCleanupCount(routingResult.AccountTargets, writeResult)
			if writeErr != nil {
				failures = append(failures, "自动执行："+writeErr.Error())
			} else if writeResult.Failed > 0 {
				partialFailures = append(partialFailures, fmt.Sprintf("自动执行部分失败：%d 项", writeResult.Failed))
			}
		}
	}
	if plan.alert && r.alerts != nil {
		persistStage(90, "正在检测告警并发送通知", []string{operationAlertEvaluation})
		alertStarted := time.Now()
		alertResult, alertErr := r.alerts.Evaluate(ctx)
		timings = append(timings, operationTiming(operationAlertEvaluation, alertStarted))
		operations = append(operations, operationAlertEvaluation)
		resultPayload["alert_evaluation"] = alertResult
		summary.Alerts = alertResult.Findings
		if alertErr != nil {
			failures = append(failures, "告警检测："+alertErr.Error())
		} else if alertResult.Status == "failed" {
			failures = append(failures, alertResult.Summary)
		}
	}
	return finish(failures, partialFailures)
}

func authenticationFailedHosts(batch upstreamsync.BatchResult) []string {
	hosts := make([]string, 0, batch.AuthFailed)
	for _, result := range batch.Hosts {
		if result.Status == "auth_failed" {
			hosts = append(hosts, result.Host)
		}
	}
	return hosts
}

func uniqueHostList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		host := configstore.CanonicalHost(value)
		if host == "" {
			continue
		}
		if _, found := seen[host]; found {
			continue
		}
		seen[host] = struct{}{}
		result = append(result, host)
	}
	return result
}

func integerResultValue(result map[string]any, key string) int {
	if result == nil {
		return 0
	}
	switch value := result[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func appliedCleanupCount(targets map[string]business.AccountRoutingTarget, result routingwrite.Result) int {
	count := 0
	for _, item := range result.Results {
		target, found := targets[item.AccountID]
		if found && target.CleanupAction != nil && item.Changed {
			count++
		}
	}
	return count
}

func appliedRoutingTransitions(targets map[string]business.AccountRoutingTarget, result routingwrite.Result) (int, int) {
	fused, recovered := 0, 0
	for _, item := range result.Results {
		if !item.Changed {
			continue
		}
		target, found := targets[item.AccountID]
		if !found || target.Schedulable == nil {
			continue
		}
		before, beforeOK := mapBool(item.Before, "schedulable")
		after, afterOK := mapBool(item.Effective, "schedulable")
		if !beforeOK || !afterOK || before == after {
			continue
		}
		if before && !after && !*target.Schedulable {
			fused++
		}
		if !before && after && *target.Schedulable {
			recovered++
		}
	}
	return fused, recovered
}

func mapBool(values map[string]any, key string) (bool, bool) {
	if values == nil {
		return false, false
	}
	value, ok := values[key].(bool)
	return value, ok
}

func strictEvidenceFallback(request RunRequest) bool {
	return request.AccountID != nil || request.GroupName != nil
}

func (r *Runner) finishTask(
	ctx context.Context,
	task taskstore.Task,
	operations []string,
	timings []business.OperationTiming,
	payload map[string]any,
	failures []string,
	partialFailures []string,
) ExecutionResult {
	payload["operations"] = operations
	payload["operation_timings"] = timings
	payload["active_operations"] = []string{}
	payload["completed_operations"] = append([]string{}, operations...)
	task.Progress, task.UpdatedAt, task.Result = 100, r.now().UTC().Format(time.RFC3339Nano), payload
	contextFailure := taskstore.ContextFailureCause(ctx)
	if errors.Is(contextFailure, mutationguard.ErrAutomaticInspectionPreempted) {
		task.Status, task.Message = "cancelled", "自动巡检已让位于手工操作"
		if task.Result == nil {
			task.Result = map[string]any{}
		}
		delete(task.Result, "error")
		task.Result["cancelled"] = true
		task.Result["cancel_reason"] = mutationguard.ErrAutomaticInspectionPreempted.Error()
		if err := taskstore.SaveFinal(context.Background(), r.tasks, task); err != nil {
			return failedExecution(&task.ID, operations, timings, err)
		}
		return ExecutionResult{TaskID: &task.ID, Status: "cancelled", Operations: operations, OperationTiming: timings}
	}
	if taskstore.MarkCancelled(ctx, &task, "巡检已取消") {
		if err := taskstore.SaveFinal(context.Background(), r.tasks, task); err != nil {
			return failedExecution(&task.ID, operations, timings, err)
		}
		return ExecutionResult{TaskID: &task.ID, Status: "cancelled", Operations: operations, OperationTiming: timings}
	}
	if contextFailure != nil {
		failures = append(failures, contextFailure.Error())
	}
	status := "succeeded"
	var errorText *string
	if len(failures) > 0 {
		status, task.Status, task.Message = "failed", "failed", "巡检完成，但存在失败项"
		if contextFailure != nil {
			task.Message = "巡检因执行上下文失败而停止：" + contextFailure.Error()
		}
		value := strings.Join(append(append([]string{}, failures...), partialFailures...), "；")
		errorText = &value
		task.Result["error"] = value
	} else if len(partialFailures) > 0 {
		status, task.Status, task.Message = "partial", "partial", "巡检完成，但存在部分失败"
		value := strings.Join(partialFailures, "；")
		errorText = &value
		task.Result["error"] = value
	} else {
		task.Status, task.Message = "succeeded", "巡检完成"
		if diagnostic, _ := payload["diagnostic_only"].(bool); diagnostic {
			task.Message = "诊断巡检完成：仅更新健康评估，未保存调度结果，未自动执行远程变更"
		}
	}
	if err := taskstore.SaveFinal(context.Background(), r.tasks, task); err != nil {
		return failedExecution(&task.ID, operations, timings, err)
	}
	return ExecutionResult{TaskID: &task.ID, Status: status, Error: errorText, Operations: operations, OperationTiming: timings}
}

func (r *Runner) evidenceAdmin(ctx context.Context) evidence.Admin {
	if r.targets == nil {
		return nil
	}
	target, err := targetguard.Settings(ctx, r.targets)
	if err != nil {
		return nil
	}
	client, err := adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 1,
	}, nil)
	if err != nil {
		return nil
	}
	return client
}

func newInspectionTask(now time.Time, automatic bool) (taskstore.Task, error) {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return taskstore.Task{}, err
	}
	operation, message := "manual-inspection", "手动巡检已排队"
	if automatic {
		operation, message = "automatic-inspection", "自动巡检已排队"
	}
	formatted := now.UTC().Format(time.RFC3339Nano)
	return taskstore.Task{
		ID: hex.EncodeToString(value), Skill: "sub2api-auto-inspection", Operation: operation,
		Status: "queued", Progress: 0, Message: message, Result: map[string]any{}, CreatedAt: formatted, UpdatedAt: formatted,
	}, nil
}

func inspectionSection(policy map[string]any, key string) (map[string]any, error) {
	raw, present := policy[key]
	if !present {
		return map[string]any{}, nil
	}
	result, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("策略字段 " + key + " 必须是对象")
	}
	return result, nil
}

func positiveSetting(section map[string]any, key string, fallback int) (int, error) {
	return boundedSetting(section, key, fallback, 1)
}

func boundedSetting(section map[string]any, key string, fallback, minimum int) (int, error) {
	raw, present := section[key]
	if !present {
		return fallback, nil
	}
	value, ok := raw.(int64)
	if !ok || value < int64(minimum) || value > 86400 {
		return 0, fmt.Errorf("策略字段 %s 必须是 %d 到 86400 之间的整数", key, minimum)
	}
	return int(value), nil
}

func boolSetting(section map[string]any, key string, fallback bool) (bool, error) {
	raw, present := section[key]
	if !present {
		return fallback, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, errors.New("策略字段 " + key + " 必须是布尔值")
	}
	return value, nil
}

func operationTiming(name string, started time.Time) business.OperationTiming {
	return operationTimingDuration(name, started, time.Since(started))
}

func operationTimingDuration(name string, started time.Time, duration time.Duration) business.OperationTiming {
	startedAt := started.UTC().Format(time.RFC3339Nano)
	if duration < 0 {
		duration = 0
	}
	return business.OperationTiming{Operation: name, DurationSeconds: duration.Seconds(), StartedAt: &startedAt}
}

func failedExecution(taskID *string, operations []string, timings []business.OperationTiming, err error) ExecutionResult {
	value := err.Error()
	return ExecutionResult{TaskID: taskID, Status: "failed", Error: &value, Operations: operations, OperationTiming: timings}
}
