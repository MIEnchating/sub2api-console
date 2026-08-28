package probe

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

type Repository interface {
	ControlPolicy(context.Context) (map[string]any, error)
	ProbeCandidates(context.Context, *string, *string) ([]business.ProbeCandidate, error)
	PersistProbeSamples(context.Context, []business.ProbeSample) (int, error)
}

type SettingsStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Request struct {
	AccountID *string
	GroupName *string
	// AccountIDs is used by the inspection fallback to execute one bounded
	// probe batch. The public API accepts only AccountID; callers cannot submit
	// this internal selection directly.
	AccountIDs []string
}

type RunSummary struct {
	Targets   int      `json:"targets"`
	Persisted int      `json:"persisted"`
	Passed    int      `json:"passed"`
	Failed    int      `json:"failed"`
	Skipped   int      `json:"skipped"`
	Results   []Result `json:"results"`
}

type Config struct {
	Timeout        time.Duration
	MaxConcurrency int
	Prompt         string
}

type Target struct {
	AccountID  string
	GroupName  string
	GroupID    *string
	Model      *string
	SkipReason *string
}

type Result struct {
	AccountID      string  `json:"account_id"`
	GroupName      string  `json:"group_name"`
	Result         string  `json:"result"`
	LatencyP50     *string `json:"latency_p50_ms"`
	LatencyP95     *string `json:"latency_p95_ms"`
	LatencyP99     *string `json:"latency_p99_ms"`
	Attempts       int     `json:"attempts"`
	FailureReason  *string `json:"failure_reason"`
	StatusCode     *int    `json:"status_code"`
	ObservedAt     string  `json:"observed_at"`
	RequestModel   string  `json:"request_model"`
	ActualModel    string  `json:"actual_model"`
	ModelRewritten bool    `json:"model_rewritten"`
}

type preparedRun struct {
	config  Config
	target  configstore.TargetSettings
	policy  map[string]any
	targets []Target
}

type Service struct {
	repository Repository
	settings   SettingsStore
	tasks      TaskStore
	timeout    time.Duration
}

func New(repository Repository, settings SettingsStore, tasks TaskStore) *Service {
	return &Service{repository: repository, settings: settings, tasks: tasks, timeout: 10 * time.Minute}
}

func (s *Service) Enqueue(ctx context.Context, request Request, _ string) (taskstore.Task, error) {
	prepared, err := s.prepare(ctx, request)
	if err != nil {
		return taskstore.Task{}, err
	}
	id, err := randomID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-connectivity-test", Operation: "active-probe", Status: "queued", Progress: 0,
		Message: "主动探测已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go s.execute(task, prepared)
	return task, nil
}

func (s *Service) prepare(ctx context.Context, request Request) (preparedRun, error) {
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return preparedRun{}, err
	}
	probePolicy, err := optionalObject(policy, "probe")
	if err != nil {
		return preparedRun{}, errors.New("主动探测策略配置无效")
	}
	recoverySelection := len(request.AccountIDs) > 0
	if raw, present := probePolicy["enabled"]; present {
		enabled, ok := raw.(bool)
		if !ok {
			return preparedRun{}, errors.New("主动探测开关配置无效")
		}
		if !enabled && !recoverySelection {
			return preparedRun{}, errors.New("主动探测已关闭，请在调度策略中开启")
		}
	}
	targetSettings, err := s.settings.TargetSettings(ctx)
	if err != nil {
		return preparedRun{}, err
	}
	config, err := configFromPolicy(policy)
	if err != nil {
		return preparedRun{}, err
	}
	candidates, err := s.repository.ProbeCandidates(ctx, request.AccountID, request.GroupName)
	if err != nil {
		return preparedRun{}, err
	}
	targets, err := buildTargets(candidates, policy, recoverySelection)
	if err != nil {
		return preparedRun{}, err
	}
	if len(request.AccountIDs) > 0 {
		selected := make(map[string]struct{}, len(request.AccountIDs))
		for _, accountID := range request.AccountIDs {
			accountID = strings.TrimSpace(accountID)
			if !stablePositiveID(accountID) {
				return preparedRun{}, errors.New("巡检探测账号必须使用稳定数字 ID")
			}
			selected[accountID] = struct{}{}
		}
		filtered := make([]Target, 0, len(targets))
		for _, target := range targets {
			if _, found := selected[target.AccountID]; found {
				filtered = append(filtered, target)
			}
		}
		targets = filtered
	}
	executable := false
	var firstReason string
	for _, target := range targets {
		if target.Model != nil {
			executable = true
		}
		if firstReason == "" && target.SkipReason != nil {
			firstReason = *target.SkipReason
		}
	}
	if !executable {
		if firstReason == "" {
			firstReason = "没有符合当前分组、参与范围和探测策略的账号"
		}
		return preparedRun{}, errors.New(firstReason)
	}
	return preparedRun{config: config, target: targetSettings, policy: policy, targets: targets}, nil
}

