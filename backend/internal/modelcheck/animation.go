package modelcheck

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const animationSkill = "sub2api-model-animation"
const animationPrompt = `请生成可直接运行的单文件HTML，使用内联SVG绘制鹈鹕骑自行车的二维循环动画。画面以鹈鹕和自行车为主体，展示清晰的身体结构、踩踏动作和车轮转动，配合协调的背景、配色与层次。动画应流畅自然、衔接连续，并适配不同屏幕尺寸。可使用内联JavaScript脚本实现动画，但不得发送网络请求、加载外部脚本或引用外部资源，只输出完整HTML，不要代码围栏或解释文字。`

type AnimationTarget struct {
	AccountID string `json:"account_id"`
	Model     string `json:"model"`
	Endpoint  string `json:"endpoint,omitempty"`
	Platform  string `json:"platform,omitempty"`
}

type AnimationRequest struct {
	PrecheckQuestions []string                 `json:"precheck_questions,omitempty"`
	Mode              string                   `json:"mode,omitempty"`
	Targets           []AnimationTarget        `json:"targets"`
	TimeoutSeconds    int                      `json:"timeout_seconds"`
	Custom            *AnimationCustomEndpoint `json:"custom,omitempty"`
}

type AnimationResult struct {
	Mode                 string          `json:"mode,omitempty"`
	Precheck             *PrecheckResult `json:"precheck,omitempty"`
	AccountID            string          `json:"account_id"`
	AccountName          string          `json:"account_name"`
	Endpoint             string          `json:"endpoint,omitempty"`
	Platform             string          `json:"platform,omitempty"`
	Model                string          `json:"model"`
	ResponseModel        string          `json:"response_model,omitempty"`
	RequestID            string          `json:"request_id"`
	Status               string          `json:"status"`
	Phase                string          `json:"phase,omitempty"`
	HTML                 string          `json:"html,omitempty"`
	Source               string          `json:"source,omitempty"`
	SourceTruncated      bool            `json:"source_truncated,omitempty"`
	Prompt               string          `json:"prompt,omitempty"`
	ReasoningEffort      string          `json:"reasoning_effort,omitempty"`
	Usage                *AnimationUsage `json:"usage,omitempty"`
	GenerationDurationMS int64           `json:"generation_duration_ms,omitempty"`
	SVG                  string          `json:"svg,omitempty"`
	Error                string          `json:"error,omitempty"`
	RetryCount           int             `json:"retry_count,omitempty"`
	DurationMS           int64           `json:"duration_ms"`
	CompletedAt          string          `json:"completed_at"`
}

// cloneAnimationResult publishes an immutable task snapshot while a worker may
// still be collecting streamed usage or precheck questions.
func cloneAnimationResult(value AnimationResult) AnimationResult {
	value.Usage = cloneAnimationUsage(value.Usage)
	value.Precheck = clonePrecheckResult(value.Precheck)
	return value
}

