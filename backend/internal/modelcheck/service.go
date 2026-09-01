package modelcheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
	ListBySkill(context.Context, string) ([]taskstore.Task, error)
}

type CredentialStore interface {
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
	UpstreamKeySecret(context.Context, string, string, string) (*configstore.UpstreamKeySecret, error)
	SaveUpstreamKeySecret(context.Context, configstore.UpstreamKeySecret) error
}

type AccountCatalog interface {
	Accounts(context.Context) ([]business.AccountStatus, error)
	Account(context.Context, string) (*business.AccountDetail, error)
}

type KeyRevealer interface {
	RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error)
}

type UpstreamAuthResolver interface {
	ResolveAuth(context.Context, string, string) (*configstore.AuthRecord, error)
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

type AccountCheckStatus struct {
	AccountID string `json:"account_id"`
	Status    string `json:"status"`
	CheckedAt string `json:"checked_at"`
	TaskID    string `json:"task_id"`
}

type selectedAccount struct {
	ID              string
	Name            string
	BaseURL         string
	Platform        string
	AuthHost        string
	UpstreamKeyID   string
	UpstreamGroupID string
	CredentialError string
}

type preparedRun struct {
	request  Request
	accounts []selectedAccount
}

type Service struct {
	tasks          TaskStore
	credentials    CredentialStore
	accounts       AccountCatalog
	keys           KeyRevealer
	resolver       UpstreamAuthResolver
	claudeProfiles map[string]claudeProfile
	solProfile     solProfile
	taskTimeout    time.Duration
}

func New(tasks TaskStore, credentials CredentialStore, accounts AccountCatalog, keys KeyRevealer) (*Service, error) {
	if tasks == nil || credentials == nil || accounts == nil || keys == nil {
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
		tasks: tasks, credentials: credentials, accounts: accounts, keys: keys,
		claudeProfiles: claudeProfiles, solProfile: sol, taskTimeout: 30 * time.Minute,
	}, nil
}

func (s *Service) UseUpstreamAuthResolver(resolver UpstreamAuthResolver) {
	s.resolver = resolver
}

func (s *Service) Capabilities() Capabilities {
	standards := make([]string, 0, len(s.claudeProfiles))
	for standard := range s.claudeProfiles {
		standards = append(standards, standard)
	}
	sort.Strings(standards)
	return Capabilities{ClaudeStandards: standards, SolModels: append([]string(nil), s.solProfile.Models...)}
}

func (s *Service) AccountStatuses(ctx context.Context) ([]AccountCheckStatus, error) {
	tasks, err := s.tasks.ListBySkill(ctx, "sub2api-model-check")
	if err != nil {
		return nil, fmt.Errorf("模型检测历史读取失败: %w", err)
	}
	sort.SliceStable(tasks, func(left, right int) bool {
		return tasks[left].UpdatedAt > tasks[right].UpdatedAt
	})
	statuses := make([]AccountCheckStatus, 0)
	seen := map[string]bool{}
	for _, task := range tasks {
		if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_input" {
			continue
		}
		for _, accountID := range taskAccountIDs(task) {
			if seen[accountID] {
				continue
			}
			seen[accountID] = true
			statuses = append(statuses, AccountCheckStatus{
				AccountID: accountID,
				Status:    accountTaskStatus(task, accountID),
				CheckedAt: task.UpdatedAt,
				TaskID:    task.ID,
			})
		}
	}
	return statuses, nil
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
		Result: map[string]any{
			"account_ids": request.AccountIDs, "phase": "queued", "completed": 0,
			"total": len(request.AccountIDs) * len(request.Models), "tests": []map[string]any{},
			"credentials_persisted": false,
		}, CreatedAt: now, UpdatedAt: now,
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
		if row.ManualPriority != nil {
			return preparedRun{}, fmt.Errorf("账号 %s 处于人工优先位，模型检测已禁用", accountID)
		}
		detail, detailErr := s.accounts.Account(ctx, accountID)
		if detailErr != nil {
			return preparedRun{}, fmt.Errorf("账号 %s 详情读取失败", accountID)
		}
		selected = append(selected, directAccountSelection(row, detail))
	}
	return preparedRun{request: request, accounts: selected}, nil
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
	task.Status, task.Progress, task.Message = "running", 3, "正在准备账号凭据"
	task.Result = map[string]any{
		"account_ids": prepared.request.AccountIDs, "phase": "credentials", "completed": 0,
		"total": len(prepared.accounts) * len(prepared.request.Models), "tests": []map[string]any{},
		"credentials_persisted": false,
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistProgress(s.tasks, task)
	credentials := s.resolveCredentials(ctx, prepared.accounts)
	credentialsPersisted := credentialsResolved(credentials)
	client := &http.Client{
		Timeout:       time.Duration(prepared.request.TimeoutSeconds) * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	task.Progress, task.Message = 5, "正在并行检测账号与模型组合"
	task.Result = map[string]any{
		"account_ids": prepared.request.AccountIDs, "phase": "testing", "completed": 0,
		"total": len(prepared.accounts) * len(prepared.request.Models), "tests": []map[string]any{},
		"credentials_persisted": credentialsPersisted,
	}
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
				credential := credentials[current.account.ID]
				input := targetRequest{
					AccountID: current.account.ID, AccountName: current.account.Name,
					Model: current.model, Rounds: prepared.request.Rounds,
					TimeoutSeconds: prepared.request.TimeoutSeconds,
				}
				var result map[string]any
				var runErr error
				if credential.err != nil {
					runErr = credential.err
				} else if s.checkerForModel(current.model) == "claude" {
					result, runErr = runClaudeCheck(ctx, directBundleSender{client: client, credential: credential.value}, s.claudeProfiles, input)
				} else {
					result, runErr = runSolCheck(ctx, directBundleSender{client: client, credential: credential.value}, s.solProfile, input)
				}
				if runErr != nil {
					result = map[string]any{
						"account_id": current.account.ID, "account_name": current.account.Name,
						"claimed_model": current.model, "verdict": "ERROR", "error": runErr.Error(),
					}
				}
				result["credentials_persisted"] = credential.err == nil
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
	completedResults := make([]map[string]any, 0, len(combinations))
	completed := 0
	for outcome := range outcomes {
		results[outcome.index] = outcome.result
		completedResults = append(completedResults, outcome.result)
		completed++
		task.Progress = 5 + completed*90/len(combinations)
		task.Message = fmt.Sprintf("已完成 %d/%d 个账号模型组合", completed, len(combinations))
		task.Result = map[string]any{
			"account_ids": prepared.request.AccountIDs, "completed": completed, "total": len(combinations),
			"phase": "testing", "tests": completedResults,
			"credentials_persisted": credentialsPersisted,
		}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	if completed != len(combinations) {
		s.finishFailed(task, prepared.request.AccountIDs, errors.New("账号模型检测被中断"))
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
		"account_ids": prepared.request.AccountIDs,
		"accounts":    len(prepared.accounts), "models": len(prepared.request.Models),
		"combinations": len(combinations), "summary": summary, "tests": results,
		"phase": "completed", "completed": len(combinations), "total": len(combinations),
		"remote_write": false, "credentials_persisted": credentialsPersisted,
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) finishFailed(task taskstore.Task, accountIDs []string, err error) {
	task.Status, task.Progress = "failed", 100
	task.Message = "账号模型检测失败：" + err.Error()
	task.Result = map[string]any{
		"account_ids": accountIDs, "error": err.Error(), "credentials_persisted": false,
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}

type credentialResolution struct {
	value directCredential
	err   error
}

func (s *Service) resolveCredentials(ctx context.Context, accounts []selectedAccount) map[string]credentialResolution {
	results := make(map[string]credentialResolution, len(accounts))
	var resultsMu sync.Mutex
	jobs := make(chan selectedAccount)
	var workers sync.WaitGroup
	for range min(4, len(accounts)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for account := range jobs {
				value, err := s.resolveCredential(ctx, account)
				resultsMu.Lock()
				results[account.ID] = credentialResolution{value: value, err: err}
				resultsMu.Unlock()
			}
		}()
	}
	for _, account := range accounts {
		jobs <- account
	}
	close(jobs)
	workers.Wait()
	return results
}

func (s *Service) resolveCredential(ctx context.Context, account selectedAccount) (directCredential, error) {
	if account.CredentialError != "" {
		return directCredential{}, visibleRequestError{message: account.CredentialError}
	}
	cached, err := s.credentials.UpstreamKeySecret(ctx, account.AuthHost, account.UpstreamKeyID, account.UpstreamGroupID)
	if err != nil {
		return directCredential{}, visibleRequestError{message: "本地 Key 读取失败：" + safeCredentialError(err)}
	}
	if cached != nil {
		fallbackBaseURL := ""
		if strings.TrimSpace(account.BaseURL) == "" {
			record, readErr := s.credentials.AuthRecord(ctx, account.AuthHost)
			if readErr != nil {
				return directCredential{}, visibleRequestError{message: "上游授权读取失败：" + safeCredentialError(readErr)}
			}
			if record != nil {
				fallbackBaseURL = record.BaseURL
			}
		}
		return directCredentialFromSecret(account, cached.Secret, fallbackBaseURL)
	}
	record, err := s.credentials.AuthRecord(ctx, account.AuthHost)
	if err != nil {
		return directCredential{}, visibleRequestError{message: "上游授权读取失败：" + safeCredentialError(err)}
	}
	if record == nil && s.resolver != nil {
		record, err = s.resolver.ResolveAuth(ctx, account.AuthHost, "model-check")
		if err != nil {
			return directCredential{}, visibleRequestError{message: "上游授权恢复失败：" + safeCredentialError(err)}
		}
	}
	if record == nil {
		return directCredential{}, visibleRequestError{message: "上游未配置可用授权，无法读取绑定 Key"}
	}
	key, err := s.keys.RevealKey(ctx, *record, account.UpstreamKeyID, account.UpstreamGroupID)
	if err != nil {
		return directCredential{}, visibleRequestError{message: "绑定 Key 读取失败：" + safeCredentialError(err)}
	}
	if strings.TrimSpace(key.Secret) == "" {
		return directCredential{}, visibleRequestError{message: "绑定 Key 未返回可用密钥"}
	}
	secret := strings.TrimSpace(key.Secret)
	if err := s.credentials.SaveUpstreamKeySecret(ctx, configstore.UpstreamKeySecret{
		Host: account.AuthHost, KeyID: account.UpstreamKeyID, GroupID: account.UpstreamGroupID, Secret: secret,
	}); err != nil {
		return directCredential{}, visibleRequestError{message: "本地 Key 保存失败：" + safeCredentialError(err)}
	}
	return directCredentialFromSecret(account, secret, record.BaseURL)
}

func directCredentialFromSecret(account selectedAccount, secret, fallbackBaseURL string) (directCredential, error) {
	baseURL := strings.TrimSpace(account.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(fallbackBaseURL)
	}
	baseURL, err := configstore.ValidateBaseURL(baseURL)
	if err != nil {
		return directCredential{}, visibleRequestError{message: "账号 Base URL 无效"}
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return directCredential{}, visibleRequestError{message: "本地 Key 未包含可用密钥"}
	}
	return directCredential{
		BaseURL: baseURL, FallbackBaseURL: directFallbackBaseURL(baseURL, account.AuthHost),
		Secret: secret, Platform: account.Platform,
	}, nil
}

func credentialsResolved(values map[string]credentialResolution) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value.err != nil {
			return false
		}
	}
	return true
}

func directAccountSelection(row business.AccountStatus, detail *business.AccountDetail) selectedAccount {
	selected := selectedAccount{ID: row.ID, Name: row.Name}
	if detail == nil {
		selected.CredentialError = "账号详情不存在"
		return selected
	}
	selected.BaseURL = firstPointerText(detail.BaseURL, detail.UpstreamBaseURL)
	selected.Platform = firstPointerText(detail.Platform)
	type bindingKey struct{ authHost, keyID, groupID string }
	unique := map[bindingKey]struct{}{}
	for _, binding := range detail.Bindings {
		if binding.Status != nil && strings.EqualFold(strings.TrimSpace(*binding.Status), "missing") {
			continue
		}
		authHost := strings.TrimSpace(binding.UpstreamHost)
		if binding.SourceAuthHost != nil && strings.TrimSpace(*binding.SourceAuthHost) != "" {
			authHost = strings.TrimSpace(*binding.SourceAuthHost)
		}
		groupID := firstPointerText(binding.UpstreamGroupID)
		key := bindingKey{authHost: authHost, keyID: strings.TrimSpace(binding.UpstreamKeyID), groupID: groupID}
		if key.authHost != "" && key.keyID != "" && key.groupID != "" {
			unique[key] = struct{}{}
		}
	}
	if len(unique) != 1 {
		if len(unique) == 0 {
			selected.CredentialError = "账号没有可用于直连检测的有效 Key 绑定"
		} else {
			selected.CredentialError = "账号存在多个不同 Key 绑定，无法确定直连凭据"
		}
		return selected
	}
	for key := range unique {
		selected.AuthHost, selected.UpstreamKeyID, selected.UpstreamGroupID = key.authHost, key.keyID, key.groupID
	}
	return selected
}

func firstPointerText(values ...*string) string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return strings.TrimSpace(*value)
		}
	}
	return ""
}

