package routingwrite

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

const restoreControlConcurrency = 4

type Repository interface {
	Mode(context.Context) (string, error)
	ControlPolicy(context.Context) (map[string]any, error)
	RoutingBaselines(context.Context) ([]business.RoutingBaseline, error)
	CaptureRoutingBaseline(context.Context, business.RoutingBaseline) error
	UpdateRoutingManagedIntent(context.Context, string, business.RoutingManagedIntent) error
	CommitRoutingReadback(context.Context, string, business.RoutingReadback, bool, business.AccountOperation) error
	AbandonRoutingControl(context.Context, string, business.RoutingReadback, business.AccountOperation) error
	ClearRoutingRuntimeBlocks(context.Context, string) error
	DeleteRoutingBaseline(context.Context, string) error
	RecordAccountOperation(context.Context, business.AccountOperation) error
	RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error)
	MarkCleanupPaused(context.Context, string, string) error
	MarkCleanupDisabled(context.Context, string) error
	DeleteAccountProjection(context.Context, string, business.AccountOperation) error
}

type manualPriorityRepository interface {
	ManualPriorityControls(context.Context, []string) (map[string]business.ManualPriorityControl, error)
}

type TargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type Admin interface {
	Account(context.Context, string) (map[string]any, error)
	Mutate(context.Context, string, string, map[string]any) (map[string]any, error)
	DeleteAccount(context.Context, string) (map[string]any, error)
}

type AccountResult struct {
	AccountID   string         `json:"account_id"`
	Changed     bool           `json:"changed"`
	RemoteWrite bool           `json:"remote_write"`
	Restored    bool           `json:"restored,omitempty"`
	Released    bool           `json:"released,omitempty"`
	Skipped     bool           `json:"skipped,omitempty"`
	Reason      *string        `json:"reason,omitempty"`
	Error       *string        `json:"error,omitempty"`
	Before      map[string]any `json:"before,omitempty"`
	Desired     map[string]any `json:"desired,omitempty"`
	Effective   map[string]any `json:"effective,omitempty"`
}

type Result struct {
	Mode            string          `json:"mode"`
	CalculationOnly bool            `json:"calculation_only"`
	RemoteWrite     bool            `json:"remote_write"`
	Changed         int             `json:"changed"`
	Restored        int             `json:"restored,omitempty"`
	Released        int             `json:"released,omitempty"`
	Succeeded       int             `json:"succeeded"`
	Failed          int             `json:"failed"`
	Results         []AccountResult `json:"results"`
	Reason          *string         `json:"reason,omitempty"`
}

type Service struct {
	targets    TargetStore
	repository Repository
	admin      Admin
	now        func() time.Time
}

type writePolicy struct {
	autoApply        map[string]bool
	changeThreshold  *big.Rat
	maxConcurrency   int
	verifyAfterWrite bool
}

func New(targets TargetStore, repository Repository) *Service {
	return &Service{targets: targets, repository: repository, now: time.Now}
}