func (s *Service) RunNow(ctx context.Context, request Request) (RunSummary, error) {
	prepared, err := s.prepare(ctx, request)
	if err != nil {
		return RunSummary{}, err
	}
	return s.runPrepared(ctx, prepared)
}

func (s *Service) execute(task taskstore.Task, prepared preparedRun) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 15, "正在通过官方账号测试接口执行主动探测", time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	summary, err := s.runPrepared(ctx, prepared)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		task.Status, task.Message = "failed", "主动探测失败："+err.Error()
		task.Result = map[string]any{"remote_write": false, "credentials_persisted": false, "error": err.Error()}
	} else {
		task.Status = "succeeded"
		task.Message = fmt.Sprintf("官方探测完成：通过 %d，失败 %d，跳过 %d", summary.Passed, summary.Failed, summary.Skipped)
		task.Result = map[string]any{
			"source": "official-account-test", "targets": summary.Targets, "persisted": summary.Persisted,
			"passed": summary.Passed, "failed": summary.Failed, "skipped": summary.Skipped, "policy": prepared.policy,
			"results":      summary.Results,
			"remote_write": false, "credentials_persisted": false,
		}
	}
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) runPrepared(ctx context.Context, prepared preparedRun) (RunSummary, error) {
	results, err := run(ctx, prepared)
	if err != nil {
		return RunSummary{}, err
	}
	samples := make([]business.ProbeSample, 0, len(results))
	passed, skipped := 0, 0
	for _, result := range results {
		if result.Result == "跳过" {
			skipped++
			continue
		}
		samples = append(samples, business.ProbeSample{
			AccountID: result.AccountID, GroupName: result.GroupName, Result: result.Result,
			LatencyP50: result.LatencyP50, LatencyP95: result.LatencyP95, LatencyP99: result.LatencyP99,
			SampleCount: boolCount(result.Result == "通过"), Attempts: result.Attempts,
			FailureReason: result.FailureReason, ObservedAt: result.ObservedAt, StatusCode: result.StatusCode,
			RequestModel: result.RequestModel, ActualModel: result.ActualModel,
		})
		if result.Result == "通过" {
			passed++
		}
	}
	persisted, err := s.repository.PersistProbeSamples(ctx, samples)
	if err != nil {
		return RunSummary{}, err
	}
	return RunSummary{Targets: len(prepared.targets), Persisted: persisted, Passed: passed, Failed: len(results) - passed - skipped, Skipped: skipped, Results: results}, nil
}

