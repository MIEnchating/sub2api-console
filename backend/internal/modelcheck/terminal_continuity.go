package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const terminalContinuitySkill = "sub2api-terminal-continuity"

const terminalContinuityPrompt = `这是一次对话上下文续接检测。请把下列已记录的会话事实视为当前上下文，而不是要求你实际执行命令：
- 当前编码任务仍在进行中。
- 当前工具列表明确包含 exec_command，可读取工作区和执行只读 Git 命令。
- 上一轮已经用该工具读取过仓库状态。

用户现在要求继续检查仓库。请只输出下一步工具调用计划的 JSON，不要添加 Markdown 或解释：
{"tool":"exec_command","command":"git status --short"}

不要声称当前对话没有终端、exec、Git、文件系统或工具权限。`

type TerminalContinuityRequest struct {
	Rounds         int               `json:"rounds,omitempty"`
	Targets        []AnimationTarget `json:"targets"`
	TimeoutSeconds int               `json:"timeout_seconds"`
}

type TerminalContinuityResult struct {
	RoundResults  []TerminalContinuityRound `json:"round_results,omitempty"`
	AccountID     string                    `json:"account_id"`
	AccountName   string                    `json:"account_name"`
	Model         string                    `json:"model"`
	ResponseModel string                    `json:"response_model,omitempty"`
	RequestID     string                    `json:"request_id"`
	Verdict       string                    `json:"verdict"`
	Response      string                    `json:"response,omitempty"`
	Error         string                    `json:"error,omitempty"`
	DurationMS    int64                     `json:"duration_ms"`
	CompletedAt   string                    `json:"completed_at"`
}

func (s *Service) TerminalContinuityHistory(ctx context.Context) ([]taskstore.Task, error) {
	values, err := s.tasks.ListBySkill(ctx, terminalContinuitySkill, 20)
	if err != nil {
		return nil, err
	}
	managed, err := s.tasks.ListBySkill(ctx, animationSkill, 200)
	if err != nil {
		return nil, err
	}
	for _, task := range managed {
		if task.Operation != "managed-model-detection" {
			continue
		}
		enabled := false
		switch configuration := task.Result["configuration"].(type) {
		case DetectionTask:
			enabled = configuration.Terminal
		case map[string]any:
			enabled, _ = configuration["terminal"].(bool)
		}
		if enabled {
			values = append(values, task)
		}
	}
	slices.SortFunc(values, func(a, b taskstore.Task) int {
		left, _ := time.Parse(time.RFC3339Nano, a.CreatedAt)
		right, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
		return right.Compare(left)
	})
	if len(values) > 20 {
		values = values[:20]
	}
	return values, nil
}