func (s *Service) prepareAnimation(ctx context.Context, request AnimationRequest) (AnimationRequest, []selectedAccount, error) {
	if !validAnimationMode(request.Mode) {
		return request, nil, errors.New("检测内容必须为动画检测、前置检测或两者")
	}
	questions, err := normalizePrecheckQuestions(request.Mode, request.PrecheckQuestions)
	if err != nil {
		return request, nil, err
	}
	request.PrecheckQuestions = questions
	if request.TimeoutSeconds == 0 {
		request.TimeoutSeconds = 120
	}
	if request.TimeoutSeconds < 5 || request.TimeoutSeconds > 120 {
		return request, nil, errors.New("动画检测超时必须在 5 到 120 秒之间")
	}
	if request.Custom != nil {
		if request.Mode == combinedMode {
			return request, nil, errors.New("自定义接口请分别执行前置检测与动画检测")
		}
		return prepareCustomAnimation(request)
	}
	if len(request.Targets) < 1 {
		return request, nil, errors.New("动画检测请选择至少一个账号")
	}
	request.Targets = append([]AnimationTarget(nil), request.Targets...)
	ids := make([]string, len(request.Targets))
	seen := map[string]bool{}
	for i := range request.Targets {
		target := &request.Targets[i]
		// Endpoint metadata is produced only from validated custom configuration.
		target.Endpoint, target.Platform = "", ""
		target.Model = strings.TrimSpace(target.Model)
		if !stablePositiveID(target.AccountID) || seen[target.AccountID] {
			return request, nil, errors.New("账号必须使用不重复的有效稳定 ID")
		}
		if target.Model == "" || utf8.RuneCountInString(target.Model) > 256 || strings.ContainsFunc(target.Model, unicode.IsControl) {
			return request, nil, errors.New("请输入 1 到 256 个字符的有效模型 ID")
		}
		seen[target.AccountID] = true
		ids[i] = target.AccountID
	}
	accounts, err := s.selectCheckAccounts(ctx, ids)
	if err != nil {
		return request, nil, err
	}
	for _, account := range accounts {
		if account.AccountType == "oauth" {
			if account.Platform != "openai" {
				return request, nil, fmt.Errorf("账号 %s 暂不支持 OAuth 检测，请选择 OpenAI OAuth 账号", account.ID)
			}
			continue
		}
		if account.CredentialError != "" {
			return request, nil, fmt.Errorf("账号 %s：%s", account.ID, account.CredentialError)
		}
		if account.Platform != "openai" && account.Platform != "anthropic" && account.Platform != "" {
			return request, nil, fmt.Errorf("账号 %s 暂不支持动画检测，请使用 OpenAI 或 Anthropic 接口账号", account.ID)
		}
	}
	return request, accounts, nil
}