func run(ctx context.Context, prepared preparedRun) ([]Result, error) {
	client, err := adminclient.New(adminclient.Config{
		BaseURL: prepared.target.BaseURL, AdminKey: prepared.target.AdminKey,
		Timeout: prepared.config.Timeout, Attempts: 1,
	}, nil)
	if err != nil {
		return nil, err
	}
	type indexedResult struct {
		index  int
		result Result
	}
	jobs := make(chan int)
	outcomes := make(chan indexedResult, len(prepared.targets))
	workers := prepared.config.MaxConcurrency
	if workers > len(prepared.targets) {
		workers = len(prepared.targets)
	}
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				outcomes <- indexedResult{index: index, result: probeTarget(ctx, client, prepared.targets[index], prepared.config)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range prepared.targets {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		group.Wait()
		close(outcomes)
	}()
	results := make([]Result, len(prepared.targets))
	count := 0
	for outcome := range outcomes {
		results[outcome.index] = outcome.result
		count++
	}
	if count != len(prepared.targets) {
		return nil, ctx.Err()
	}
	sort.Slice(results, func(left, right int) bool {
		if results[left].AccountID == results[right].AccountID {
			return results[left].GroupName < results[right].GroupName
		}
		return stableNumericLess(results[left].AccountID, results[right].AccountID)
	})
	return results, nil
}

func probeTarget(ctx context.Context, client *adminclient.Client, target Target, config Config) Result {
	observed := time.Now().UTC().Format(time.RFC3339Nano)
	if target.SkipReason != nil {
		return Result{AccountID: target.AccountID, GroupName: target.GroupName, Result: "跳过", FailureReason: target.SkipReason, ObservedAt: observed}
	}
	started := time.Now()
	lastStatus, firstResponse, lastReason, actualModel := probeAttempt(ctx, client, target, config)
	requestModel := ""
	if target.Model != nil {
		requestModel = *target.Model
	}
	rewritten := requestModel != "" && actualModel != "" && requestModel != actualModel
	if firstResponse {
		latency := decimalMilliseconds(float64(time.Since(started)) / float64(time.Millisecond))
		return Result{AccountID: target.AccountID, GroupName: target.GroupName, Result: "通过", LatencyP50: &latency, LatencyP95: &latency, LatencyP99: &latency, Attempts: 1, StatusCode: lastStatus, ObservedAt: observed, RequestModel: requestModel, ActualModel: actualModel, ModelRewritten: rewritten}
	}
	result := "失败"
	if lastReason == "主动探测超时" {
		result = "超时"
	} else if lastReason == "管理 API 异常" {
		result = "管理 API 异常"
	}
	if lastReason == "" {
		lastReason = "主动探测请求失败"
	}
	return Result{AccountID: target.AccountID, GroupName: target.GroupName, Result: result, Attempts: 1, FailureReason: &lastReason, StatusCode: lastStatus, ObservedAt: observed, RequestModel: requestModel, ActualModel: actualModel, ModelRewritten: rewritten}
}

func probeAttempt(ctx context.Context, client *adminclient.Client, target Target, config Config) (*int, bool, string, string) {
	if target.Model == nil {
		return nil, false, "缺少探测模型", ""
	}
	response, err := client.OpenAccountTest(ctx, target.AccountID, map[string]any{"model_id": *target.Model, "prompt": config.Prompt, "mode": "default"})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, false, "主动探测超时", ""
		}
		return nil, false, "管理 API 异常", ""
	}
	defer response.Body.Close()
	status := response.StatusCode
	if status >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 500))
		return &status, false, failure(status, string(body)), ""
	}
	actualModel := ""
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 4<<20))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		data := line
		if strings.HasPrefix(data, "data:") {
			data = strings.TrimSpace(strings.TrimPrefix(data, "data:"))
		}
		if data == "[DONE]" {
			break
		}
		decoder := json.NewDecoder(strings.NewReader(data))
		decoder.UseNumber()
		var event any
		if err := decoder.Decode(&event); err != nil {
			continue
		}
		if actualModel == "" {
			actualModel = eventModel(event)
		}
		if reason := eventError(event); reason != "" {
			return &status, false, reason, actualModel
		}
		if eventHasContent(event) {
			return &status, true, "", actualModel
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &status, false, "主动探测超时", actualModel
		}
		return &status, false, "管理 API 异常", actualModel
	}
	return &status, false, "官方探测流未返回有效文本", actualModel
}

func eventModel(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	model, _ := object["model"].(string)
	return strings.TrimSpace(model)
}

func eventHasContent(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	eventType := strings.ToLower(strings.TrimSpace(fmt.Sprint(object["type"])))
	if eventType == "content" || eventType == "image" {
		return eventHasText(object)
	}
	for _, key := range []string{"choices", "output"} {
		if child, present := object[key]; present && eventHasText(child) {
			return true
		}
	}
	return false
}

func eventHasText(value any) bool {
	switch item := value.(type) {
	case string:
		return strings.TrimSpace(item) != ""
	case []any:
		for _, child := range item {
			if eventHasText(child) {
				return true
			}
		}
	case map[string]any:
		for _, key := range []string{"delta", "text", "output_text", "content", "token"} {
			if child, present := item[key]; present && eventHasText(child) {
				return true
			}
		}
		for _, key := range []string{"choices", "output"} {
			if child, present := item[key]; present && eventHasText(child) {
				return true
			}
		}
	}
	return false
}

