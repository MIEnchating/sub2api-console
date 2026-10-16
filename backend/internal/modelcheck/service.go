package modelcheck

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelfingerprint"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountquality"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
	ListBySkill(context.Context, string, int) ([]taskstore.Task, error)
	AnimationHistory(context.Context) ([]taskstore.Task, error)
	ModelCheckHistory(context.Context, time.Time) ([]taskstore.Task, error)
	DetectionConfidenceHistory(context.Context, time.Time) ([]taskstore.DetectionConfidenceResult, error)
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

type animationEvidenceWriter interface {
	PersistAnimationEvidence(context.Context, []business.AnimationEvidence) error
}

type KeyRevealer interface {
	RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error)
}

type UpstreamAuthResolver interface {
	ResolveAuth(context.Context, string, string) (*configstore.AuthRecord, error)
}

type Request struct {
	PublicReference     bool
	BehaviorFingerprint bool
	Fingerprint         bool
	AccountIDs          []string
	Models              []string
	Rounds              int
	TimeoutSeconds      int
}

type Capabilities struct {
	PublicReferences []modelfingerprint.CatalogEntry `json:"public_references"`
	AstraModels      []string                        `json:"astra_models"`
	ClaudeStandards  []string                        `json:"claude_standards"`
	SolModels        []string                        `json:"sol_models"`
}

type AccountCheckStatus struct {
	AccountID  string                    `json:"account_id"`
	Status     string                    `json:"status"`
	CheckedAt  string                    `json:"checked_at"`
	TaskID     string                    `json:"task_id"`
	Confidence accountquality.Statistics `json:"confidence"`
}

type selectedAccount struct {
	ID              string
	Name            string
	BaseURL         string
	Platform        string
	AccountType     string
	AuthHost        string
	UpstreamKeyID   string
	UpstreamGroupID string
	CredentialError string
}

type preparedRun struct {
	request            Request
	accounts           []selectedAccount
	claudeProfiles     map[string]claudeProfile
	solProfile         solProfile
	profileVersion     string
	profileFingerprint string
	oauthTarget        *configstore.TargetSettings
}

type Service struct {
	qualityRecheckMu      sync.Mutex
	detectionTimeouts     detectionTimeoutState
	qualityModel          atomic.Pointer[animationquality.Detector]
	qualitySettings       qualitySettingsState
	animationPrompt       animationPromptState
	tasks                 TaskStore
	credentials           CredentialStore
	accounts              AccountCatalog
	keys                  KeyRevealer
	resolver              UpstreamAuthResolver
	profilesMu            sync.RWMutex
	claudeProfiles        map[string]claudeProfile
	solProfile            solProfile
	configuration         configurationState
	profileRepository     configurationRepository
	oauthTransport        http.RoundTripper
	oauthCredentialSource OAuthCredentialSource
	taskRunner            taskrunner.Runner
	animationRunner       taskrunner.Runner
	taskTimeout           time.Duration
	animation             animationState
	terminalMu            sync.Mutex
	terminalActive        map[string]bool
	detectionTasks        *detectionTaskState
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
	service := &Service{
		tasks: tasks, credentials: credentials, accounts: accounts, keys: keys,
		claudeProfiles: claudeProfiles, solProfile: sol, taskTimeout: 30 * time.Minute,
		terminalActive: map[string]bool{},
	}
	service.configuration = newBuiltinConfiguration(claudeProfiles, sol)
	if repository, ok := accounts.(configurationRepository); ok {
		service.profileRepository = repository
	}
	if err := service.loadDetectionTimeouts(context.Background()); err != nil {
		return nil, err
	}
	if err := service.loadQualitySettings(context.Background()); err != nil {
		return nil, err
	}
	if err := service.loadAnimationPrompt(context.Background()); err != nil {
		return nil, err
	}
	if err := service.loadAnimationConfiguration(context.Background()); err != nil {
		return nil, err
	}
	if err := service.loadDetectionTasks(context.Background()); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) UseUpstreamAuthResolver(resolver UpstreamAuthResolver) {
	s.resolver = resolver
}

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