func taskAccountIDs(task taskstore.Task) []string {
	values := stringValues(task.Result["account_ids"])
	for _, result := range taskResultRows(task.Result["tests"]) {
		if accountID, ok := result["account_id"].(string); ok {
			values = append(values, accountID)
		}
	}
	return normalizedUnique(values)
}

func accountTaskStatus(task taskstore.Task, accountID string) string {
	if task.Status != "succeeded" {
		return "inconclusive"
	}
	matched, consistent, inconsistent := 0, 0, 0
	for _, result := range taskResultRows(task.Result["tests"]) {
		if resultAccountID, _ := result["account_id"].(string); resultAccountID != accountID {
			continue
		}
		matched++
		if modelCheckRequestFailed(result) {
			continue
		}
		switch verdict, _ := result["verdict"].(string); verdict {
		case "MATCH", "GROUP_MATCH", "SOL_CONSISTENT":
			consistent++
		case "MISMATCH", "LUNA_LIKE", "TERRA_LIKE":
			inconsistent++
		}
	}
	if inconsistent > 0 {
		return "inconsistent"
	}
	if matched > 0 && consistent == matched {
		return "consistent"
	}
	return "inconclusive"
}

func modelCheckRequestFailed(result map[string]any) bool {
	if verdict, _ := result["verdict"].(string); verdict == "ERROR" {
		return true
	}
	requests, _ := result["requests"].(map[string]any)
	if requests == nil {
		return false
	}
	successful, successfulOK := numericValue(requests["successful"])
	total, totalOK := numericValue(requests["total"])
	return successfulOK && totalOK && successful == 0 && total > 0
}

func taskResultRows(value any) []map[string]any {
	if rows, ok := value.([]map[string]any); ok {
		return rows
	}
	items, _ := value.([]any)
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func stringValues(value any) []string {
	if values, ok := value.([]string); ok {
		return append([]string(nil), values...)
	}
	items, _ := value.([]any)
	values := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			values = append(values, value)
		}
	}
	return values
}

func numericValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
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