func (s *Service) Apply(ctx context.Context, targets map[string]business.AccountRoutingTarget, actor string) (Result, error) {
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return Result{}, err
	}
	capabilities, valid := runtimepolicy.For(mode)
	if !valid {
		return Result{}, fmt.Errorf("运行模式无效：%s", mode)
	}
	result := Result{Mode: mode, CalculationOnly: !capabilities.AutomaticRemoteApply, Results: []AccountResult{}}
	if !capabilities.AutomaticRemoteApply {
		reason := "当前运行模式只保存计算结果"
		result.Reason = &reason
		return result, nil
	}
	if len(targets) == 0 {
		reason := "本轮没有需要自动执行的账号目标"
		result.Reason = &reason
		return result, nil
	}
	policyDocument, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return Result{}, err
	}
	policy, err := parseWritePolicy(policyDocument)
	if err != nil {
		return Result{}, err
	}
	if manualRepository, ok := s.repository.(manualPriorityRepository); ok {
		controls, controlErr := manualRepository.ManualPriorityControls(ctx, sortedTargetIDs(targets))
		if controlErr != nil {
			return Result{}, fmt.Errorf("人工优先位保护状态读取失败：%w", controlErr)
		}
		if len(controls) > 0 {
			filtered := make(map[string]business.AccountRoutingTarget, len(targets)-len(controls))
			for _, accountID := range sortedTargetIDs(targets) {
				target := targets[accountID]
				if _, protected := controls[accountID]; protected {
					reason := "账号处于人工优先位，自动调度写回已跳过"
					result.Results = append(result.Results, AccountResult{AccountID: accountID, Skipped: true, Reason: &reason})
					continue
				}
				filtered[accountID] = target
			}
			targets = filtered
		}
	}
	if len(targets) == 0 {
		reason := "本轮账号均处于人工优先位，没有需要自动执行的目标"
		result.Reason = &reason
		return result, nil
	}
	admin, err := s.adminClient(ctx)
	if err != nil {
		return Result{}, err
	}
	orderedIDs := orderedTargetIDs(targets)
	currentPayloads := map[string]map[string]any{}
	if _, supported := admin.(accountLister); supported {
		currentPayloads, err = listAccountPayloads(ctx, admin)
		if err != nil {
			return Result{}, fmt.Errorf("批量读取账号失败：%w", err)
		}
	}
	admin = limitAdmin(admin, policy.maxConcurrency)
	items := make([]AccountResult, len(orderedIDs))
	regularCount := len(orderedIDs)
	for index, accountID := range orderedIDs {
		if targets[accountID].CleanupAction != nil {
			regularCount = index
			break
		}
	}
	runBatch := func(start, end int) {
		if start >= end {
			return
		}
		coordinator := newBatchWriteCoordinator(ctx, admin, end-start, policy.verifyAfterWrite)
		var wait sync.WaitGroup
		for index := start; index < end; index++ {
			index, accountID := index, orderedIDs[index]
			wait.Add(1)
			go func() {
				defer wait.Done()
				items[index] = s.applyAccountCoordinated(ctx, admin, targets[accountID], policy, actor, currentPayloads[accountID], coordinator)
			}()
		}
		wait.Wait()
	}
	runBatch(0, regularCount)
	runBatch(regularCount, len(orderedIDs))
	for index, item := range items {
		result.Results = append(result.Results, item)
		result.RemoteWrite = result.RemoteWrite || item.RemoteWrite
		target := targets[orderedIDs[index]]
		if target.CleanupAction == nil && (item.Changed || item.Error != nil) {
			s.recordRoutingApplyEvent(ctx, target, item)
		}
		if item.Changed {
			result.Changed++
		}
		if item.Released {
			result.Released++
		}
		if item.Error != nil {
			result.Failed++
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

func (s *Service) recordRoutingApplyEvent(ctx context.Context, target business.AccountRoutingTarget, result AccountResult) {
	eventType, status := "routing.applied", "succeeded"
	summary := "账号 " + target.AccountID + " 自动写回已生效"
	payload := map[string]any{
		"account_id": target.AccountID, "groups": target.GroupNames,
		"state": target.DesiredHealth, "desired": result.Desired, "effective": result.Effective,
		"remote_write": result.RemoteWrite,
	}
	if result.Error != nil {
		eventType, status = "routing.apply_failed", "failed"
		summary = "账号 " + target.AccountID + " 自动写回失败：" + *result.Error
		payload["error"] = *result.Error
	}
	s.recordRuntimeEvent(ctx, eventType, status, summary, payload)
}

// Guardian completes routing writeback for the whole round before destructive
// cleanup starts. Keep cleanup targets last so a recovery write cannot be
// delayed until after another account has already been removed from the pool.
func orderedTargetIDs(targets map[string]business.AccountRoutingTarget) []string {
	regular := make(map[string]business.AccountRoutingTarget, len(targets))
	cleanup := make(map[string]business.AccountRoutingTarget)
	for accountID, target := range targets {
		if target.CleanupAction != nil {
			cleanup[accountID] = target
			continue
		}
		regular[accountID] = target
	}
	return append(sortedTargetIDs(regular), sortedTargetIDs(cleanup)...)
}

func (s *Service) RestoreControl(ctx context.Context, actor string) (Result, error) {
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return Result{}, err
	}
	if mode != runtimepolicy.Full {
		return Result{}, errors.New("交还控制权只能在完全模式执行")
	}
	baselines, err := s.repository.RoutingBaselines(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{Mode: mode, Results: []AccountResult{}}
	if len(baselines) == 0 {
		reason := "没有需要交还的账号"
		result.Reason = &reason
		return result, nil
	}
	admin, err := s.adminClient(ctx)
	if err != nil {
		return Result{}, err
	}
	currentPayloads := map[string]map[string]any{}
	if _, supported := admin.(accountLister); supported {
		currentPayloads, err = listAccountPayloads(ctx, admin)
		if err != nil {
			return Result{}, fmt.Errorf("批量读取账号失败：%w", err)
		}
	}
	admin = limitAdmin(admin, restoreControlConcurrency)
	coordinator := newBatchWriteCoordinator(ctx, admin, len(baselines), true)
	items := make([]AccountResult, len(baselines))
	var wait sync.WaitGroup
	for index, baseline := range baselines {
		index, baseline := index, baseline
		wait.Add(1)
		go func() {
			defer wait.Done()
			items[index] = s.restoreAccount(ctx, admin, baseline, actor, currentPayloads[baseline.AccountID], coordinator)
		}()
	}
	wait.Wait()
	for _, item := range items {
		result.Results = append(result.Results, item)
		result.RemoteWrite = result.RemoteWrite || item.RemoteWrite
		if item.Error != nil {
			result.Failed++
			continue
		}
		result.Succeeded++
		result.Restored++
		if item.Changed {
			result.Changed++
		}
	}
	return result, nil
}

func (s *Service) applyAccountCoordinated(
	ctx context.Context,
	admin Admin,
	target business.AccountRoutingTarget,
	policy writePolicy,
	actor string,
	currentPayload map[string]any,
	coordinator *batchWriteCoordinator,
) AccountResult {
	submitted := false
	defer func() {
		if !submitted {
			coordinator.Skip()
		}
	}()
	result := AccountResult{AccountID: target.AccountID}
	operationID, err := randomOperationID("routing-writeback")
	if err != nil {
		return failedResult(result, err)
	}
	if len(target.GroupNames) == 0 {
		err := errors.New("目标缺少守护分组，已阻止自动执行")
		s.recordOperation(ctx, operation(operationID, "routing.writeback", target, actor, nil, nil, false, false, err))
		return failedResult(result, err)
	}
	if target.CleanupAction != nil && *target.CleanupAction == "delete" {
		return s.deleteCleanupAccount(ctx, admin, target, actor, operationID, currentPayload)
	}
	if currentPayload == nil {
		currentPayload, err = admin.Account(ctx, target.AccountID)
		if err != nil {
			s.recordOperation(ctx, operation(operationID, "routing.writeback", target, actor, nil, nil, false, false, err))
			return failedResult(result, err)
		}
	}
	current, err := remoteValues(currentPayload)
	if err != nil {
		s.recordOperation(ctx, operation(operationID, "routing.writeback", target, actor, nil, nil, false, false, err))
		return failedResult(result, err)
	}
	result.Before = current.asMap()
	if target.AbandonControl {
		op := operation(operationID, "routing.release_external", target, actor, current.asMap(), current.asMap(), false, true, nil)
		if err := s.repository.AbandonRoutingControl(ctx, target.AccountID, current.readback(nil), op); err != nil {
			s.recordLocalApplyFailure(ctx, operationID, "routing.release_external", target, actor, current.asMap(), current.asMap(), false, true, "local-release", err)
			return failedResult(result, err)
		}
		result.Released, result.Effective = true, current.asMap()
		reason := "检测到 Sub2API 人工修改，已保留当前值并停止 Console 托管"
		result.Reason = &reason
		return result
	}
	desired, err := desiredValues(target, policy, current)
	if err != nil {
		s.recordOperation(ctx, operation(operationID, "routing.writeback", target, actor, current.asMap(), nil, false, false, err))
		return failedResult(result, err)
	}
	if target.ReleaseControl {
		baseline, found, loadErr := s.baseline(ctx, target.AccountID)
		if loadErr != nil {
			return failedResult(result, loadErr)
		}
		if !found {
			if err := s.repository.DeleteRoutingBaseline(ctx, target.AccountID); err != nil {
				return failedResult(result, err)
			}
			reason := "接管前没有可恢复字段，已交还控制权"
			result.Restored, result.Reason = true, &reason
			return result
		}
		var conflicts []string
		desired, conflicts = restorableBaselineValues(baseline, current)
		if len(conflicts) > 0 {
			reason := "以下字段已被外部修改，交还时保留当前值：" + strings.Join(conflicts, ",")
			result.Reason = &reason
		}
	}
	desired = applyDeadband(desired, current, policy.changeThreshold, target.ReleaseControl)
	result.Desired = desired
	if len(desired) == 0 {
		if target.CleanupAction != nil {
			switch *target.CleanupAction {
			case "pause":
				if err := s.repository.MarkCleanupPaused(ctx, target.AccountID, "认证持续失效，已自动暂停"); err != nil {
					s.recordCleanupOutcome(ctx, target, "pause", false, err)
					s.recordLocalApplyFailure(ctx, operationID, "routing.writeback", target, actor, current.asMap(), current.asMap(), false, true, "local-cleanup", err)
					return failedResult(result, err)
				}
				s.recordCleanupOutcome(ctx, target, "pause", false, nil)
				result.Changed = true
				reason := "账号已不接流量，本地暂停状态已生效"
				result.Reason = &reason
				return result
			case "disable":
				if err := s.repository.MarkCleanupDisabled(ctx, target.AccountID); err != nil {
					s.recordCleanupOutcome(ctx, target, "disable", false, err)
					s.recordLocalApplyFailure(ctx, operationID, "routing.writeback", target, actor, current.asMap(), current.asMap(), false, true, "local-cleanup", err)
					return failedResult(result, err)
				}
				s.recordCleanupOutcome(ctx, target, "disable", false, nil)
				result.Changed = true
				reason := "账号已不接流量，本地停用状态已生效"
				result.Reason = &reason
				return result
			}
		}
		state := confirmedRoutingState(target, current)
		if target.ReleaseControl {
			state = &target.DesiredHealth
		}
		if state != nil {
			if !target.ReleaseControl {
				if err := s.captureManagedOwnership(ctx, target, policy, current); err != nil {
					return failedResult(result, err)
				}
			}
			op := operation(operationID, operationType(target.ReleaseControl), target, actor, current.asMap(), current.asMap(), false, true, nil)
			op.FieldName = nil
			if err := s.repository.CommitRoutingReadback(ctx, target.AccountID, current.readback(state), target.ReleaseControl, op); err != nil {
				s.recordLocalApplyFailure(ctx, operationID, operationType(target.ReleaseControl), target, actor, current.asMap(), current.asMap(), false, true, "local-commit", err)
				return failedResult(result, err)
			}
			result.Effective = current.asMap()
			result.Restored = target.ReleaseControl
			if result.Reason == nil {
				reason := "远端状态已符合目标，读回确认已生效"
				if target.ReleaseControl {
					reason = "远端状态已符合接管前基线，控制权已交还"
				}
				result.Reason = &reason
			}
			return result
		}
		reason := "没有达到自动执行阈值的字段"
		if target.WriteCooldown || target.ScalingCooldown {
			reason = "调权或扩容仍在冷却期"
		}
		result.Skipped, result.Reason = true, &reason
		s.recordOperation(ctx, operation(operationID, "routing.writeback", target, actor, current.asMap(), desired, false, true, nil))
		return result
	}
	if !target.ReleaseControl {
		if err := s.captureManagedOwnership(ctx, target, policy, current); err != nil {
			s.recordLocalApplyFailure(ctx, operationID, "routing.writeback", target, actor, current.asMap(), desired, false, false, "ownership-capture", err)
			return failedResult(result, err)
		}
	}
	result.RemoteWrite = true
	submitted = true
	write := coordinator.Submit(ctx, target.AccountID, desired, current)
	if write.err != nil && !write.remoteConfirmed {
		s.recordOperation(ctx, operation(operationID, "routing.writeback", target, actor, current.asMap(), desired, false, false, write.err))
		return failedResult(result, write.err)
	}
	after := write.after
	if write.err != nil && !write.readbackConfirmed {
		s.recordOperation(ctx, operation(operationID, "routing.writeback", target, actor, current.asMap(), after.asMap(), true, false, write.err))
		return failedResult(result, write.err)
	}
	state := confirmedRoutingState(target, after)
	op := operation(operationID, operationType(target.ReleaseControl), target, actor, current.asMap(), after.asMap(), write.remoteConfirmed, write.readbackConfirmed, write.err)
	op.FieldName = changedReadbackFieldNames(current, after)
	if write.err != nil {
		op.Phase = "remote-partial"
	}
	if err := s.repository.CommitRoutingReadback(ctx, target.AccountID, after.readback(state), target.ReleaseControl, op); err != nil {
		s.recordLocalApplyFailure(ctx, operationID, operationType(target.ReleaseControl), target, actor, current.asMap(), after.asMap(), true, write.readbackConfirmed, "local-commit", err)
		return failedResult(result, err)
	}
	if shouldRecoverRuntime(current, after) {
		s.recoverRuntime(ctx, admin, target)
	}
	result.Changed, result.Effective = changedReadbackFieldNames(current, after) != nil, after.asMap()
	if write.err != nil {
		return failedResult(result, write.err)
	}
	if target.CleanupAction != nil {
		switch *target.CleanupAction {
		case "pause":
			if err := s.repository.MarkCleanupPaused(ctx, target.AccountID, "认证持续失效，已自动暂停"); err != nil {
				s.recordCleanupOutcome(ctx, target, "pause", true, err)
				s.recordLocalApplyFailure(ctx, operationID, "routing.writeback", target, actor, current.asMap(), after.asMap(), true, true, "local-cleanup", err)
				result.RemoteWrite = true
				return failedResult(result, err)
			}
			s.recordCleanupOutcome(ctx, target, "pause", true, nil)
		case "disable":
			if err := s.repository.MarkCleanupDisabled(ctx, target.AccountID); err != nil {
				s.recordCleanupOutcome(ctx, target, "disable", true, err)
				s.recordLocalApplyFailure(ctx, operationID, "routing.writeback", target, actor, current.asMap(), after.asMap(), true, true, "local-cleanup", err)
				result.RemoteWrite = true
				return failedResult(result, err)
			}
			s.recordCleanupOutcome(ctx, target, "disable", true, nil)
		}
	}
	result.Restored = target.ReleaseControl
	return result
}

func (s *Service) captureManagedOwnership(
	ctx context.Context,
	target business.AccountRoutingTarget,
	policy writePolicy,
	current values,
) error {
	intent := managedIntentForTarget(target, policy)
	if intent.Schedulable == nil && intent.Priority == nil && intent.LoadFactor == nil && intent.Concurrency == nil {
		return nil
	}
	if err := s.repository.CaptureRoutingBaseline(ctx, current.baseline(target.AccountID, s.now().UTC())); err != nil {
		return err
	}
	return s.repository.UpdateRoutingManagedIntent(ctx, target.AccountID, intent)
}

func managedIntentForTarget(target business.AccountRoutingTarget, policy writePolicy) business.RoutingManagedIntent {
	result := business.RoutingManagedIntent{}
	if policy.autoApply["schedulable"] {
		result.Schedulable = cloneBool(target.Schedulable)
	}
	if policy.autoApply["priority"] {
		result.Priority = cloneInt(target.Priority)
	}
	if policy.autoApply["load_factor"] {
		result.LoadFactor = cloneString(target.LoadFactor)
	}
	if policy.autoApply["concurrency"] {
		result.Concurrency = cloneInt(target.Concurrency)
	}
	return result
}

func shouldRecoverRuntime(before, after values) bool {
	return before.schedulable != nil && !*before.schedulable && after.schedulable != nil && *after.schedulable
}

func (s *Service) recoverRuntime(ctx context.Context, admin Admin, target business.AccountRoutingTarget) {
	for _, endpoint := range []struct {
		path  string
		label string
	}{
		{path: "/admin/accounts/" + target.AccountID + "/clear-error", label: "清除错误信息"},
		{path: "/admin/accounts/" + target.AccountID + "/recover-state", label: "复位运行状态"},
	} {
		if _, err := admin.Mutate(ctx, "POST", endpoint.path, nil); err != nil {
			s.recordRuntimeEvent(ctx, "routing_recovery_cleanup_failed", "failed", "账号 "+target.AccountID+endpoint.label+"失败", map[string]any{
				"account_id": target.AccountID, "groups": target.GroupNames, "endpoint": endpoint.path, "error": err.Error(),
			})
			if strings.HasSuffix(endpoint.path, "/recover-state") {
				return
			}
		}
	}
	if err := s.repository.ClearRoutingRuntimeBlocks(ctx, target.AccountID); err != nil {
		s.recordRuntimeEvent(ctx, "routing_recovery_cleanup_failed", "failed", "账号 "+target.AccountID+"本地运行状态清理失败", map[string]any{
			"account_id": target.AccountID, "groups": target.GroupNames, "error": err.Error(),
		})
	}
}

func confirmedRoutingState(target business.AccountRoutingTarget, current values) *string {
	if target.Schedulable == nil || current.schedulable == nil || *target.Schedulable != *current.schedulable {
		return nil
	}
	state := target.DesiredHealth
	return &state
}

func (s *Service) deleteCleanupAccount(
	ctx context.Context,
	admin Admin,
	target business.AccountRoutingTarget,
	actor, operationID string,
	currentPayload map[string]any,
) AccountResult {
	result := AccountResult{AccountID: target.AccountID}
	var err error
	if currentPayload == nil {
		currentPayload, err = admin.Account(ctx, target.AccountID)
		if err != nil {
			s.recordOperation(ctx, operation(operationID, "cleanup.delete", target, actor, nil, nil, false, false, err))
			return failedResult(result, err)
		}
	}
	current, err := remoteValues(currentPayload)
	if err != nil {
		return failedResult(result, err)
	}
	payload := map[string]any{
		"account_id": target.AccountID, "groups": target.GroupNames, "action": "delete",
		"remote_write": true, "snapshot": current.asMap(), "snapshot_note": "快照不含凭据，删除后无法据此重建",
	}
	if _, err := s.repository.RecordRuntimeEvent(ctx, "cleanup_delete_pending", "succeeded", "认证持续失效，准备自动删除账号 "+target.AccountID, payload); err != nil {
		return failedResult(result, err)
	}
	if current.schedulable != nil && *current.schedulable {
		result.RemoteWrite = true
		if predisableErr := writeSchedulable(ctx, admin, target.AccountID, false); predisableErr != nil {
			warning := copyMap(payload)
			warning["error"] = predisableErr.Error()
			s.recordRuntimeEvent(ctx, "cleanup_predisable_failed", "failed", "删除前摘除流量失败，仍将继续删除账号 "+target.AccountID, warning)
		}
	}
	result.RemoteWrite = true
	if _, err := deleteAccount(ctx, admin, target.AccountID, true); err != nil {
		s.recordOperation(ctx, operation(operationID, "cleanup.delete", target, actor, current.asMap(), nil, false, false, err))
		failed := copyMap(payload)
		failed["error"] = err.Error()
		s.recordRuntimeEvent(ctx, "cleanup_delete_failed", "failed", "自动删除账号失败 "+target.AccountID, failed)
		return failedResult(result, err)
	}
	op := operation(operationID, "cleanup.delete", target, actor, current.asMap(), map[string]any{"deleted": true}, true, true, nil)
	field := "deleted"
	op.FieldName = &field
	if err := s.repository.DeleteAccountProjection(ctx, target.AccountID, op); err != nil {
		result.RemoteWrite = true
		return failedResult(result, err)
	}
	s.recordRuntimeEvent(ctx, "cleanup_deleted", "succeeded", "账号 "+target.AccountID+" 已自动删除；本地快照不含凭据，无法据此重建", payload)
	result.Changed, result.RemoteWrite = true, true
	result.Effective = map[string]any{"deleted": true}
	return result
}

func copyMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (s *Service) recordCleanupOutcome(ctx context.Context, target business.AccountRoutingTarget, action string, remoteWrite bool, cause error) {
	status := "succeeded"
	eventType := "cleanup_" + action + "d"
	summary := "账号 " + target.AccountID + " 已自动" + cleanupWriteActionLabel(action)
	payload := map[string]any{
		"account_id": target.AccountID, "groups": target.GroupNames, "action": action, "remote_write": remoteWrite,
	}
	if cause != nil {
		status = "failed"
		eventType = "cleanup_" + action + "_failed"
		summary = "账号 " + target.AccountID + " 自动" + cleanupWriteActionLabel(action) + "失败"
		payload["error"] = cause.Error()
	}
	s.recordRuntimeEvent(ctx, eventType, status, summary, payload)
}

func (s *Service) recordRuntimeEvent(ctx context.Context, eventType, status, summary string, payload map[string]any) {
	if _, err := s.repository.RecordRuntimeEvent(ctx, eventType, status, summary, payload); err != nil {
		slog.Error("调度运行事件保存失败", "event_type", eventType, "status", status, "error", err)
	}
}

func (s *Service) recordLocalApplyFailure(
	ctx context.Context,
	operationID, kind string,
	target business.AccountRoutingTarget,
	actor string,
	before, after any,
	remote, readback bool,
	phase string,
	cause error,
) {
	op := operation(operationID, kind, target, actor, before, after, remote, readback, cause)
	op.Phase = phase
	s.recordOperation(ctx, op)
}

func cleanupWriteActionLabel(action string) string {
	if action == "pause" {
		return "暂停"
	}
	if action == "disable" {
		return "停用"
	}
	return action
}

func (s *Service) restoreAccount(
	ctx context.Context,
	admin Admin,
	baseline business.RoutingBaseline,
	actor string,
	beforePayload map[string]any,
	coordinator *batchWriteCoordinator,
) AccountResult {
	submitted := false
	defer func() {
		if !submitted {
			coordinator.Skip()
		}
	}()
	target := business.AccountRoutingTarget{AccountID: baseline.AccountID, GroupNames: []string{"交还控制权"}, ReleaseControl: true}
	result := AccountResult{AccountID: baseline.AccountID, Restored: true}
	operationID, err := randomOperationID("routing-restore")
	if err != nil {
		return failedResult(result, err)
	}
	if beforePayload == nil {
		beforePayload, err = admin.Account(ctx, baseline.AccountID)
		if err != nil {
			return failedResult(result, err)
		}
	}
	before, err := remoteValues(beforePayload)
	if err != nil {
		return failedResult(result, err)
	}
	desired, conflicts := restorableBaselineValues(baseline, before)
	changed := changedFields(desired, before)
	after, readbackConfirmed := before, true
	if len(changed) > 0 {
		submitted = true
		write := coordinator.Submit(ctx, baseline.AccountID, changed, before)
		result.RemoteWrite = write.remoteConfirmed
		if write.err != nil {
			s.recordOperation(ctx, operation(operationID, "routing.restore", target, actor, before.asMap(), changed, write.remoteConfirmed, write.readbackConfirmed, write.err))
			return failedResult(result, write.err)
		}
		after, readbackConfirmed = write.after, write.readbackConfirmed
	}
	op := operation(operationID, "routing.restore", target, actor, before.asMap(), after.asMap(), len(changed) > 0, readbackConfirmed, nil)
	if err := s.repository.CommitRoutingReadback(ctx, baseline.AccountID, after.readback(nil), true, op); err != nil {
		return failedResult(result, err)
	}
	result.Changed, result.RemoteWrite, result.Effective = len(changed) > 0, len(changed) > 0, after.asMap()
	result.Before = before.asMap()
	if len(conflicts) > 0 {
		reason := "已交还控制权；保留外部修改字段：" + strings.Join(conflicts, ",")
		result.Reason = &reason
	}
	return result
}

type routingWriteOutcome struct {
	remoteConfirmed bool
	payload         map[string]any
	err             error
}

func writeRoutingValues(ctx context.Context, admin Admin, accountID string, desired map[string]any) routingWriteOutcome {
	fields := copyMap(desired)
	rawSchedulable, hasSchedulable := fields["schedulable"]
	delete(fields, "schedulable")
	result := routingWriteOutcome{}
	errorsByStep := []error{}
	if len(fields) > 0 {
		if payload, err := admin.Mutate(ctx, "PUT", "/admin/accounts/"+accountID, fields); err != nil {
			errorsByStep = append(errorsByStep, fmt.Errorf("写回账号参数失败：%w", err))
		} else {
			result.remoteConfirmed, result.payload = true, payload
		}
	}
	if !hasSchedulable {
		result.err = errors.Join(errorsByStep...)
		return result
	}
	schedulable, ok := rawSchedulable.(bool)
	if !ok {
		errorsByStep = append(errorsByStep, errors.New("目标调度状态必须是布尔值"))
		result.err = errors.Join(errorsByStep...)
		return result
	}
	if payload, err := writeSchedulablePayload(ctx, admin, accountID, schedulable); err != nil {
		errorsByStep = append(errorsByStep, fmt.Errorf("写回可调度状态失败：%w", err))
	} else {
		result.remoteConfirmed, result.payload = true, payload
	}
	result.err = errors.Join(errorsByStep...)
	return result
}

func changedReadbackFieldNames(before, after values) *string {
	fields := []string{}
	if !sameBool(before.schedulable, after.schedulable) {
		fields = append(fields, "schedulable")
	}
	if !sameInt64(before.priority, after.priority) {
		fields = append(fields, "priority")
	}
	if !sameString(before.loadFactor, after.loadFactor) {
		fields = append(fields, "load_factor")
	}
	if !sameInt64(before.concurrency, after.concurrency) {
		fields = append(fields, "concurrency")
	}
	if !sameString(before.status, after.status) {
		fields = append(fields, "status")
	}
	if len(fields) == 0 {
		return nil
	}
	sort.Strings(fields)
	value := strings.Join(fields, ",")
	return &value
}

func writeSchedulable(ctx context.Context, admin Admin, accountID string, schedulable bool) error {
	_, err := writeSchedulablePayload(ctx, admin, accountID, schedulable)
	return err
}

func writeSchedulablePayload(ctx context.Context, admin Admin, accountID string, schedulable bool) (map[string]any, error) {
	body := map[string]any{"schedulable": schedulable}
	payload, err := admin.Mutate(ctx, "POST", "/admin/accounts/"+accountID+"/schedulable", body)
	if err == nil {
		return payload, nil
	}
	var httpError *adminclient.HTTPError
	if !errors.As(err, &httpError) || (httpError.StatusCode != 404 && httpError.StatusCode != 405) {
		return nil, err
	}
	// Older Sub2API releases did not expose the dedicated scheduling endpoint.
	payload, fallbackErr := admin.Mutate(ctx, "PUT", "/admin/accounts/"+accountID, body)
	return payload, fallbackErr
}

type values struct {
	schedulable *bool
	priority    *int64
	loadFactor  *string
	concurrency *int64
	status      *string
}

func remoteValues(raw map[string]any) (values, error) {
	result := values{}
	if value, present := raw["schedulable"]; present && value != nil {
		parsed, ok := value.(bool)
		if !ok {
			return values{}, errors.New("账号调度状态读回不可判定")
		}
		result.schedulable = &parsed
	}
	var err error
	if result.priority, err = optionalInteger(raw["priority"], false); err != nil {
		return values{}, errors.New("账号优先级读回不可判定")
	}
	if result.concurrency, err = optionalInteger(raw["concurrency"], false); err != nil {
		return values{}, errors.New("账号并发读回不可判定")
	}
	if result.loadFactor, err = optionalNonnegativeIntegerText(raw["load_factor"]); err != nil {
		return values{}, errors.New("账号负载因子读回不可判定")
	}
	if rawStatus, present := raw["status"]; present && rawStatus != nil {
		status, ok := rawStatus.(string)
		status = strings.ToLower(strings.TrimSpace(status))
		if !ok || status == "" {
			return values{}, errors.New("账号启用状态读回不可判定")
		}
		result.status = &status
	}
	return result, nil
}

func desiredValues(target business.AccountRoutingTarget, policy writePolicy, current values) (map[string]any, error) {
	result := map[string]any{}
	if policy.autoApply["schedulable"] && target.Schedulable != nil {
		result["schedulable"] = *target.Schedulable
	}
	if policy.autoApply["priority"] && target.Priority != nil {
		if *target.Priority < 0 {
			return nil, errors.New("目标优先级不能为负数")
		}
		result["priority"] = *target.Priority
	}
	if !target.ScalingCooldown && policy.autoApply["concurrency"] && target.Concurrency != nil {
		if *target.Concurrency < 0 {
			return nil, errors.New("目标并发不能为负数")
		}
		result["concurrency"] = *target.Concurrency
	}
	if !target.WriteCooldown && policy.autoApply["load_factor"] && target.LoadFactor != nil {
		value, err := optionalNonnegativeIntegerText(*target.LoadFactor)
		if err != nil || value == nil {
			return nil, errors.New("目标负载因子必须是非负整数")
		}
		result["load_factor"] = json.Number(*value)
	}
	if target.CleanupAction != nil {
		switch *target.CleanupAction {
		case "pause":
			result["schedulable"] = false
		case "disable":
			result["schedulable"], result["status"] = false, "inactive"
		default:
			return nil, errors.New("自动处置动作不受支持")
		}
	}
	return changedFields(result, current), nil
}

func parseWritePolicy(document map[string]any) (writePolicy, error) {
	result := writePolicy{autoApply: map[string]bool{}, changeThreshold: big.NewRat(1, 10), maxConcurrency: 4}
	rawApply, ok := document["auto_apply"].(map[string]any)
	if !ok {
		return writePolicy{}, errors.New("策略字段 auto_apply 必须是对象")
	}
	for _, field := range []string{"schedulable", "priority", "load_factor", "concurrency"} {
		raw, present := rawApply[field]
		if !present {
			result.autoApply[field] = false
			continue
		}
		value, ok := raw.(bool)
		if !ok {
			return writePolicy{}, fmt.Errorf("策略字段 auto_apply.%s 无效", field)
		}
		result.autoApply[field] = value
	}
	weights, ok := document["weights"].(map[string]any)
	if !ok {
		return writePolicy{}, errors.New("策略字段 weights 必须是对象")
	}
	if raw, present := weights["change_threshold"]; present {
		value, err := decimal(raw)
		if err != nil || value.Sign() <= 0 || value.Cmp(big.NewRat(1, 1)) > 0 {
			return writePolicy{}, errors.New("策略字段 weights.change_threshold 必须大于 0 且不超过 1")
		}
		result.changeThreshold = value
	}
	if rawWriteback, present := document["writeback"]; present {
		writeback, ok := rawWriteback.(map[string]any)
		if !ok {
			return writePolicy{}, errors.New("策略字段 writeback 必须是对象")
		}
		if raw, present := writeback["concurrency"]; present {
			value, err := integer(raw)
			if err != nil || value < 1 || value > 16 {
				return writePolicy{}, errors.New("策略字段 writeback.concurrency 必须在 1 到 16 之间")
			}
			result.maxConcurrency = int(value)
		}
		if raw, present := writeback["verification"]; present {
			value, ok := raw.(bool)
			if !ok {
				return writePolicy{}, errors.New("策略字段 writeback.verification 必须是布尔值")
			}
			result.verifyAfterWrite = value
		}
	}
	return result, nil
}

func (s *Service) baseline(ctx context.Context, accountID string) (business.RoutingBaseline, bool, error) {
	rows, err := s.repository.RoutingBaselines(ctx)
	if err != nil {
		return business.RoutingBaseline{}, false, err
	}
	for _, item := range rows {
		if item.AccountID == accountID {
			return item, true, nil
		}
	}
	return business.RoutingBaseline{}, false, nil
}

func (s *Service) adminClient(ctx context.Context) (Admin, error) {
	if s.admin != nil {
		return s.admin, nil
	}
	if s.targets == nil {
		return nil, errors.New("未配置 Admin API 客户端")
	}
	target, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	return adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3,
	}, nil)
}