// UseAnimationSchedulerRunner keeps the long-lived scheduler loop isolated
// from user-triggered model-check tasks.
func (s *Service) UseAnimationSchedulerRunner(runner taskrunner.Runner) { s.animationRunner = runner }

func (s *Service) Capabilities() Capabilities {
	s.profilesMu.RLock()
	defer s.profilesMu.RUnlock()
	standards := make([]string, 0, len(s.claudeProfiles))
	for standard := range s.claudeProfiles {
		standards = append(standards, standard)
	}
	sort.Strings(standards)
	return Capabilities{PublicReferences: modelfingerprint.Catalog(), ClaudeStandards: standards, SolModels: append([]string(nil), s.solProfile.Models...), AstraModels: []string{astraModel}}
}

func (s *Service) AccountStatuses(ctx context.Context) ([]AccountCheckStatus, error) {
	now := time.Now().UTC()
	tasks, err := s.tasks.ModelCheckHistory(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("模型检测历史读取失败: %w", err)
	}
	sort.SliceStable(tasks, func(left, right int) bool {
		return tasks[left].UpdatedAt > tasks[right].UpdatedAt
	})
	statuses := make([]AccountCheckStatus, 0)
	confidence := accountConfidence(tasks, now)
	detections, err := s.tasks.DetectionConfidenceHistory(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("智商检测历史读取失败: %w", err)
	}
	if err = s.overlayQualityConfidence(ctx, detections); err != nil {
		return nil, fmt.Errorf("动画质量记录读取失败: %w", err)
	}
	addDetectionConfidence(confidence, detections, now)
	seen := map[string]bool{}
	for _, task := range tasks {
		if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_input" {
			continue
		}
		var taskStatuses map[string]string
		for _, accountID := range taskAccountIDs(task) {
			if seen[accountID] {
				continue
			}
			seen[accountID] = true
			if taskStatuses == nil {
				taskStatuses = accountTaskStatuses(task)
			}
			status := taskStatuses[accountID]
			if status == "" {
				status = "inconclusive"
			}
			statuses = append(statuses, AccountCheckStatus{
				AccountID:  accountID,
				Status:     status,
				CheckedAt:  task.UpdatedAt,
				TaskID:     task.ID,
				Confidence: confidence[accountID],
			})
		}
	}
	for accountID, statistics := range confidence {
		if !seen[accountID] {
			statuses = append(statuses, AccountCheckStatus{AccountID: accountID, Status: "not_checked", Confidence: statistics})
		}
	}
	sort.SliceStable(statuses, func(i, j int) bool { return statuses[i].AccountID < statuses[j].AccountID })
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
			"account_ids": prepared.request.AccountIDs, "phase": "queued", "completed": 0,
			"total": len(prepared.accounts) * len(prepared.request.Models), "tests": []map[string]any{}, "system_info": true,
			"profile_version": prepared.profileVersion, "profile_fingerprint": prepared.profileFingerprint,
			"credentials_persisted": false,
		}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) { s.execute(parent, task, prepared) }); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) prepare(ctx context.Context, request Request) (preparedRun, error) {
	s.profilesMu.RLock()
	claudeProfiles := s.claudeProfiles
	solProfile := s.solProfile
	profileVersion := s.configuration.Active.ID
	profileFingerprint := s.configuration.Active.Fingerprint
	s.profilesMu.RUnlock()
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
		if !utf8.ValidString(model) || utf8.RuneCountInString(model) > 256 || strings.ContainsAny(model, "\r\n\x00") {
			return preparedRun{}, errors.New("模型名称无效，请重新选择")
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
	selected, err := s.selectCheckAccounts(ctx, request.AccountIDs)
	if err != nil {
		return preparedRun{}, err
	}
	profileVersion, profileFingerprint = verificationVersion, verificationVersion
	if request.PublicReference {
		profileVersion += "+" + modelfingerprint.Version
		profileFingerprint = modelfingerprint.Digest()
	}
	for _, account := range selected {
		if account.AccountType == "oauth" {
			for _, model := range request.Models {
				if strings.HasPrefix(model, "claude-") || strings.HasPrefix(model, "gemini-") {
					return preparedRun{}, errors.New("OpenAI OAuth 账号不支持所选平台模型，请重新选择")
				}
			}
		}
	}
	target, err := s.prepareOAuthTarget(ctx, selected)
	if err != nil {
		return preparedRun{}, err
	}
	return preparedRun{
		request: request, accounts: selected,
		claudeProfiles: claudeProfiles, solProfile: solProfile,
		profileVersion: profileVersion, profileFingerprint: profileFingerprint,
		oauthTarget: target,
	}, nil
}

func (s *Service) selectCheckAccounts(ctx context.Context, accountIDs []string) ([]selectedAccount, error) {
	selected := make([]selectedAccount, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if err := business.CheckAccountUpstreamManaged(ctx, s.accounts, accountID); err != nil {
			if errors.Is(err, business.ErrUpstreamExcluded) {
				continue
			}
			return nil, err
		}
		detail, err := s.detectionAccount(ctx, accountID)
		if errors.Is(err, sql.ErrNoRows) || err == nil && detail == nil {
			return nil, fmt.Errorf("账号 %s 不存在，请刷新账号列表后重试", accountID)
		}
		if err != nil {
			return nil, fmt.Errorf("账号 %s 详情读取失败", accountID)
		}
		if detail.ID != accountID {
			return nil, fmt.Errorf("账号 %s 详情的稳定 ID 不匹配", accountID)
		}
		selected = append(selected, directAccountSelection(detail.AccountStatus, detail))
	}
	return selected, nil
}

func (s *Service) checkerForModel(model string) string {
	s.profilesMu.RLock()
	defer s.profilesMu.RUnlock()
	return checkerForModel(model, s.claudeProfiles, s.solProfile)
}

func checkerForModel(model string, claudeProfiles map[string]claudeProfile, solProfile solProfile) string {
	if model == astraModel {
		return "astra"
	}
	if inferClaudeStandard(model, claudeProfiles) != "" {
		return "claude"
	}
	for _, candidate := range solProfile.Models {
		if model == candidate {
			return "sol"
		}
	}
	return ""
}

func (s *Service) execute(parent context.Context, task taskstore.Task, prepared preparedRun) {
	ctx, cancel := context.WithTimeout(parent, s.taskTimeout)
	defer cancel()
	task.Status, task.Progress, task.Message = "running", 3, "正在准备账号凭据"
	task.Result = map[string]any{
		"account_ids": prepared.request.AccountIDs, "phase": "credentials", "completed": 0,
		"total": len(prepared.accounts) * len(prepared.request.Models), "tests": []map[string]any{}, "system_info": true,
		"profile_version": prepared.profileVersion, "profile_fingerprint": prepared.profileFingerprint,
		"credentials_persisted": false,
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	if prepared.oauthTarget != nil {
		ctx = targetguard.Expect(ctx, *prepared.oauthTarget)
	}
	guarded, release, err := s.acquirePreparedAccounts(ctx, prepared.accounts, true)
	if err != nil {
		s.finishFailed(ctx, task, prepared.request.AccountIDs, err)
		return
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("账号模型检测租约释放失败", "account_ids", prepared.request.AccountIDs, "error", err)
		}
	}()
	ctx = guarded
	managed := make([]selectedAccount, 0, len(prepared.accounts))
	for _, account := range prepared.accounts {
		if err := business.CheckAccountUpstreamManaged(ctx, s.accounts, account.ID); err != nil {
			if errors.Is(err, business.ErrUpstreamExcluded) {
				continue
			}
			s.finishFailed(ctx, task, prepared.request.AccountIDs, err)
			return
		}
		managed = append(managed, account)
	}
	prepared.accounts = managed
	prepared.request.AccountIDs = make([]string, 0, len(managed))
	for _, account := range managed {
		prepared.request.AccountIDs = append(prepared.request.AccountIDs, account.ID)
	}
	if prepared.oauthTarget != nil {
		ctx, err = targetguard.Pin(ctx, s.credentials.(targetguard.Store))
		if err != nil {
			s.finishFailed(guarded, task, prepared.request.AccountIDs, errors.New("管理目标在模型检测排队后已变化或不可用，请重新提交"))
			return
		}
	}
	credentials := s.resolveCredentials(ctx, prepared.accounts)
	credentialsPersisted := credentialsResolved(credentials)
	client := &http.Client{
		Timeout:       time.Duration(prepared.request.TimeoutSeconds) * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	s.profilesMu.RLock()
	oauthTransport := s.oauthTransport
	s.profilesMu.RUnlock()
	if oauthTransport == nil {
		oauthTransport = newOAuthDirectTransport(prepared.request.TimeoutSeconds)
	}
	oauthClient := &http.Client{Transport: oauthTransport, Timeout: client.Timeout, CheckRedirect: client.CheckRedirect}
	defer oauthClient.CloseIdleConnections()
	task.Progress, task.Message = 5, "正在并行检测账号与模型组合"
	task.Result = map[string]any{
		"account_ids": prepared.request.AccountIDs, "phase": "testing", "completed": 0,
		"total": len(prepared.accounts) * len(prepared.request.Models), "tests": []map[string]any{},
		"profile_version": prepared.profileVersion, "profile_fingerprint": prepared.profileFingerprint,
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
	type combinationEvent struct {
		index    int
		started  bool
		progress bool
		result   map[string]any
	}
	jobs := make(chan int)
	challenges := make([][]verificationChallenge, prepared.request.Rounds)
	for round := range challenges {
		var err error
		challenges[round], err = verificationChallenges()
		if err != nil {
			s.finishFailed(ctx, task, prepared.request.AccountIDs, errors.New("检测题目生成失败，请重试"))
			return
		}
	}
	workers := min(2, len(prepared.accounts))
	// A single ordered stream prevents a late start event from reviving a
	// completed combination. Buffer only in proportion to active workers.
	events := make(chan combinationEvent, workers)
	var workerGroup sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			for accountIndex := range jobs {
				for modelIndex := range prepared.request.Models {
					index := accountIndex*len(prepared.request.Models) + modelIndex
					if ctx.Err() != nil {
						return
					}
					current := combinations[index]
					events <- combinationEvent{index: index, started: true, result: map[string]any{
						"account_id": current.account.ID, "account_name": current.account.Name,
						"claimed_model": current.model,
						"mode":          "verification",
					}}
					credential := credentials[current.account.ID]
					input := targetRequest{
						Challenges:  challenges,
						Fingerprint: prepared.request.Fingerprint, BehaviorFingerprint: prepared.request.BehaviorFingerprint, PublicReference: prepared.request.PublicReference,
						AccountID: current.account.ID, AccountName: current.account.Name,
						Model: current.model, Rounds: prepared.request.Rounds,
						TimeoutSeconds: prepared.request.TimeoutSeconds,
					}
					input.Progress = func(report VerificationReport) {
						raw, _ := json.Marshal(report)
						snapshot := map[string]any{}
						if json.Unmarshal(raw, &snapshot) != nil {
							return
						}
						snapshot["account_id"], snapshot["account_name"], snapshot["claimed_model"] = current.account.ID, current.account.Name, current.model
						snapshot["completed_probes"], snapshot["planned_probes"] = len(report.Probes), report.Total
						snapshot["checker"], snapshot["timeout_seconds"] = "verification", prepared.request.TimeoutSeconds
						events <- combinationEvent{index: index, progress: true, result: snapshot}
					}
					result, runErr := runPreparedCheck(ctx, client, oauthClient, credential, prepared, input)
					if runErr != nil {
						result = failedVerification(current.account, input, runErr)
					}
					result["credentials_persisted"] = credential.err == nil && credential.oauth == nil
					events <- combinationEvent{index: index, result: result}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range prepared.accounts {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workerGroup.Wait()
		close(events)
	}()
	results := make([]map[string]any, len(combinations))
	completedResults := make([]map[string]any, 0, len(combinations))
	completed := 0
	active := map[int]map[string]any{}
	for event := range events {
		if event.progress {
			active[event.index] = event.result
			task.Message = fmt.Sprintf("账号 %s：已完成 %v/%v 项检测", event.result["account_name"], event.result["completed_probes"], event.result["planned_probes"])
			if current, ok := event.result["current_probes"].([]any); ok && len(current) > 1 {
				task.Message = fmt.Sprintf("账号 %s · %s：%d 项检测正在并发执行", event.result["account_name"], event.result["claimed_model"], len(current))
			} else if current, ok := event.result["current_probe"].(map[string]any); ok {
				task.Message = fmt.Sprintf("账号 %s · %s：第 %v 轮「%v」等待回复", event.result["account_name"], event.result["claimed_model"], current["round"], current["label"])
			}
		} else if event.started {
			active[event.index] = event.result
			task.Message = fmt.Sprintf("正在检测账号 %s 的模型 %s；已完成 %d/%d 个组合", event.result["account_name"], event.result["claimed_model"], completed, len(combinations))
		} else {
			delete(active, event.index)
			results[event.index] = event.result
			completedResults = append(completedResults, event.result)
			completed++
			task.Message = fmt.Sprintf("已完成 %d/%d 个账号模型组合", completed, len(combinations))
		}
		finishedUnits, plannedUnits := 0, 0
		for index, combination := range combinations {
			count := verificationProbeCount(prepared.request.Rounds, prepared.request.Fingerprint, prepared.request.BehaviorFingerprint)
			if credentials[combination.account.ID].oauth == nil {
				count += publicReferenceCount(combination.model, prepared.request.PublicReference)
			}
			plannedUnits += count
			if results[index] != nil {
				finishedUnits += count
			}
		}
		for _, item := range active {
			if count, ok := item["completed_probes"].(int); ok {
				finishedUnits += count
			}
		}
		task.Progress = 5 + finishedUnits*90/max(1, plannedUnits)
		queued := make([]map[string]any, 0)
		for index, combination := range combinations {
			if results[index] == nil && active[index] == nil {
				queued = append(queued, map[string]any{"account_id": combination.account.ID, "account_name": combination.account.Name, "claimed_model": combination.model})
			}
		}
		task.Result = map[string]any{
			"account_ids": prepared.request.AccountIDs, "completed": completed, "total": len(combinations),
			"phase": "testing", "tests": slices.Clone(completedResults), "system_info": true,
			"active":          maps.Clone(active),
			"queued":          queued,
			"profile_version": prepared.profileVersion, "profile_fingerprint": prepared.profileFingerprint,
			"credentials_persisted": credentialsPersisted,
		}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	if completed != len(combinations) {
		s.finishFailed(ctx, task, prepared.request.AccountIDs, errors.New("账号模型检测被中断"))
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
		"phase": "completed", "completed": len(combinations), "total": len(combinations), "system_info": true,
		"profile_version": prepared.profileVersion, "profile_fingerprint": prepared.profileFingerprint,
		"remote_write": false, "credentials_persisted": credentialsPersisted,
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.MarkCancelled(ctx, &task, "账号模型检测已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) acquirePreparedAccounts(
	ctx context.Context,
	expected []selectedAccount,
	skipExcluded ...bool,
) (context.Context, func() error, error) {
	if len(expected) == 0 {
		return ctx, func() error { return nil }, nil
	}
	resources := make([]string, 0, len(expected)*2)
	for _, account := range expected {
		resources = append(resources, mutationguard.Account(account.ID))
		if resource := mutationguard.Upstream(account.AuthHost); resource != "" {
			resources = append(resources, resource)
		}
	}
	guarded, release, err := mutationguard.Acquire(ctx, s.accounts, resources...)
	if err != nil {
		return nil, nil, fmt.Errorf("账号模型检测租约获取失败：%w", err)
	}
	fail := func(err error) (context.Context, func() error, error) {
		if releaseErr := release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("账号模型检测租约释放失败：%w", releaseErr))
		}
		return nil, nil, err
	}
	for _, queued := range expected {
		if err := business.CheckAccountUpstreamManaged(guarded, s.accounts, queued.ID); err != nil {
			if len(skipExcluded) > 0 && skipExcluded[0] && errors.Is(err, business.ErrUpstreamExcluded) {
				continue
			}
			return fail(err)
		}
		detail, readErr := s.detectionAccount(guarded, queued.ID)
		if readErr != nil {
			return fail(fmt.Errorf("获取模型检测租约后账号 %s 详情读取失败：%w", queued.ID, readErr))
		}
		if detail == nil {
			return fail(fmt.Errorf("账号 %s 在模型检测排队后已被删除", queued.ID))
		}
		if current := directAccountSelection(detail.AccountStatus, detail); current != queued {
			return fail(fmt.Errorf("账号 %s 在模型检测排队后配置或 Key 绑定已变化，请重新提交", queued.ID))
		}
	}
	return guarded, release, nil
}

func (s *Service) finishFailed(ctx context.Context, task taskstore.Task, accountIDs []string, err error) {
	task.Status, task.Progress = "failed", 100
	task.Message = "账号模型检测失败：" + err.Error()
	task.Result = maps.Clone(task.Result)
	if task.Result == nil {
		task.Result = map[string]any{}
	}
	task.Result["account_ids"] = accountIDs
	task.Result["error"] = err.Error()
	if _, ok := task.Result["credentials_persisted"]; !ok {
		task.Result["credentials_persisted"] = false
	}
	delete(task.Result, "active")
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.MarkCancelled(ctx, &task, "账号模型检测已取消")
	task.Result["phase"] = task.Status
	taskstore.PersistFinal(s.tasks, task)
}

type credentialResolution struct {
	value directCredential
	oauth *oauthCredential
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
				if ctx.Err() != nil {
					return
				}
				var resolution credentialResolution
				if account.AccountType == "oauth" {
					resolution.oauth, resolution.err = s.resolveOAuthAccountCredential(ctx, account)
				} else {
					resolution.value, resolution.err = s.resolveCredential(ctx, account)
				}
				resultsMu.Lock()
				results[account.ID] = resolution
				resultsMu.Unlock()
			}
		}()
	}
	_ = taskrunner.Feed(ctx, jobs, accounts)
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
		if value.err != nil || value.oauth != nil {
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
	selected.AccountType = strings.ToLower(firstPointerText(detail.AccountType))
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

func accountTaskStatuses(task taskstore.Task) map[string]string {
	statuses := map[string]string{}
	if task.Status != "succeeded" {
		return statuses
	}
	type counts struct{ matched, consistent, inconsistent int }
	byAccount := map[string]counts{}
	for _, result := range taskResultRows(task.Result["tests"]) {
		accountID, ok := result["account_id"].(string)
		if !ok || accountID == "" {
			continue
		}
		count := byAccount[accountID]
		count.matched++
		switch modelCheckOutcome(result) {
		case "passed":
			count.consistent++
		case "failed":
			count.inconsistent++
		}
		byAccount[accountID] = count
	}
	for id, count := range byAccount {
		switch {
		case count.inconsistent > 0:
			statuses[id] = "inconsistent"
		case count.matched > 0 && count.consistent == count.matched:
			statuses[id] = "consistent"
		default:
			statuses[id] = "inconclusive"
		}
	}
	return statuses
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
	case json.Number:
		value, err := number.Float64()
		return value, err == nil
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