func (s *Service) EnqueueAnimation(ctx context.Context, request AnimationRequest) (taskstore.Task, error) {
	request, accounts, err := s.prepareAnimation(ctx, request)
	if err != nil {
		return taskstore.Task{}, err
	}
	oauthTarget, err := s.prepareOAuthTarget(ctx, accounts)
	if err != nil {
		return taskstore.Task{}, err
	}
	s.animation.activeMu.Lock()
	for _, target := range request.Targets {
		if s.animation.active[target.AccountID] {
			s.animation.activeMu.Unlock()
			return taskstore.Task{}, errors.New("所选账号正在执行动画检测，请等待完成后重试")
		}
	}
	for _, target := range request.Targets {
		s.animation.active[target.AccountID] = true
	}
	s.animation.activeMu.Unlock()
	release := func() {
		s.animation.activeMu.Lock()
		for _, target := range request.Targets {
			delete(s.animation.active, target.AccountID)
		}
		s.animation.activeMu.Unlock()
		s.animation.scheduleMu.Lock()
		for _, target := range request.Targets {
			for _, item := range splitAnimationSchedule(AnimationSchedule{AccountID: target.AccountID, Mode: request.Mode}) {
				key := animationScheduleKey(item.AccountID, item.Mode)
				if schedule, ok := s.animation.schedules[key]; ok && schedule.ScheduleType != "daily" {
					s.animation.next[key] = nextAnimationTime(schedule, time.Now())
				}
			}
		}
		s.animation.scheduleMu.Unlock()
	}
	id, err := randomTaskID()
	if err != nil {
		release()
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ids := make([]string, len(request.Targets))
	for i, target := range request.Targets {
		ids[i] = target.AccountID
	}
	task := taskstore.Task{ID: id, Skill: animationSkill, Operation: "account-model-animation", Status: "queued", Message: "动画生成检测已排队", CreatedAt: now, UpdatedAt: now,
		Result: map[string]any{"account_ids": ids, "targets": request.Targets, "completed": 0, "total": len(accounts), "animations": []AnimationResult{}, "remote_write": false}}
	if request.Mode == precheckMode {
		task.Operation, task.Message = "account-model-precheck", "前置检测已排队"
		task.Result["mode"] = precheckMode
	}
	if request.Mode == combinedMode {
		task.Operation, task.Message = "account-model-combined", "前置与动画检测已排队"
		task.Result["mode"], task.Result["total"] = combinedMode, 2*len(accounts)
	}
	if err := s.prepareRecovery(ctx, &task, detectionRecovery{Request: request}, accounts); err != nil {
		release()
		return taskstore.Task{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		release()
		return taskstore.Task{}, err
	}
	if err := taskrunner.GoTask(s.taskRunner, id, func(parent context.Context) {
		defer release()
		if oauthTarget != nil {
			parent = targetguard.Expect(parent, *oauthTarget)
		}
		s.executeAnimation(parent, task, request, accounts)
	}); err != nil {
		release()
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) AnimationHistory(ctx context.Context) ([]taskstore.Task, error) {
	tasks, err := s.tasks.ListBySkill(ctx, animationSkill, 20)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Result = map[string]any{}
	}
	return tasks, nil
}

func (s *Service) executeAnimation(parent context.Context, task taskstore.Task, request AnimationRequest, accounts []selectedAccount) {
	ctx, cancel := context.WithTimeout(parent, s.taskTimeout)
	defer cancel()
	runStarted := time.Now()
	runStartedAt := runStarted.UTC().Format(time.RFC3339Nano)
	setRunTiming := func(completedAt time.Time) {
		if task.Result == nil {
			task.Result = map[string]any{}
		}
		task.Result["started_at"] = runStartedAt
		task.Result["completed_at"] = completedAt.UTC().Format(time.RFC3339Nano)
		task.Result["duration_ms"] = completedAt.Sub(runStarted).Milliseconds()
	}
	task.Status, task.Message = "running", "正在生成鹈鹕骑自行车动画"
	if request.Mode == precheckMode {
		task.Message = "正在执行前置检测"
	}
	if request.Mode == combinedMode {
		task.Message = "正在依次执行前置与动画检测"
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	modes := []string{request.Mode}
	unit := "个账号"
	if request.Mode == combinedMode {
		modes = []string{precheckMode, "animation"}
		unit = "项检测"
	}
	total := len(accounts) * len(modes)
	completed := previousResults[AnimationResult](task, "animations")
	done := map[string]bool{}
	succeeded := 0
	for _, result := range completed {
		if result.Phase != "" && result.Phase != "completed" {
			continue
		}
		done[animationResultKey(result.AccountID, result.Mode)] = true
		if result.Status == "succeeded" {
			succeeded++
		}
	}
	if err := s.persistAnimationEvidence(ctx, task.ID, completed); err != nil {
		slog.Error("保存动画检测证据失败", "task_id", task.ID, "error", err)
	}
	results := make(chan AnimationResult, total*8)
	var workers sync.WaitGroup
	for index, account := range accounts {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for _, mode := range modes {
				if done[animationResultKey(account.ID, mode)] {
					continue
				}
				if ctx.Err() != nil {
					return
				}
				result := AnimationResult{AccountID: account.ID, AccountName: account.Name, Model: request.Targets[index].Model, RequestID: fmt.Sprintf("%s-%s", task.ID, account.ID), Status: "failed"}
				if request.Custom != nil {
					// Retain validated endpoint metadata, never the execution credential.
					result.Endpoint = request.Custom.BaseURL
					result.Platform = request.Custom.Platform
				}
				result.Mode = mode
				if request.Mode == combinedMode {
					result.RequestID += "-" + mode
				}
				result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
				result.Phase = "queued"
				results <- cloneAnimationResult(result)
				err := s.runAnimationWithRetry(ctx, account, request.TimeoutSeconds, request.Custom, request.PrecheckQuestions, &result, func() {
					results <- cloneAnimationResult(result)
				})
				if err != nil && taskstore.Interrupted(ctx) {
					return
				}
				result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
				if err != nil {
					if request.Custom != nil {
						err = errors.New(strings.ReplaceAll(err.Error(), request.Custom.APIKey, "[已隐藏]"))
					}
					result.Error = safeCredentialError(err)
				} else {
					result.Status = "succeeded"
				}
				result.Phase = "completed"
				results <- cloneAnimationResult(result)
			}
		}()
	}
	go func() { workers.Wait(); close(results) }()
	activeResults := make(map[string]AnimationResult, total)
	for _, result := range completed {
		if result.Phase != "" && result.Phase != "completed" {
			continue
		}
		activeResults[animationResultKey(result.AccountID, result.Mode)] = result
	}
	for result := range results {
		key := animationResultKey(result.AccountID, result.Mode)
		if result.Phase == "completed" {
			activeResults[key] = cloneAnimationResult(result)
			if err := s.persistAnimationEvidence(ctx, task.ID, []AnimationResult{result}); err != nil {
				slog.Error("保存动画检测证据失败", "task_id", task.ID, "account_id", result.AccountID, "mode", result.Mode, "error", err)
			}
			completed = completed[:0]
			for _, item := range activeResults {
				if item.Phase == "completed" {
					completed = append(completed, item)
				}
			}
			succeeded = 0
			for _, item := range completed {
				if item.Status == "succeeded" {
					succeeded++
				}
			}
			task.Progress = len(completed) * 100 / total
			task.Message = fmt.Sprintf("%s已完成 %d/%d %s", animationModeLabel(request.Mode), len(completed), total, unit)
		} else {
			activeResults[key] = result
			task.Message = fmt.Sprintf("%s执行中：%d/%d %s", animationModeLabel(request.Mode), len(completed), total, unit)
		}
		// Copy results to keep previously published task snapshots immutable.
		allResults := make([]AnimationResult, 0, len(activeResults))
		for _, item := range activeResults {
			allResults = append(allResults, item)
		}
		task.Result = map[string]any{"account_ids": task.Result["account_ids"], "targets": request.Targets, "completed": len(completed), "total": total, "animations": allResults, "remote_write": false}
		if request.Mode == precheckMode || request.Mode == combinedMode {
			task.Result["mode"] = request.Mode
		}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	task.Status = "succeeded"
	if succeeded == 0 {
		task.Status = "failed"
	} else if succeeded < total {
		task.Status = "partial"
	}
	task.Message = fmt.Sprintf("%s完成：成功 %d，失败 %d", animationModeLabel(request.Mode), succeeded, total-succeeded)
	setRunTiming(time.Now())
	taskstore.MarkCancelled(ctx, &task, animationModeLabel(request.Mode)+"已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) runAnimationTarget(ctx context.Context, account selectedAccount, timeout int, custom *AnimationCustomEndpoint, questions []string, result *AnimationResult, onPhase ...func()) (time.Duration, error) {
	releaseSlot, err := s.reserveAnimationSlot(ctx)
	if err != nil {
		return 0, errors.New("动画检测已取消或任务超时")
	}
	started := time.Now()
	defer releaseSlot()
	result.Phase = "running"
	for _, notify := range onPhase {
		if notify != nil {
			notify()
		}
	}
	if custom == nil && account.AccountType == "oauth" {
		err := s.runOAuthAnimationTarget(ctx, account, timeout, questions, result, onPhase...)
		return time.Since(started), err
	}
	guarded, release, credential, err := s.animationCredential(ctx, account, custom)
	if err != nil {
		return time.Since(started), err
	}
	defer release()
	if result.Mode == precheckMode {
		client := &http.Client{Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		err := runPrecheckTarget(guarded, client, credential, timeout, questions, result)
		return time.Since(started), err
	}
	requestCtx, cancel, firstOutputTimedOut, markFirstOutput := firstOutputTimeout(guarded, timeout)
	defer cancel()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	text, model, err := sendAnimation(requestCtx, client, credential, result.Model, result.RequestID, result.Usage, func() {
		markFirstOutput()
		result.Phase = "generating"
		for _, notify := range onPhase {
			if notify != nil {
				notify()
			}
		}
	})
	if firstOutputTimedOut() && err != nil {
		err = context.DeadlineExceeded
	}
	if err != nil {
		return time.Since(started), err
	}
	if strings.Contains(text, credential.Secret) {
		return time.Since(started), errors.New("上游生成内容包含敏感信息，已拒绝展示")
	}
	err = setAnimationArtifact(result, text)
	if err != nil {
		return time.Since(started), err
	}
	result.ResponseModel = safeCredentialText(strings.ReplaceAll(model, credential.Secret, "[已隐藏]"))
	return time.Since(started), nil
}