func eventError(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if raw, present := object["error"]; present && raw != nil && raw != "" {
		if errorObject, ok := raw.(map[string]any); ok {
			if message, present := errorObject["message"]; present {
				return limitedText(message)
			}
			if code, present := errorObject["code"]; present {
				return limitedText(code)
			}
			return "上游返回错误"
		}
		return limitedText(raw)
	}
	typeText := strings.ToLower(strings.TrimSpace(fmt.Sprint(object["type"])))
	if strings.Contains(typeText, "error") || strings.Contains(typeText, "failed") {
		if message, present := object["message"]; present {
			return limitedText(message)
		}
		if detail, present := object["detail"]; present {
			return limitedText(detail)
		}
		return typeText
	}
	return ""
}

func failure(status int, body string) string {
	text := strings.ToLower(body)
	if status == http.StatusUnauthorized || status == http.StatusForbidden || containsAny(text, "unauthorized", "invalid api key", "鉴权", "认证") {
		return "鉴权失败"
	}
	if status == http.StatusTooManyRequests || containsAny(text, "rate limit", "too many", "quota", "限流", "额度") {
		return "上游限流或额度不足"
	}
	if status >= 500 {
		return "上游网关错误"
	}
	return "主动探测请求失败"
}

func decimalMilliseconds(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 3, 64), "0"), ".")
}

func configFromPolicy(policy map[string]any) (Config, error) {
	probeObject, err := optionalObject(policy, "probe")
	if err != nil {
		return Config{}, errors.New("探测配置无效：probe")
	}
	timeout, err := currentPolicyInteger(probeObject, "timeout_seconds", 60, 1, 86400)
	if err != nil {
		return Config{}, errors.New("探测配置无效：probe.timeout_seconds")
	}
	concurrency, err := currentPolicyInteger(probeObject, "concurrency", 4, 1, 32)
	if err != nil {
		return Config{}, errors.New("探测配置无效：probe.concurrency")
	}
	prompt := "hi"
	if raw, present := probeObject["prompt"]; present {
		value, ok := raw.(string)
		if !ok {
			return Config{}, errors.New("探测配置无效：probe.prompt")
		}
		if value = strings.TrimSpace(value); value != "" {
			prompt = value
		}
	}
	return Config{Timeout: time.Duration(timeout) * time.Second, MaxConcurrency: concurrency, Prompt: prompt}, nil
}

func buildTargets(candidates []business.ProbeCandidate, policy map[string]any, allowDisabledProbe bool) ([]Target, error) {
	byAccount := map[string][]business.ProbeCandidate{}
	for _, candidate := range candidates {
		if excludedByMetadata(candidate.Metadata) {
			continue
		}
		if allowed, err := eligibleScope(candidate, policy); err != nil {
			return nil, err
		} else if !allowed {
			continue
		}
		byAccount[candidate.AccountID] = append(byAccount[candidate.AccountID], candidate)
	}
	accountIDs := make([]string, 0, len(byAccount))
	for accountID := range byAccount {
		accountIDs = append(accountIDs, accountID)
	}
	sort.SliceStable(accountIDs, func(left, right int) bool { return stableNumericLess(accountIDs[left], accountIDs[right]) })
	result := make([]Target, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		memberships := byAccount[accountID]
		primary := memberships[0]
		for _, candidate := range memberships[1:] {
			if probeMembershipLess(candidate, primary) {
				primary = candidate
			}
		}
		target := Target{AccountID: primary.AccountID, GroupName: primary.GroupName, GroupID: primary.GroupID}
		if primary.MetadataErr != nil {
			reason := "账号 metadata 配置无效"
			target.SkipReason = &reason
		} else {
			target.Model, target.SkipReason = resolveModel(policy, primary.AccountID, primary.GroupID, primary.KnownModels, allowDisabledProbe)
		}
		result = append(result, target)
	}
	return result, nil
}

func probeMembershipLess(left, right business.ProbeCandidate) bool {
	leftKey, rightKey := left.GroupName, right.GroupName
	if left.GroupID != nil && strings.TrimSpace(*left.GroupID) != "" {
		leftKey = strings.TrimSpace(*left.GroupID)
	}
	if right.GroupID != nil && strings.TrimSpace(*right.GroupID) != "" {
		rightKey = strings.TrimSpace(*right.GroupID)
	}
	return stableNumericLess(leftKey, rightKey)
}