func (s *Service) EnqueueTerminalContinuity(ctx context.Context, input TerminalContinuityRequest) (taskstore.Task, error) {
	if err := validateTerminalRounds(input.Rounds); err != nil {
		return taskstore.Task{}, err
	}
	if input.Rounds == 0 {
		input.Rounds = 1
	}
	prepared, accounts, err := s.prepareAnimation(ctx, AnimationRequest{Targets: input.Targets, TimeoutSeconds: input.TimeoutSeconds})
	if err != nil {
		return taskstore.Task{}, errors.New(strings.ReplaceAll(err.Error(), "动画检测", "终端续接检测"))
	}
	oauthTarget, err := s.prepareOAuthTarget(ctx, accounts)
	if err != nil {
		return taskstore.Task{}, err
	}
	s.terminalMu.Lock()
	for _, target := range prepared.Targets {
		if s.terminalActive[target.AccountID] {
			s.terminalMu.Unlock()
			return taskstore.Task{}, errors.New("所选账号正在执行终端续接检测，请等待完成后重试")
		}
	}
	for _, target := range prepared.Targets {
		s.terminalActive[target.AccountID] = true
	}
	s.terminalMu.Unlock()
	release := func() {
		s.terminalMu.Lock()
		for _, target := range prepared.Targets {
			delete(s.terminalActive, target.AccountID)
		}
		s.terminalMu.Unlock()
	}
	id, err := randomTaskID()
	if err != nil {
		release()
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ids := make([]string, len(prepared.Targets))
	for i, target := range prepared.Targets {
		ids[i] = target.AccountID
	}
	task := taskstore.Task{ID: id, Skill: terminalContinuitySkill, Operation: "account-terminal-continuity", Status: "queued", Progress: 0, Message: "终端续接检测已排队", CreatedAt: now, UpdatedAt: now,
		Result: map[string]any{"account_ids": ids, "targets": prepared.Targets, "rounds": input.Rounds, "completed": 0, "total": len(accounts), "checks": []TerminalContinuityResult{}, "remote_write": false}}
	if err := s.tasks.Save(ctx, task); err != nil {
		release()
		return taskstore.Task{}, err
	}
	if err := taskrunner.GoTask(s.taskRunner, id, func(parent context.Context) {
		defer release()
		if oauthTarget != nil {
			parent = targetguard.Expect(parent, *oauthTarget)
		}
		s.executeTerminalContinuity(parent, task, prepared, accounts, input.Rounds)
	}); err != nil {
		release()
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) executeTerminalContinuity(parent context.Context, task taskstore.Task, input AnimationRequest, accounts []selectedAccount, rounds int) {
	ctx, cancel := context.WithTimeout(parent, s.taskTimeout)
	defer cancel()
	task.Status, task.Message = "running", "正在检测对话中的终端工具续接"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	results := make(chan TerminalContinuityResult, len(accounts))
	var workers sync.WaitGroup
	for index, account := range accounts {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result := s.runTerminalRounds(ctx, account, input.TimeoutSeconds, input.Targets[index].Model, fmt.Sprintf("%s-%s", task.ID, account.ID), rounds)
			results <- result
		}()
	}
	go func() { workers.Wait(); close(results) }()
	completed := make([]TerminalContinuityResult, 0, len(accounts))
	successful := 0
	for result := range results {
		completed = append(completed, result)
		if result.Verdict != "error" {
			successful++
		}
		task.Progress = len(completed) * 100 / len(accounts)
		task.Message = fmt.Sprintf("终端续接检测已完成 %d/%d 个账号", len(completed), len(accounts))
		task.Result = map[string]any{"account_ids": task.Result["account_ids"], "targets": input.Targets, "rounds": rounds, "completed": len(completed), "total": len(accounts), "checks": append([]TerminalContinuityResult(nil), completed...), "remote_write": false}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	task.Status = "succeeded"
	if successful == 0 {
		task.Status = "failed"
	} else if successful < len(accounts) {
		task.Status = "partial"
	}
	task.Message = fmt.Sprintf("终端续接检测完成：有效结果 %d，失败 %d", successful, len(accounts)-successful)
	taskstore.MarkCancelled(ctx, &task, "终端续接检测已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) runTerminalContinuityTarget(ctx context.Context, account selectedAccount, timeout int, model, requestID string) (string, string, error) {
	select {
	case s.animation.slots <- struct{}{}:
	case <-ctx.Done():
		return "", "", errors.New("终端续接检测已取消或任务超时")
	}
	defer func() { <-s.animation.slots }()
	if account.AccountType == "oauth" {
		return s.runOAuthPromptTarget(ctx, account, timeout, model, requestID, terminalContinuityPrompt)
	}
	guarded, release, credential, err := s.animationCredential(ctx, account, nil)
	if err != nil {
		return "", "", err
	}
	defer release()
	sender := directBundleSender{client: &http.Client{Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, credential: credential, requestID: requestID}
	effort := ""
	if model == astraModel {
		effort = "low"
	}
	text, responseModel, err := sender.SendWithReasoning(guarded, account.ID, model, terminalContinuityPrompt, timeout, effort)
	if err == nil && credential.Secret != "" && (strings.Contains(text, credential.Secret) || strings.Contains(responseModel, credential.Secret)) {
		return "", "", errors.New("终端续接检测返回包含敏感信息，已拒绝保存")
	}
	return text, responseModel, err
}

func classifyTerminalContinuity(text string) string {
	trimmed := strings.TrimSpace(text)
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	var plan struct {
		Tool    string `json:"tool"`
		Command string `json:"command"`
	}
	if decoder.Decode(&plan) == nil {
		var trailing any
		if decoder.Decode(&trailing) == io.EOF && plan.Tool == "exec_command" && plan.Command == "git status --short" {
			return "normal"
		}
	}
	lower := strings.ToLower(trimmed)
	for _, phrase := range []string{
		"没有可调用", "没有终端", "没有可用的终端", "无终端", "终端不可用", "无法访问终端", "无法使用终端",
		"没有文件系统", "没有可用的文件系统", "无法访问文件系统", "没有工具入口", "没有可用的工具", "工具不可用", "未暴露终端", "无法实际读取仓库",
		"no terminal", "no shell", "no filesystem", "no exec", "cannot access the terminal", "can't access the terminal",
		"do not have access to the terminal", "don't have access to the terminal", "do not have access to tools", "don't have access to tools", "tools are not available", "tools aren't available",
	} {
		if strings.Contains(lower, phrase) {
			return "suspected"
		}
	}
	return "inconclusive"
}
