package modelcheck

import (
	"context"
	"errors"
	"fmt"
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
const animationPrompt = `生成一幅鹈鹕骑自行车的 SVG 动画。画面需包含清晰的鹈鹕、车架、两个旋转的车轮和蹬踏动作。使用 viewBox="0 0 640 400"，背景简洁，动画循环播放。只返回一个完整的 SVG 元素，不要 Markdown、解释、脚本、foreignObject、外部资源或链接。仅使用 SVG 图形及 animate/animateTransform 实现动画。不要 style 元素或 style 属性，颜色与线条使用 SVG 表现属性，输出不超过 24 KB。`

type AnimationTarget struct {
	AccountID string `json:"account_id"`
	Model     string `json:"model"`
}

type AnimationRequest struct {
	PrecheckQuestions []string                 `json:"precheck_questions,omitempty"`
	Mode              string                   `json:"mode,omitempty"`
	Targets           []AnimationTarget        `json:"targets"`
	TimeoutSeconds    int                      `json:"timeout_seconds"`
	Custom            *AnimationCustomEndpoint `json:"custom,omitempty"`
}

type AnimationResult struct {
	Mode          string          `json:"mode,omitempty"`
	Precheck      *PrecheckResult `json:"precheck,omitempty"`
	AccountID     string          `json:"account_id"`
	AccountName   string          `json:"account_name"`
	Model         string          `json:"model"`
	ResponseModel string          `json:"response_model,omitempty"`
	RequestID     string          `json:"request_id"`
	Status        string          `json:"status"`
	SVG           string          `json:"svg,omitempty"`
	Error         string          `json:"error,omitempty"`
	RetryCount    int             `json:"retry_count,omitempty"`
	DurationMS    int64           `json:"duration_ms"`
	CompletedAt   string          `json:"completed_at"`
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
	if len(request.Targets) < 1 || len(request.Targets) > 20 {
		return request, nil, errors.New("动画检测请选择 1 到 20 个账号")
	}
	request.Targets = append([]AnimationTarget(nil), request.Targets...)
	ids := make([]string, len(request.Targets))
	seen := map[string]bool{}
	for i := range request.Targets {
		target := &request.Targets[i]
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
			if schedule, ok := s.animation.schedules[target.AccountID]; ok {
				s.animation.next[target.AccountID] = time.Now().Add(time.Duration(schedule.IntervalMinutes) * time.Minute)
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
	results := make(chan AnimationResult, total)
	var workers sync.WaitGroup
	for index, account := range accounts {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for phase, mode := range modes {
				if phase > 0 && ctx.Err() != nil {
					return
				}
				result := AnimationResult{AccountID: account.ID, AccountName: account.Name, Model: request.Targets[index].Model, RequestID: fmt.Sprintf("%s-%s", task.ID, account.ID), Status: "failed"}
				result.Mode = mode
				if request.Mode == combinedMode {
					result.RequestID += "-" + mode
				}
				started := time.Now()
				err := s.runAnimationWithRetry(ctx, account, request.TimeoutSeconds, request.Custom, request.PrecheckQuestions, &result)
				result.DurationMS = time.Since(started).Milliseconds()
				result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
				if err != nil {
					if request.Custom != nil {
						err = errors.New(strings.ReplaceAll(err.Error(), request.Custom.APIKey, "[已隐藏]"))
					}
					result.Error = safeCredentialError(err)
				} else {
					result.Status = "succeeded"
				}
				results <- result
			}
		}()
	}
	go func() { workers.Wait(); close(results) }()
	completed := make([]AnimationResult, 0, total)
	succeeded := 0
	for result := range results {
		completed = append(completed, result)
		if result.Status == "succeeded" {
			succeeded++
		}
		task.Progress = len(completed) * 100 / total
		task.Message = fmt.Sprintf("%s已完成 %d/%d %s", animationModeLabel(request.Mode), len(completed), total, unit)
		// Copy results to keep previously published task snapshots immutable.
		task.Result = map[string]any{"account_ids": task.Result["account_ids"], "targets": request.Targets, "completed": len(completed), "total": total, "animations": append([]AnimationResult(nil), completed...), "remote_write": false}
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
	taskstore.MarkCancelled(ctx, &task, animationModeLabel(request.Mode)+"已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) runAnimationTarget(ctx context.Context, account selectedAccount, timeout int, custom *AnimationCustomEndpoint, questions []string, result *AnimationResult) error {
	select {
	case s.animation.slots <- struct{}{}:
	case <-ctx.Done():
		return errors.New("动画检测已取消或任务超时")
	}
	defer func() { <-s.animation.slots }()
	if custom == nil && account.AccountType == "oauth" {
		return s.runOAuthAnimationTarget(ctx, account, timeout, questions, result)
	}
	guarded, release, credential, err := s.animationCredential(ctx, account, custom)
	if err != nil {
		return err
	}
	defer release()
	if result.Mode == precheckMode {
		client := &http.Client{Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		return runPrecheckTarget(guarded, client, credential, timeout, questions, result)
	}
	requestCtx, cancel := context.WithTimeout(guarded, time.Duration(timeout)*time.Second)
	defer cancel()
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	text, model, err := sendAnimation(requestCtx, client, credential, result.Model, result.RequestID)
	if err != nil {
		return err
	}
	if strings.Contains(text, credential.Secret) {
		return errors.New("上游生成内容包含敏感信息，已拒绝展示")
	}
	svg, err := sanitizeAnimationSVG(text)
	if err != nil {
		return err
	}
	result.SVG, result.ResponseModel = svg, safeCredentialText(strings.ReplaceAll(model, credential.Secret, "[已隐藏]"))
	return nil
}