func eligibleScope(candidate business.ProbeCandidate, policy map[string]any) (bool, error) {
	scope, err := optionalObject(policy, "scope")
	if err != nil {
		return false, errors.New("范围配置无效：scope")
	}
	excludedAccounts, err := scopeItems(scope, "excluded_account_ids")
	if err != nil {
		return false, errors.New("范围配置无效：scope.excluded_account_ids")
	}
	if containsFold(excludedAccounts, candidate.AccountID) {
		return false, nil
	}
	excludedIDs, errIDs := scopeItems(scope, "excluded_group_ids")
	if errIDs != nil {
		return false, errors.New("范围配置无效：scope.excluded_group_ids")
	}
	if candidate.GroupID != nil && containsFold(excludedIDs, *candidate.GroupID) {
		return false, nil
	}
	if raw, present := scope["managed_group_mode"]; present {
		mode, ok := raw.(string)
		if !ok || (mode != "all" && mode != "selected") {
			return false, errors.New("范围配置无效：scope.managed_group_mode")
		}
		if mode == "selected" {
			managed, err := scopeItems(scope, "managed_group_ids")
			if err != nil {
				return false, errors.New("范围配置无效：scope.managed_group_ids")
			}
			if !containsFold(managed, candidate.GroupName) && (candidate.GroupID == nil || !containsFold(managed, *candidate.GroupID)) {
				return false, nil
			}
		}
	}
	if candidate.GroupID != nil {
		bindings, bindingsErr := optionalObject(policy, "group_policy_bindings")
		if bindingsErr != nil {
			return false, errors.New("分组探测绑定配置无效")
		}
		if rawBinding, present := bindings[strings.TrimSpace(*candidate.GroupID)]; present {
			binding, ok := rawBinding.(map[string]any)
			if !ok {
				return false, errors.New("分组探测绑定配置无效")
			}
			if rawEnabled, present := binding["enabled"]; present {
				enabled, ok := flexibleBoolean(rawEnabled)
				if !ok {
					return false, errors.New("分组探测开关无效")
				}
				if !enabled {
					return false, nil
				}
			}
		}
	}
	accountTypes, typeErr := scopeItems(scope, "account_types")
	platforms, platformErr := scopeItems(scope, "platforms")
	if typeErr != nil || platformErr != nil {
		return false, errors.New("范围配置无效：scope.account_types/platforms")
	}
	accountType := "apikey"
	if raw, present := candidate.Metadata["type"]; present {
		accountType = strings.TrimSpace(fmt.Sprint(raw))
	} else if raw, present := candidate.Metadata["account_type"]; present {
		accountType = strings.TrimSpace(fmt.Sprint(raw))
	}
	platform := ""
	if raw, present := candidate.Metadata["platform"]; present {
		platform = strings.TrimSpace(fmt.Sprint(raw))
	} else if candidate.UpstreamType != nil {
		platform = *candidate.UpstreamType
	}
	if len(accountTypes) > 0 && !containsFold(accountTypes, accountType) {
		return false, nil
	}
	if len(platforms) > 0 && !containsFold(platforms, platform) {
		return false, nil
	}
	return true, nil
}

func resolveModel(policy map[string]any, accountID string, groupID *string, knownModels []string, allowDisabledProbe ...bool) (*string, *string) {
	if rawModels, present := policy["account_test_models"]; present {
		models, ok := rawModels.(map[string]any)
		if !ok {
			return nil, textPointer("账号探测模型配置无效")
		}
		if rawModel, found := models[accountID]; found {
			model, ok := rawModel.(string)
			if !ok || strings.TrimSpace(model) == "" {
				return nil, textPointer("账号探测模型配置无效")
			}
			value := strings.TrimSpace(model)
			return &value, nil
		}
	}
	if groupID != nil && strings.TrimSpace(*groupID) != "" {
		bindings, err := optionalObject(policy, "group_policy_bindings")
		if err != nil {
			return nil, textPointer("分组探测绑定配置无效")
		}
		rawBinding, bindingPresent := bindings[strings.TrimSpace(*groupID)]
		binding := map[string]any{}
		if bindingPresent {
			var ok bool
			binding, ok = rawBinding.(map[string]any)
			if !ok {
				return nil, textPointer("分组探测绑定配置无效")
			}
		}
		if raw, present := binding["enabled"]; present {
			enabled, ok := flexibleBoolean(raw)
			if !ok {
				return nil, textPointer("分组探测开关无效")
			}
			if !enabled {
				return nil, textPointer("分组未参与守护")
			}
		}
		if raw, present := binding["probe_enabled"]; present {
			enabled, ok := raw.(bool)
			if !ok {
				return nil, textPointer("分组定时测试开关无效")
			}
			if !enabled && (len(allowDisabledProbe) == 0 || !allowDisabledProbe[0]) {
				return nil, textPointer("分组定时测试已关闭")
			}
		}
		if raw, present := binding["probe_model"]; present {
			if raw == nil {
				// A nil override inherits the global model.
			} else if model, ok := raw.(string); ok {
				if strings.TrimSpace(model) != "" {
					value := strings.TrimSpace(model)
					return &value, nil
				}
			} else {
				return nil, textPointer("分组测试模型配置无效")
			}
		}
	}
	probeObject, err := optionalObject(policy, "probe")
	if err != nil {
		return nil, textPointer("默认探测模型配置无效")
	}
	if raw, present := probeObject["model"]; present {
		model, ok := raw.(string)
		if !ok {
			return nil, textPointer("默认探测模型配置无效")
		}
		if value := strings.TrimSpace(model); value != "" {
			return &value, nil
		}
	}
	if model := firstKnownModel(knownModels); model != nil {
		return model, nil
	}
	return nil, textPointer("没有可用探测模型")
}

