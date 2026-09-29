package modelcheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type detectionRecovery struct {
	Request       AnimationRequest  `json:"request"`
	Rounds        int               `json:"rounds,omitempty"`
	Configuration *DetectionTask    `json:"configuration,omitempty"`
	Target        string            `json:"target,omitempty"`
	Bindings      map[string]string `json:"bindings,omitempty"`
}

func accountFingerprint(account selectedAccount) string {
	account.Name = ""
	account.CredentialError = ""
	raw, _ := json.Marshal(account)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (s *Service) prepareRecovery(ctx context.Context, task *taskstore.Task, input detectionRecovery, accounts []selectedAccount) error {
	if input.Request.Custom != nil {
		return nil
	}
	if store, ok := s.credentials.(targetguard.Store); ok {
		target, err := targetguard.Expected(ctx, store)
		if err != nil {
			return err
		}
		input.Target = targetguard.Fingerprint(target)
	}
	input.Bindings = map[string]string{}
	for _, account := range accounts {
		input.Bindings[account.ID] = accountFingerprint(account)
	}
	return taskstore.WithRecovery(task, input)
}

func (s *Service) ResumeTask(ctx context.Context, task taskstore.Task) error {
	var input detectionRecovery
	if err := taskstore.RecoveryInput(task, &input); err != nil {
		return err
	}
	if input.Request.Custom != nil {
		return errors.New("自定义接口的临时密钥不能用于自动恢复，请重新提交")
	}
	if input.Target != "" {
		store, ok := s.credentials.(targetguard.Store)
		if !ok {
			return targetguard.ErrChanged
		}
		target, err := targetguard.Expected(ctx, store)
		if err != nil {
			return err
		}
		if targetguard.Fingerprint(target) != input.Target {
			return targetguard.ErrChanged
		}
		ctx = targetguard.Expect(ctx, target)
	}
	if input.Configuration != nil {
		return s.resumeManagedTask(ctx, task, input)
	}
	request, accounts, err := s.prepareAnimation(ctx, input.Request)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if input.Bindings[account.ID] != accountFingerprint(account) {
			return errors.New("账号绑定在任务中断后已变化，请重新提交")
		}
	}
	release, err := s.reserveDetectionAccounts(accounts)
	if err != nil {
		return taskrecovery.ErrBusy
	}
	target, err := s.prepareOAuthTarget(ctx, accounts)
	if err != nil {
		release()
		return err
	}
	err = taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) {
		defer release()
		if target != nil {
			parent = targetguard.Expect(parent, *target)
		}
		if task.Operation == "account-terminal-continuity" {
			s.executeTerminalContinuity(parent, task, request, accounts, input.Rounds)
		} else {
			s.executeAnimation(parent, task, request, accounts)
		}
	})
	if err != nil {
		release()
	}
	return err
}

type reservedDetectionKey struct{}

func (s *Service) resumeManagedTask(ctx context.Context, task taskstore.Task, input detectionRecovery) error {
	state := s.detectionTasks
	state.mu.Lock()
	value, ok := state.values[input.Configuration.ID]
	if !ok || value.Version != input.Configuration.Version {
		state.mu.Unlock()
		return errors.New("检测任务配置已变化或已删除，请重新提交")
	}
	if state.active[value.ID] != "" {
		state.mu.Unlock()
		return taskrecovery.ErrBusy
	}
	value = cloneDetectionTask(value)
	state.active[value.ID] = task.ID
	state.last[value.ID] = task.ID
	state.mu.Unlock()
	releaseState := func() {
		state.mu.Lock()
		delete(state.active, value.ID)
		if current, ok := state.values[value.ID]; ok && current.Version == value.Version && current.ScheduleType != "daily" {
			state.next[value.ID] = nextAnimationTime(detectionTiming(current), time.Now())
		}
		state.mu.Unlock()
	}
	scope, err := state.repository.DetectionTaskScope(ctx, value.GroupIDs)
	if err != nil {
		releaseState()
		return err
	}
	_, accounts, err := s.prepareDetectionTask(ctx, value, scope.AccountIDs)
	if err != nil {
		releaseState()
		return err
	}
	if err := bindDetectionRecovery(&task, accounts); err != nil {
		releaseState()
		return err
	}
	releaseAccounts, err := s.reserveDetectionAccounts(accounts)
	if err != nil {
		releaseState()
		return taskrecovery.ErrBusy
	}
	release := func() { releaseAccounts(); releaseState() }
	target, err := s.prepareOAuthTarget(ctx, accounts)
	if err != nil {
		release()
		return err
	}
	err = taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) {
		defer release()
		if target != nil {
			parent = targetguard.Expect(parent, *target)
		}
		parent = context.WithValue(parent, reservedDetectionKey{}, true)
		s.executeDetectionTask(parent, task, value)
	})
	if err != nil {
		release()
	}
	return err
}

func previousResults[T any](task taskstore.Task, key string) []T {
	raw, err := json.Marshal(task.Result[key])
	if err != nil {
		return nil
	}
	var values []T
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	return values
}

func animationResultKey(accountID, mode string) string {
	if mode == "" {
		mode = "animation"
	}
	return accountID + ":" + mode
}

func upsertTerminal(values []TerminalContinuityResult, result TerminalContinuityResult) []TerminalContinuityResult {
	for i, value := range values {
		if value.AccountID == result.AccountID {
			values[i] = result
			return values
		}
	}
	return append(values, result)
}
func terminalCounts(values []TerminalContinuityResult) (int, int) {
	complete, success := 0, 0
	for _, value := range values {
		if !value.Incomplete {
			complete++
			if value.Verdict != "error" {
				success++
			}
		}
	}
	return complete, success
}

func bindDetectionRecovery(task *taskstore.Task, accounts []selectedAccount) error {
	if task.Recovery == nil {
		return nil
	}
	var input detectionRecovery
	if err := taskstore.RecoveryInput(*task, &input); err != nil {
		return err
	}
	if len(input.Bindings) > 0 {
		if len(input.Bindings) != len(accounts) {
			return errors.New("检测分组成员在中断后已变化，请重新提交")
		}
		for _, account := range accounts {
			if input.Bindings[account.ID] != accountFingerprint(account) {
				return errors.New("检测账号或绑定在中断后已变化，请重新提交")
			}
		}
		return nil
	}
	input.Bindings = map[string]string{}
	for _, account := range accounts {
		input.Bindings[account.ID] = accountFingerprint(account)
	}
	return taskstore.WithRecovery(task, input)
}
