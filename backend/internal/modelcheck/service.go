package modelcheck

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type SettingsStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type AccountCatalog interface {
	Accounts(context.Context) ([]business.AccountStatus, error)
}

type Request struct {
	AccountIDs     []string
	Models         []string
	Rounds         int
	TimeoutSeconds int
}

type Capabilities struct {
	ClaudeStandards []string `json:"claude_standards"`
	SolModels       []string `json:"sol_models"`
}

type selectedAccount struct {
	ID   string
	Name string
}

type preparedRun struct {
	request  Request
	settings configstore.TargetSettings
	accounts []selectedAccount
}

type Service struct {
	tasks          TaskStore
	settings       SettingsStore
	accounts       AccountCatalog
	claudeProfiles map[string]claudeProfile
	solProfile     solProfile
	taskTimeout    time.Duration
}

func New(tasks TaskStore, settings SettingsStore, accounts AccountCatalog) (*Service, error) {
	if tasks == nil || settings == nil || accounts == nil {
		return nil, errors.New("模型检测任务依赖尚未就绪")
	}
	claudeProfiles, err := loadClaudeProfiles()
	if err != nil {
		return nil, err
	}
	sol, err := loadSolProfile()
	if err != nil {
		return nil, err
	}
	return &Service{
		tasks: tasks, settings: settings, accounts: accounts,
		claudeProfiles: claudeProfiles, solProfile: sol, taskTimeout: 30 * time.Minute,
	}, nil
}

func (s *Service) Capabilities() Capabilities {
	standards := make([]string, 0, len(s.claudeProfiles))
	for standard := range s.claudeProfiles {
		standards = append(standards, standard)
	}
	sort.Strings(standards)
	return Capabilities{ClaudeStandards: standards, SolModels: append([]string(nil), s.solProfile.Models...)}
}

func (s *Service) Enqueue(ctx context.Context, request Request) (taskstore.Task, error) {
	prepared, err := s.prepare(ctx, request)
	if err != nil {
		return taskstore.Task{}, err
	}
	id, err := randomTaskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-model-check", Operation: "account-model-behavior-check",
		Status: "queued", Progress: 0, Message: "账号模型检测已排队",
		Result: map[string]any{"credentials_persisted": false}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go s.execute(task, prepared)
	return task, nil
}