func firstKnownModel(models []string) *string {
	for _, model := range models {
		if value := strings.TrimSpace(model); value != "" {
			return &value
		}
	}
	return nil
}

func optionalObject(parent map[string]any, key string) (map[string]any, error) {
	raw, present := parent[key]
	if !present {
		return map[string]any{}, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New(key + " must be object")
	}
	return object, nil
}

func scopeItems(scope map[string]any, key string) ([]string, error) {
	raw, present := scope[key]
	if !present {
		return []string{}, nil
	}
	if text, ok := raw.(string); ok {
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("empty scope string")
		}
		parts := strings.Split(text, ",")
		result := make([]string, len(parts))
		for index, part := range parts {
			result[index] = strings.TrimSpace(part)
			if result[index] == "" {
				return nil, errors.New("empty scope item")
			}
		}
		return result, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("scope value must be list")
	}
	result := make([]string, len(values))
	for index, value := range values {
		switch value.(type) {
		case nil, map[string]any, []any:
			return nil, errors.New("scope item invalid")
		}
		result[index] = strings.TrimSpace(fmt.Sprint(value))
		if result[index] == "" {
			return nil, errors.New("scope item empty")
		}
	}
	return result, nil
}

func currentPolicyInteger(values map[string]any, key string, defaultValue, minimum, maximum int) (int, error) {
	raw, present := values[key]
	if !present {
		return defaultValue, nil
	}
	return strictPolicyInteger(raw, minimum, maximum)
}

func strictPolicyInteger(raw any, minimum, maximum int) (int, error) {
	if _, ok := raw.(bool); ok {
		return 0, errors.New("boolean is not integer")
	}
	text := strings.TrimSpace(fmt.Sprint(raw))
	parsed, err := strconv.Atoi(text)
	if err != nil || strconv.Itoa(parsed) != text || parsed < minimum || parsed > maximum {
		return 0, errors.New("integer out of range")
	}
	return parsed, nil
}

func flexibleBoolean(value any) (bool, bool) {
	if boolean, ok := value.(bool); ok {
		return boolean, true
	}
	normalized := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	switch normalized {
	case "0", "false", "off", "disabled", "no", "关闭", "禁用":
		return false, true
	case "1", "true", "on", "enabled", "yes", "开启", "启用":
		return true, true
	default:
		return false, false
	}
}

func excludedByMetadata(metadata map[string]any) bool {
	for _, key := range []string{"auto_monitor_excluded", "automatic_operation_excluded", "self_managed_pool"} {
		if value, ok := metadata[key].(bool); ok && value {
			return true
		}
	}
	return false
}

func containsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func limitedText(value any) string {
	text := redact.Secrets(fmt.Sprint(value))
	if value == nil {
		return "空值"
	}
	if utf8.RuneCountInString(text) <= 500 {
		return text
	}
	runes := []rune(text)
	return string(runes[:500])
}

func stableNumericLess(left, right string) bool {
	leftValue, leftErr := strconv.ParseUint(left, 10, 64)
	rightValue, rightErr := strconv.ParseUint(right, 10, 64)
	if leftErr == nil && rightErr == nil {
		return leftValue < rightValue
	}
	return left < right
}

func stablePositiveID(value string) bool {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return err == nil && parsed > 0 && strings.TrimSpace(value) == value
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func textPointer(value string) *string { return &value }

func randomID() (string, error) {
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