func applyDeadband(desired map[string]any, current values, threshold *big.Rat, release bool) map[string]any {
	if release {
		return changedFields(desired, current)
	}
	result := changedFields(desired, current)
	for _, field := range []string{"load_factor"} {
		raw, present := result[field]
		if !present {
			continue
		}
		wanted, err := decimal(raw)
		if err != nil {
			continue
		}
		var currentRaw any
		switch field {
		case "priority":
			currentRaw = current.priority
		case "load_factor":
			currentRaw = current.loadFactor
		case "concurrency":
			currentRaw = current.concurrency
		}
		currentValue, err := decimalPointer(currentRaw)
		if err != nil || currentValue == nil {
			continue
		}
		difference := new(big.Rat).Abs(new(big.Rat).Sub(wanted, currentValue))
		denominator := new(big.Rat).Abs(currentValue)
		if denominator.Sign() == 0 {
			denominator.SetInt64(1)
		}
		if new(big.Rat).Quo(difference, denominator).Cmp(threshold) < 0 {
			delete(result, field)
		}
	}
	return result
}

func changedFields(desired map[string]any, current values) map[string]any {
	result := map[string]any{}
	for field, value := range desired {
		matched := false
		switch field {
		case "schedulable":
			wanted, ok := value.(bool)
			matched = ok && current.schedulable != nil && wanted == *current.schedulable
		case "priority":
			wanted, err := integer(value)
			matched = err == nil && current.priority != nil && wanted == *current.priority
		case "concurrency":
			wanted, err := integer(value)
			matched = err == nil && current.concurrency != nil && wanted == *current.concurrency
		case "load_factor":
			wanted, err := optionalNonnegativeIntegerText(value)
			matched = err == nil && wanted != nil && current.loadFactor != nil && *wanted == *current.loadFactor
		case "status":
			wanted, ok := value.(string)
			matched = ok && current.status != nil && strings.EqualFold(strings.TrimSpace(wanted), *current.status)
		}
		if !matched {
			result[field] = value
		}
	}
	return result
}

