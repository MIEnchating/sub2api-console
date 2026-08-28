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
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type RunnerRepository interface {
	ControlPolicy(context.Context) (map[string]any, error)
	Mode(context.Context) (string, error)
	AlertPolicy(context.Context) (business.AlertPolicy, error)
	InspectionTaskDue(context.Context, string, int, time.Time) (bool, error)
	MarkInspectionTask(context.Context, string, time.Time) error
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

type InspectionTaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type InspectionTargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type RunRequest struct {
	AccountID *string
	GroupName *string
	Actor     string
	Automatic bool
}

type Runner struct {
	repository RunnerRepository
	targets    InspectionTargetStore
	evidence   EvidenceCollector
	router     Router
	writer     RoutingWriter
	alerts     AlertEvaluator
	upstreams  UpstreamSynchronizer
	tasks      InspectionTaskStore
	now        func() time.Time
}

type duePlan struct {
	traffic   bool
	probes    bool
	upstreams bool
	alert     bool
	routing   bool
	policy    map[string]any
	mode      string
	probeIDs  []string
}

type upstreamSyncOutcome struct {
	batch     upstreamsync.BatchResult
	err       error
	startedAt time.Time
}

type evidenceCollectionOutcome struct {
	result    evidence.Result
	err       error
	startedAt time.Time
}

const (
	operationUpstreamSync       = "upstream_sync"
	operationTrafficRefresh     = "traffic_refresh"
	operationActiveProbe        = "active_probe"
	operationRoutingCalculation = "routing_calculation"
	operationRoutingWriteback   = "routing_writeback"
	operationAlertEvaluation    = "alert_evaluation"
)

func NewRunner(
	repository RunnerRepository,
	targets InspectionTargetStore,
	evidenceCollector EvidenceCollector,
	router Router,
	writer RoutingWriter,
	alerts AlertEvaluator,
	upstreams UpstreamSynchronizer,
	tasks InspectionTaskStore,
) *Runner {
	return &Runner{
		repository: repository, targets: targets, evidence: evidenceCollector, router: router,
		writer: writer, alerts: alerts, upstreams: upstreams, tasks: tasks, now: time.Now,
	}
}

func (r *Runner) Execute(ctx context.Context, _ business.AutoInspectionConfig) (ExecutionResult, error) {
	return r.Run(ctx, RunRequest{Actor: "自动巡检", Automatic: true})
}

func (r *Runner) Preview(ctx context.Context, now time.Time) (QueueItem, error) {
	plan, err := r.plan(ctx, RunRequest{Automatic: true}, now.UTC())
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
	operations := make([]QueueOperation, 0, 5)
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
	if !plan.traffic && !plan.probes && !plan.upstreams && !plan.routing && !plan.alert {
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
		result := failedExecution(&task.ID, nil, nil, err)
		r.failQueuedTask(task, err)
		return result
	}
	return r.executeTask(ctx, task, request, plan, now)
}

func (r *Runner) failQueuedTask(task taskstore.Task, err error) {
	task.Status, task.Progress, task.Message, task.UpdatedAt = "failed", 100, "巡检启动失败："+err.Error(), r.now().UTC().Format(time.RFC3339Nano)
	task.Result = map[string]any{"error": err.Error(), "remote_write": false}
	taskstore.PersistFinal(r.tasks, task)
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
	}
	return result, nil
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
	operations := []string{}
	timings := []business.OperationTiming{}
	plannedOperations, err := dueQueueOperations(plan)
	if err != nil {
		r.failQueuedTask(task, err)
		return failedExecution(&task.ID, operations, timings, err)
	}
	resultPayload := map[string]any{
		"planned_operations":   plannedOperations,
		"active_operations":    []string{},
		"completed_operations": []string{},
	}
	failures := []string{}
	summary := InspectionSummary{}
	var monitoringEnabled *bool
	persistStage := func(progress int, message string, active []string) {
		resultPayload["active_operations"] = append([]string{}, active...)
		resultPayload["completed_operations"] = append([]string{}, operations...)
		task.Status, task.Progress, task.Message = "running", progress, message
		task.UpdatedAt, task.Result = r.now().UTC().Format(time.RFC3339Nano), resultPayload
		taskstore.PersistProgress(r.tasks, task)
	}
	finish := func(currentFailures []string) ExecutionResult {
		result := r.finishTask(ctx, task, operations, timings, resultPayload, currentFailures)
		result.MonitoringEnabled = monitoringEnabled
		result.Summary = &summary
		return result
	}
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 5, "正在准备本轮巡检任务", r.now().UTC().Format(time.RFC3339Nano)
	task.Result = resultPayload
	if err := r.tasks.Save(ctx, task); err != nil {
		return failedExecution(&task.ID, operations, timings, err)
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
			upstreamResults <- upstreamSyncOutcome{batch: batch, err: syncErr, startedAt: upstreamStartedAt}
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
			evidenceResults <- evidenceCollectionOutcome{result: evidenceResult, err: collectErr, startedAt: evidenceStarted}
		}()
	}
	if plan.upstreams {
		outcome := <-upstreamResults
		timings = append(timings, operationTiming(operationUpstreamSync, outcome.startedAt))
		operations = append(operations, operationUpstreamSync)
		resultPayload["upstream_sync"] = outcome.batch
		if outcome.err != nil {
			failures = append(failures, "上游同步："+outcome.err.Error())
		} else if outcome.batch.AuthFailed > 0 || outcome.batch.Failed > 0 {
			failures = append(failures, fmt.Sprintf("上游同步部分失败：鉴权 %d，其他 %d", outcome.batch.AuthFailed, outcome.batch.Failed))
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
		timings = append(timings, operationTiming("evidence_collection", outcome.startedAt))
		if err != nil {
			return finish(append(failures, "请求记录与探针："+err.Error()))
		}
		resultPayload["evidence"] = evidenceResult
	}
	if plan.upstreams || runInspection || plan.routing {
		persistStage(65, "正在计算健康状态与调度目标", []string{operationRoutingCalculation})
		routingStarted := time.Now()
		capabilities, valid := runtimepolicy.For(plan.mode)
		if !valid {
			return finish(append(failures, "运行模式无效："+plan.mode))
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
			return finish(append(failures, "调度计算："+err.Error()))
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
				failures = append(failures, fmt.Sprintf("自动执行失败 %d 项", writeResult.Failed))
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
	return finish(failures)
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
) ExecutionResult {
	payload["operations"] = operations
	payload["operation_timings"] = timings
	payload["active_operations"] = []string{}
	payload["completed_operations"] = append([]string{}, operations...)
	task.Progress, task.UpdatedAt, task.Result = 100, r.now().UTC().Format(time.RFC3339Nano), payload
	if errors.Is(ctx.Err(), context.Canceled) {
		task.Status, task.Message = "cancelled", "巡检已取消"
		task.Result["cancelled"] = true
		if err := taskstore.SaveFinal(context.Background(), r.tasks, task); err != nil {
			return failedExecution(&task.ID, operations, timings, err)
		}
		return ExecutionResult{TaskID: &task.ID, Status: "cancelled", Operations: operations, OperationTiming: timings}
	}
	status := "succeeded"
	var errorText *string
	if len(failures) > 0 {
		status, task.Status, task.Message = "failed", "failed", "巡检完成，但存在失败项"
		value := strings.Join(failures, "；")
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
	target, err := r.targets.TargetSettings(ctx)
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
	startedAt := started.UTC().Format(time.RFC3339Nano)
	return business.OperationTiming{Operation: name, DurationSeconds: time.Since(started).Seconds(), StartedAt: &startedAt}
}

func failedExecution(taskID *string, operations []string, timings []business.OperationTiming, err error) ExecutionResult {
	value := err.Error()
	return ExecutionResult{TaskID: taskID, Status: "failed", Error: &value, Operations: operations, OperationTiming: timings}
}