func (s *Service) prepare(ctx context.Context, request Request) (preparedRun, error) {
	request.AccountIDs = normalizedUnique(request.AccountIDs)
	request.Models = normalizedUnique(request.Models)
	if len(request.AccountIDs) == 0 || len(request.AccountIDs) > 20 {
		return preparedRun{}, errors.New("请选择 1 到 20 个账号")
	}
	if len(request.Models) == 0 || len(request.Models) > 20 {
		return preparedRun{}, errors.New("请选择 1 到 20 个模型")
	}
	if len(request.AccountIDs)*len(request.Models) > 100 {
		return preparedRun{}, errors.New("单次检测不能超过 100 个账号模型组合")
	}
	for _, accountID := range request.AccountIDs {
		if !stablePositiveID(accountID) {
			return preparedRun{}, errors.New("账号必须使用有效的稳定 ID")
		}
	}
	for _, model := range request.Models {
		if utf8.RuneCountInString(model) > 256 || s.checkerForModel(model) == "" {
			return preparedRun{}, fmt.Errorf("模型 %s 暂无行为检测画像", model)
		}
	}
	if request.Rounds == 0 {
		request.Rounds = 1
	}
	if request.Rounds < 1 || request.Rounds > 3 {
		return preparedRun{}, errors.New("检测轮次必须在 1 到 3 之间")
	}
	if request.TimeoutSeconds == 0 {
		request.TimeoutSeconds = 45
	}
	if request.TimeoutSeconds < 5 || request.TimeoutSeconds > 120 {
		return preparedRun{}, errors.New("单次请求超时必须在 5 到 120 秒之间")
	}
	settings, err := s.settings.TargetSettings(ctx)
	if err != nil {
		return preparedRun{}, errors.New("管理连接配置不可用：" + err.Error())
	}
	rows, err := s.accounts.Accounts(ctx)
	if err != nil {
		return preparedRun{}, errors.New("账号列表读取失败")
	}
	byID := make(map[string]business.AccountStatus, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	selected := make([]selectedAccount, 0, len(request.AccountIDs))
	for _, accountID := range request.AccountIDs {
		row, found := byID[accountID]
		if !found {
			return preparedRun{}, fmt.Errorf("账号 %s 不存在", accountID)
		}
		selected = append(selected, selectedAccount{ID: row.ID, Name: row.Name})
	}
	return preparedRun{request: request, settings: settings, accounts: selected}, nil
}

func (s *Service) checkerForModel(model string) string {
	if inferClaudeStandard(model, s.claudeProfiles) != "" {
		return "claude"
	}
	for _, candidate := range s.solProfile.Models {
		if model == candidate {
			return "sol"
		}
	}
	return ""
}

func (s *Service) execute(task taskstore.Task, prepared preparedRun) {
	ctx, cancel := context.WithTimeout(context.Background(), s.taskTimeout)
	defer cancel()
	client, err := adminclient.New(adminclient.Config{
		BaseURL: prepared.settings.BaseURL, AdminKey: prepared.settings.AdminKey,
		Timeout: time.Duration(prepared.request.TimeoutSeconds) * time.Second, Attempts: 1,
	}, nil)
	if err != nil {
		s.finishFailed(task, err)
		return
	}
	task.Status, task.Progress, task.Message = "running", 5, "正在执行账号与模型检测矩阵"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistProgress(s.tasks, task)

	type combination struct {
		account selectedAccount
		model   string
	}
	combinations := make([]combination, 0, len(prepared.accounts)*len(prepared.request.Models))
	for _, account := range prepared.accounts {
		for _, model := range prepared.request.Models {
			combinations = append(combinations, combination{account: account, model: model})
		}
	}
	type outcome struct {
		index  int
		result map[string]any
	}
	jobs := make(chan int)
	outcomes := make(chan outcome, len(combinations))
	workers := min(2, len(combinations))
	var workerGroup sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			for index := range jobs {
				current := combinations[index]
				input := targetRequest{
					AccountID: current.account.ID, AccountName: current.account.Name,
					Model: current.model, Rounds: prepared.request.Rounds,
					TimeoutSeconds: prepared.request.TimeoutSeconds,
				}
				var result map[string]any
				var runErr error
				if s.checkerForModel(current.model) == "claude" {
					result, runErr = runClaudeCheck(ctx, adminBundleSender{client: client}, s.claudeProfiles, input)
				} else {
					result, runErr = runSolCheck(ctx, adminBundleSender{client: client}, s.solProfile, input)
				}
				if runErr != nil {
					result = map[string]any{
						"account_id": current.account.ID, "account_name": current.account.Name,
						"claimed_model": current.model, "verdict": "ERROR", "error": runErr.Error(),
					}
				}
				outcomes <- outcome{index: index, result: result}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range combinations {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workerGroup.Wait()
		close(outcomes)
	}()
	results := make([]map[string]any, len(combinations))
	completed := 0
	for outcome := range outcomes {
		results[outcome.index] = outcome.result
		completed++
		task.Progress = 5 + completed*90/len(combinations)
		task.Message = fmt.Sprintf("已完成 %d/%d 个账号模型组合", completed, len(combinations))
		task.Result = map[string]any{"completed": completed, "total": len(combinations), "credentials_persisted": false}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	if completed != len(combinations) {
		s.finishFailed(task, errors.New("账号模型检测被中断"))
		return
	}
	summary := map[string]int{}
	for _, result := range results {
		verdict, _ := result["verdict"].(string)
		summary[verdict]++
	}
	task.Status, task.Progress = "succeeded", 100
	task.Message = fmt.Sprintf("账号模型检测完成：%d 个账号，%d 个模型", len(prepared.accounts), len(prepared.request.Models))
	task.Result = map[string]any{
		"accounts": len(prepared.accounts), "models": len(prepared.request.Models),
		"combinations": len(combinations), "summary": summary, "tests": results,
		"remote_write": false, "credentials_persisted": false,
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) finishFailed(task taskstore.Task, err error) {
	task.Status, task.Progress = "failed", 100
	task.Message = "账号模型检测失败：" + err.Error()
	task.Result = map[string]any{"error": err.Error(), "credentials_persisted": false}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}

type adminBundleSender struct {
	client *adminclient.Client
}

func (sender adminBundleSender) Send(ctx context.Context, accountID, model, prompt string, timeoutSeconds int) (string, string, error) {
	requestContext, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	response, err := sender.client.OpenAccountTest(requestContext, accountID, map[string]any{
		"model_id": model, "prompt": prompt, "mode": "behavior",
	})
	if err != nil {
		return "", "", safeTransportError(err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return "", "", visibleRequestError{message: fmt.Sprintf("账号测试通道返回 HTTP %d", response.StatusCode)}
	}
	var text strings.Builder
	actualModel := ""
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 4<<20))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "" || line == "[DONE]" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		eventType, _ := event["type"].(string)
		if eventType == "test_start" {
			actualModel, _ = event["model"].(string)
		}
		if eventType == "content" {
			if value, ok := event["text"].(string); ok {
				text.WriteString(value)
			}
		}
		if eventType == "error" {
			message, _ := event["error"].(string)
			return "", actualModel, visibleRequestError{message: safeAccountTestError(message)}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", actualModel, safeTransportError(err)
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", actualModel, visibleRequestError{message: "账号测试通道未返回有效文本"}
	}
	return text.String(), strings.TrimSpace(actualModel), nil
}

func safeAccountTestError(value string) string {
	value = strings.TrimSpace(redact.Secrets(value))
	if value == "" {
		return "账号测试通道返回错误"
	}
	runes := []rune(value)
	if len(runes) > 300 {
		value = string(runes[:300]) + "..."
	}
	return value
}

func normalizedUnique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func stablePositiveID(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatUint(parsed, 10) == value
}

func randomTaskID() (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "model-check-" + hex.EncodeToString(buffer), nil
}