func verifyReadback(desired map[string]any, after values) error {
	remaining := changedFields(desired, after)
	if len(remaining) > 0 {
		fields := make([]string, 0, len(remaining))
		for field := range remaining {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		return errors.New("账号自动执行后读回不一致：" + strings.Join(fields, ","))
	}
	return nil
}

func restorableBaselineValues(value business.RoutingBaseline, current values) (map[string]any, []string) {
	result := map[string]any{}
	conflicts := []string{}
	legacy := value.OwnershipVersion == 0
	if !sameBool(value.Schedulable, current.schedulable) {
		if legacy || sameBool(value.ManagedSchedulable, current.schedulable) {
			if value.Schedulable != nil {
				result["schedulable"] = *value.Schedulable
			}
		} else if value.ManagedSchedulable != nil {
			conflicts = append(conflicts, "schedulable")
		}
	}
	if !sameInt64(value.Priority, current.priority) {
		if legacy || sameInt64(value.ManagedPriority, current.priority) {
			if value.Priority != nil {
				result["priority"] = *value.Priority
			}
		} else if value.ManagedPriority != nil {
			conflicts = append(conflicts, "priority")
		}
	}
	if !sameString(value.LoadFactor, current.loadFactor) {
		if legacy || sameString(value.ManagedLoadFactor, current.loadFactor) {
			if value.LoadFactor == nil {
				result["load_factor"] = json.Number("0")
			} else {
				result["load_factor"] = json.Number(*value.LoadFactor)
			}
		} else if value.ManagedLoadFactor != nil {
			conflicts = append(conflicts, "load_factor")
		}
	}
	if !sameInt64(value.Concurrency, current.concurrency) {
		if legacy || sameInt64(value.ManagedConcurrency, current.concurrency) {
			if value.Concurrency != nil {
				result["concurrency"] = *value.Concurrency
			}
		} else if value.ManagedConcurrency != nil {
			conflicts = append(conflicts, "concurrency")
		}
	}
	if value.Status != nil && !sameString(value.Status, current.status) {
		if legacy || sameString(value.ManagedStatus, current.status) {
			result["status"] = *value.Status
		} else if value.ManagedStatus != nil {
			conflicts = append(conflicts, "status")
		}
	}
	return result, conflicts
}

func (v values) baseline(accountID string, now time.Time) business.RoutingBaseline {
	return business.RoutingBaseline{
		AccountID: accountID, Schedulable: cloneBool(v.schedulable), Priority: cloneInt(v.priority),
		LoadFactor: cloneString(v.loadFactor), Concurrency: cloneInt(v.concurrency), Status: cloneString(v.status),
		CapturedAt: now.Format(time.RFC3339Nano), OwnershipVersion: 1,
	}
}

func sameBool(left, right *bool) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func sameInt64(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func sameString(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && strings.TrimSpace(*left) == strings.TrimSpace(*right)
}

func (v values) readback(state *string) business.RoutingReadback {
	return business.RoutingReadback{
		Schedulable: cloneBool(v.schedulable), Priority: cloneInt(v.priority), LoadFactor: cloneString(v.loadFactor),
		Concurrency: cloneInt(v.concurrency), RoutingState: state,
	}
}

func (v values) asMap() map[string]any {
	return map[string]any{"schedulable": v.schedulable, "priority": v.priority, "load_factor": v.loadFactor, "concurrency": v.concurrency, "status": v.status}
}

func operation(id, kind string, target business.AccountRoutingTarget, actor string, before, after any, remote, readback bool, cause error) business.AccountOperation {
	state, phase := "succeeded", "readback"
	var detail *string
	if cause != nil {
		state = "failed"
		value := cause.Error()
		detail = &value
		if remote {
			phase = "remote-readback"
		} else {
			phase = "remote-write"
		}
	} else if remote && !readback {
		phase = "remote-write"
	} else if !remote && !readback {
		phase = "calculation"
	}
	field := "schedulable,priority,load_factor,concurrency"
	return business.AccountOperation{
		OperationID: id, OperationType: kind, State: state, Phase: phase, Actor: actor, Error: detail,
		RemoteConfirmed: remote, ReadbackConfirmed: readback, ObjectID: target.AccountID, GroupNames: target.GroupNames,
		FieldName: &field, Before: before, After: after, Writeback: remote || cause != nil,
	}
}

func (s *Service) recordOperation(ctx context.Context, operation business.AccountOperation) {
	if err := s.repository.RecordAccountOperation(ctx, operation); err != nil {
		slog.Error("调度写回操作记录保存失败", "operation_id", operation.OperationID, "account_id", operation.ObjectID, "error", err)
	}
}

func operationType(release bool) string {
	if release {
		return "routing.restore"
	}
	return "routing.writeback"
}

func optionalInteger(raw any, nonnegative bool) (*int64, error) {
	if raw == nil || strings.TrimSpace(fmt.Sprint(raw)) == "" {
		return nil, nil
	}
	value, err := integer(raw)
	if err != nil || (nonnegative && value < 0) {
		return nil, errors.New("not integer")
	}
	return &value, nil
}

func optionalNonnegativeIntegerText(raw any) (*string, error) {
	if raw == nil || strings.TrimSpace(fmt.Sprint(raw)) == "" {
		return nil, nil
	}
	value, err := integer(raw)
	if err != nil || value < 0 {
		return nil, errors.New("not nonnegative integer")
	}
	text := strconv.FormatInt(value, 10)
	return &text, nil
}

func integer(raw any) (int64, error) {
	switch value := raw.(type) {
	case int:
		return int64(value), nil
	case int64:
		return value, nil
	case json.Number:
		return value.Int64()
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) || value > math.MaxInt64 || value < math.MinInt64 {
			return 0, errors.New("not integer")
		}
		return int64(value), nil
	case *int64:
		if value == nil {
			return 0, errors.New("nil")
		}
		return *value, nil
	case *string:
		if value == nil {
			return 0, errors.New("nil")
		}
		return strconv.ParseInt(*value, 10, 64)
	default:
		return strconv.ParseInt(strings.TrimSpace(fmt.Sprint(raw)), 10, 64)
	}
}

func decimal(raw any) (*big.Rat, error) {
	if raw == nil {
		return nil, errors.New("nil decimal")
	}
	if _, ok := raw.(bool); ok {
		return nil, errors.New("bool decimal")
	}
	value, ok := new(big.Rat).SetString(strings.TrimSpace(fmt.Sprint(raw)))
	if !ok {
		return nil, errors.New("invalid decimal")
	}
	return value, nil
}

func decimalPointer(raw any) (*big.Rat, error) {
	switch value := raw.(type) {
	case *int64:
		if value == nil {
			return nil, nil
		}
		return big.NewRat(*value, 1), nil
	case *string:
		if value == nil {
			return nil, nil
		}
		return decimal(*value)
	default:
		return decimal(raw)
	}
}

func sortedTargetIDs(values map[string]business.AccountRoutingTarget) []string {
	result := make([]string, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool {
		leftNumber, leftOK := new(big.Int).SetString(result[left], 10)
		rightNumber, rightOK := new(big.Int).SetString(result[right], 10)
		if leftOK && rightOK {
			return leftNumber.Cmp(rightNumber) < 0
		}
		return result[left] < result[right]
	})
	return result
}

func randomOperationID(prefix string) (string, error) {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(value), nil
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneInt(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func failedResult(result AccountResult, err error) AccountResult {
	value := err.Error()
	result.Error = &value
	return result
}
