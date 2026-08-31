package authrecovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamdetect"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type Repository interface {
	PersistAuthRecoveryOutcomes(context.Context, []business.AuthRecoveryOutcome, string) (business.AuthRecoverySummary, error)
}

type HostMetadataSource interface {
	UpstreamAuthSeed(context.Context, string) (*business.UpstreamAuthSeed, error)
}

type PrivateStore interface {
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
	AuthRecordIndex(context.Context) ([]configstore.AuthRecordSummary, error)
	SaveAuthRecord(context.Context, configstore.AuthRecord, map[string]bool) error
	VaultEntry(context.Context, string) (*configstore.VaultEntry, error)
	VaultEntryIndex(context.Context) ([]configstore.VaultEntrySummary, error)
}

type Authenticator interface {
	Refresh(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error)
	Login(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error)
}

type Configurator interface {
	ConfigureAuthRecord(context.Context, upstreamconfig.Input) (string, error)
}

type PlatformDetector interface {
	Detect(context.Context, string) (upstreamdetect.Result, error)
}

type recoveredAuthCommitter interface {
	CommitRecoveredAuth(context.Context, configstore.AuthRecord) error
}

type upstreamProvisioner interface {
	Create(context.Context, upstreamconfig.Input, string) (upstreamconfig.Configuration, error)
}

type BalanceSyncer interface {
	SyncHost(context.Context, string, upstreamsync.Scope, string) (upstreamsync.HostResult, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type taskReader interface {
	Get(context.Context, string) (taskstore.Task, error)
}

type CaptchaFlow interface {
	Prepare(context.Context, configstore.AuthRecord, string, *string) (CaptchaChallenge, error)
	PrepareCredential(context.Context, configstore.AuthRecord, configstore.VaultEntry, bool, *string) (CaptchaChallenge, error)
	Submit(context.Context, string, string) (CaptchaResult, error)
	Cancel(string) (*CaptchaChallenge, *string)
}

type ManualInput struct {
	Host         string
	AuthMode     *string
	AccessToken  *string
	RefreshToken *string
	AdminKey     *string
	UserID       *string
	Username     *string
	Password     *string
	SaveToVault  bool
	Entry        *string
	Headers      map[string]string
	Present      map[string]bool
}

type BalanceResult struct {
	Status        string  `json:"status"`
	BalanceStatus string  `json:"balance_status"`
	Balance       *string `json:"balance,omitempty"`
	Reason        *string `json:"reason,omitempty"`
}

type ManualResult struct {
	Host             string            `json:"host"`
	Verified         bool              `json:"verified"`
	BalanceSync      *BalanceResult    `json:"balance_sync,omitempty"`
	CaptchaChallenge *CaptchaChallenge `json:"captcha_challenge,omitempty"`
}

type CaptchaCompletion struct {
	CaptchaResult
	Projection  business.AuthRecoverySummary `json:"projection"`
	BalanceSync BalanceResult                `json:"balance_sync"`
}

type Service struct {
	repository    Repository
	private       PrivateStore
	authenticator Authenticator
	configurator  Configurator
	balances      BalanceSyncer
	tasks         TaskStore
	captcha       CaptchaFlow
	detector      PlatformDetector
	timeout       time.Duration
}

func (s *Service) UsePlatformDetector(detector PlatformDetector) {
	s.detector = detector
}

func New(repository Repository, private PrivateStore, authenticator Authenticator, configurator Configurator, balances BalanceSyncer, tasks TaskStore, captcha ...CaptchaFlow) *Service {
	service := &Service{
		repository: repository, private: private, authenticator: authenticator, configurator: configurator,
		balances: balances, tasks: tasks, timeout: 10 * time.Minute,
	}
	if len(captcha) > 0 {
		service.captcha = captcha[0]
	}
	return service
}

func (s *Service) VerifyManual(ctx context.Context, input ManualInput, actor string) (ManualResult, error) {
	host := configstore.CanonicalHost(input.Host)
	if host == "" {
		return ManualResult{}, errors.New("上游 Host 不能为空")
	}
	current, err := s.recoveryRecord(ctx, host)
	if err != nil {
		return ManualResult{}, err
	}
	mode := defaultManualMode(*current)
	if input.AuthMode != nil && strings.TrimSpace(*input.AuthMode) != "" {
		mode = strings.TrimSpace(*input.AuthMode)
	}
	headers := cloneMap(current.Headers)
	explicitHeaders := input.Present["headers"]
	if explicitHeaders {
		headers = cloneMap(input.Headers)
	}
	replaceAuthorizationHeader := directAuthorizationWasProvided(mode, input) && !explicitHeaders
	if replaceAuthorizationHeader {
		headers = withoutBearerAuthorization(headers)
	}
	configuration := upstreamconfig.Input{
		Host: host, BaseURL: current.BaseURL, UpstreamType: current.UpstreamType, AuthMode: mode, RechargeRate: "1",
		AccessToken: input.AccessToken, RefreshToken: input.RefreshToken, AdminKey: input.AdminKey, UserID: input.UserID,
		Username: input.Username, Password: input.Password, SaveToVault: input.SaveToVault, Entry: input.Entry,
		Headers: headers, Cookies: cloneMap(current.Cookies), Present: map[string]bool{},
	}
	for key, present := range input.Present {
		configuration.Present[key] = present
	}
	if replaceAuthorizationHeader {
		configuration.Present["headers"] = true
	}
	setCredentialClears(&configuration, mode)
	verifiedHost, err := s.configurator.ConfigureAuthRecord(ctx, configuration)
	if err != nil {
		var interaction *upstreamauth.InteractionError
		if errors.As(err, &interaction) && interaction.Code == "image_captcha_required" && isManualAuthMode(mode) {
			if s.captcha == nil {
				return ManualResult{}, errors.New("图片验证码恢复服务尚未就绪")
			}
			if input.Username == nil || strings.TrimSpace(*input.Username) == "" || input.Password == nil || *input.Password == "" {
				return ManualResult{}, errors.New("图片验证码登录必须填写用户名和密码")
			}
			entry := host
			if input.Entry != nil && strings.TrimSpace(*input.Entry) != "" {
				entry = strings.TrimSpace(*input.Entry)
			}
			candidate := cloneAuthRecord(*current)
			candidate.Host, candidate.AuthMode, candidate.Headers = host, mode, cloneMap(headers)
			credential := configstore.VaultEntry{
				Entry: entry, Username: input.Username, Password: input.Password,
				Hosts: []string{host}, Headers: cloneMap(headers),
			}
			challenge, prepareErr := s.captcha.PrepareCredential(ctx, candidate, credential, input.SaveToVault, nil)
			if prepareErr != nil {
				return ManualResult{}, fmt.Errorf("图片验证码准备失败：%w", prepareErr)
			}
			return ManualResult{Host: host, Verified: false, CaptchaChallenge: &challenge}, nil
		}
		return ManualResult{}, err
	}
	result, syncErr := s.balances.SyncHost(ctx, verifiedHost, upstreamsync.Scope{Balance: true}, actor)
	balance := balanceResult(result, syncErr)
	return ManualResult{Host: verifiedHost, Verified: true, BalanceSync: &balance}, nil
}

func (s *Service) Enqueue(ctx context.Context, host, entry, actor string) (taskstore.Task, error) {
	host = configstore.CanonicalHost(host)
	entry = strings.TrimSpace(entry)
	if host == "" {
		return taskstore.Task{}, errors.New("上游 Host 不能为空")
	}
	record, err := s.recoveryRecord(ctx, host)
	if err != nil {
		return taskstore.Task{}, err
	}
	if entry != "" {
		credential, err := s.private.VaultEntry(ctx, entry)
		if err != nil {
			return taskstore.Task{}, err
		}
		if credential == nil {
			return taskstore.Task{}, errors.New("所选密码箱项不存在")
		}
	}
	id, err := recoveryTaskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-upstream-auth", Operation: "recover-host", Status: "queued", Progress: 0,
		Message: "鉴权恢复已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go s.execute(task, *record, entry, actor)
	return task, nil
}

// ResolveAuth provisions a missing exact-Host authorization only from an explicit,
// unambiguous password-vault association.
func (s *Service) ResolveAuth(ctx context.Context, host, actor string) (*configstore.AuthRecord, error) {
	host = configstore.CanonicalHost(host)
	if host == "" {
		return nil, errors.New("上游 Host 不能为空")
	}
	record, err := s.private.AuthRecord(ctx, host)
	if err != nil || record != nil {
		return record, err
	}

	entries, err := s.private.VaultEntryIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("密码箱索引读取失败：%w", err)
	}
	matches := matchingVaultEntries(entries, host)
	if len(matches) == 0 {
		return nil, fmt.Errorf("密码箱中没有显式关联 Host %q 的凭据", host)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("Host %q 匹配到多个密码箱项（%s），请只保留一个明确关联", host, strings.Join(vaultEntryNames(matches), "、"))
	}

	seed, publicExists, err := s.resolveAuthSeed(ctx, host, matches[0])
	if err != nil {
		return nil, err
	}
	mode := recoveryLoginMode(seed.UpstreamType)
	if mode == "" {
		return nil, fmt.Errorf("密码箱匹配到的上游类型 %q 不支持账号密码恢复", seed.UpstreamType)
	}
	entry := matches[0].Entry
	name := host
	input := upstreamconfig.Input{
		Host: host, Name: &name, BaseURL: seed.BaseURL, UpstreamType: seed.UpstreamType, AuthMode: mode,
		RechargeRate: "1", Entry: &entry, Present: map[string]bool{"entry": true},
	}
	if publicExists {
		if _, err := s.configurator.ConfigureAuthRecord(ctx, input); err != nil {
			return nil, err
		}
	} else {
		provisioner, ok := s.configurator.(upstreamProvisioner)
		if !ok {
			return nil, errors.New("鉴权恢复服务无法创建缺失的上游配置")
		}
		if _, err := provisioner.Create(ctx, input, actor); err != nil {
			return nil, err
		}
	}
	record, err = s.private.AuthRecord(ctx, host)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("上游登录已完成，但未生成私有授权记录")
	}
	return record, nil
}

func matchingVaultEntries(entries []configstore.VaultEntrySummary, host string) []configstore.VaultEntrySummary {
	result := []configstore.VaultEntrySummary{}
	for _, entry := range entries {
		for _, candidate := range entry.Hosts {
			if configstore.CanonicalHost(candidate) == host {
				result = append(result, entry)
				break
			}
		}
	}
	return result
}

func vaultEntryNames(entries []configstore.VaultEntrySummary) []string {
	result := make([]string, len(entries))
	for index := range entries {
		result[index] = entries[index].Entry
	}
	sort.Strings(result)
	return result
}

func (s *Service) resolveAuthSeed(ctx context.Context, host string, entry configstore.VaultEntrySummary) (business.UpstreamAuthSeed, bool, error) {
	source, ok := s.repository.(HostMetadataSource)
	if !ok {
		return business.UpstreamAuthSeed{}, false, errors.New("无法读取上游基础信息")
	}
	seed, err := source.UpstreamAuthSeed(ctx, host)
	if err != nil {
		return business.UpstreamAuthSeed{}, false, err
	}
	if seed != nil {
		return *seed, true, nil
	}

	records, err := s.private.AuthRecordIndex(ctx)
	if err != nil {
		return business.UpstreamAuthSeed{}, false, err
	}
	associatedHosts := map[string]struct{}{}
	for _, value := range entry.Hosts {
		if normalized := configstore.CanonicalHost(value); normalized != "" {
			associatedHosts[normalized] = struct{}{}
		}
	}
	if unqualifiedHost := hostWithoutPort(host); unqualifiedHost != "" {
		for _, related := range records {
			_, hostAssociated := associatedHosts[configstore.CanonicalHost(related.Host)]
			_, baseURLAssociated := associatedHosts[configstore.CanonicalHost(related.BaseURL)]
			if configstore.CanonicalHost(related.Host) == unqualifiedHost && (hostAssociated || baseURLAssociated) && recoveryLoginMode(related.UpstreamType) != "" {
				return business.UpstreamAuthSeed{Host: host, BaseURL: related.BaseURL, UpstreamType: related.UpstreamType}, false, nil
			}
		}
	}
	candidates := map[string]business.UpstreamAuthSeed{}
	for _, related := range records {
		_, hostAssociated := associatedHosts[configstore.CanonicalHost(related.Host)]
		_, baseURLAssociated := associatedHosts[configstore.CanonicalHost(related.BaseURL)]
		if (!hostAssociated && !baseURLAssociated) || recoveryLoginMode(related.UpstreamType) == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(related.UpstreamType)) + "\x00" + strings.TrimSpace(related.BaseURL)
		candidates[key] = business.UpstreamAuthSeed{
			Host: host, BaseURL: related.BaseURL, UpstreamType: related.UpstreamType,
		}
	}
	if len(candidates) == 0 {
		return business.UpstreamAuthSeed{}, false, fmt.Errorf("密码箱项 %q 已关联 Host %q，但缺少可复用的平台和 Base URL 配置", entry.Entry, host)
	}
	if len(candidates) > 1 {
		return business.UpstreamAuthSeed{}, false, fmt.Errorf("密码箱项 %q 关联了多组不同的平台或 Base URL，请先明确 Host %q 使用哪一组配置", entry.Entry, host)
	}
	for _, candidate := range candidates {
		return candidate, false, nil
	}
	panic("unreachable")
}

func hostWithoutPort(host string) string {
	parsed, err := url.Parse("//" + configstore.CanonicalHost(host))
	if err != nil || parsed.Port() == "" {
		return ""
	}
	return configstore.CanonicalHost(parsed.Hostname())
}

func (s *Service) execute(task taskstore.Task, record configstore.AuthRecord, entry, actor string) {
	host := record.Host
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 15, "正在尝试刷新鉴权", time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	outcome := s.recover(ctx, record, entry)
	var challenge *CaptchaChallenge
	if pointerOr(outcome.Code, "") == "image_captcha_required" && entry != "" && s.captcha != nil {
		prepared, prepareErr := s.captcha.Prepare(ctx, record, entry, &task.ID)
		if prepareErr != nil {
			reason := "图片验证码准备失败：" + safeReason(prepareErr.Error())
			outcome = failedOutcome(outcome, "image_captcha_prepare_failed", reason, false, outcome.RefreshAttempt, stringPointer("image_captcha_ocr"))
		} else {
			code, reason, kind := "image_captcha_ocr", "图片验证码已准备，等待输入后继续复核", "image_captcha_ocr"
			outcome.Code, outcome.Reason, outcome.InteractionKind = &code, &reason, &kind
			challenge = &prepared
		}
	}
	summary, persistErr := s.repository.PersistAuthRecoveryOutcomes(ctx, []business.AuthRecoveryOutcome{outcome}, actor)
	var balance *BalanceResult
	if outcome.Success {
		value, err := s.balances.SyncHost(ctx, host, upstreamsync.Scope{Balance: true}, actor)
		converted := balanceResult(value, err)
		balance = &converted
	}
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = map[string]any{
		"host": host, "outcome": outcome, "projection": summary, "credentials_persisted": outcome.Success,
		"balance_sync": balance,
	}
	if challenge != nil {
		task.Result["captcha_challenge"] = *challenge
	}
	if persistErr != nil {
		task.Status, task.Message = "failed", "鉴权结果保存失败："+safeReason(persistErr.Error())
		task.Result["error"] = safeReason(persistErr.Error())
	} else if challenge != nil {
		task.Status, task.Progress, task.Message = "waiting_input", 90, "图片验证码已准备，输入验证码后继续恢复"
	} else if outcome.Success {
		task.Status = "succeeded"
		if balance != nil && balance.Status == "succeeded" {
			task.Message = "鉴权已恢复，余额读取成功"
		} else {
			task.Message = "鉴权已恢复，余额读取失败"
		}
	} else {
		task.Status, task.Message = "failed", "鉴权恢复未通过："+pointerOr(outcome.Code, "已记录原因")
	}
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) SubmitCaptcha(ctx context.Context, challengeID, code, actor string) (CaptchaCompletion, error) {
	if s.captcha == nil {
		return CaptchaCompletion{}, errors.New("图片验证码恢复服务尚未就绪")
	}
	result, err := s.captcha.Submit(ctx, challengeID, code)
	if err != nil {
		return CaptchaCompletion{}, err
	}
	outcome := successfulOutcome(business.AuthRecoveryOutcome{Host: result.Host, Attempted: true},
		"recovered_by_image_captcha", "图片验证码登录成功并完成鉴权复核", "vault")
	kind := "image_captcha_ocr"
	outcome.InteractionKind = &kind
	projection, err := s.repository.PersistAuthRecoveryOutcomes(ctx, []business.AuthRecoveryOutcome{outcome}, actor)
	if err != nil {
		s.finishCaptchaParent(result.ParentTaskID, "failed", "图片验证码已复核，但鉴权结果保存失败", map[string]any{
			"host": result.Host, "credentials_persisted": true, "error": safeReason(err.Error()),
		})
		return CaptchaCompletion{}, err
	}
	value, syncErr := s.balances.SyncHost(ctx, result.Host, upstreamsync.Scope{Balance: true}, actor)
	balance := balanceResult(value, syncErr)
	message := "图片验证码恢复成功，余额读取失败"
	if balance.Status == "succeeded" {
		message = "图片验证码恢复成功，余额读取成功"
	}
	s.finishCaptchaParent(result.ParentTaskID, "succeeded", message, map[string]any{
		"host": result.Host, "outcome": outcome, "projection": projection, "credentials_persisted": true,
		"balance_sync": balance, "captcha_recovery": result,
	})
	return CaptchaCompletion{CaptchaResult: result, Projection: projection, BalanceSync: balance}, nil
}

func (s *Service) CancelCaptcha(challengeID string) bool {
	if s.captcha == nil {
		return false
	}
	challenge, parentTaskID := s.captcha.Cancel(challengeID)
	if challenge == nil {
		return false
	}
	s.finishCaptchaParent(parentTaskID, "cancelled", "图片验证码恢复已取消", map[string]any{
		"host": challenge.Host, "cancelled": true, "credentials_persisted": false,
	})
	return true
}

func (s *Service) finishCaptchaParent(parentTaskID *string, status, message string, result map[string]any) {
	if parentTaskID == nil || strings.TrimSpace(*parentTaskID) == "" {
		return
	}
	reader, ok := s.tasks.(taskReader)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	task, err := reader.Get(ctx, *parentTaskID)
	if err != nil {
		return
	}
	task.Status, task.Progress, task.Message, task.Result = status, 100, message, result
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := taskstore.SaveFinal(ctx, s.tasks, task); err != nil {
		return
	}
}

func (s *Service) recover(ctx context.Context, record configstore.AuthRecord, entry string) business.AuthRecoveryOutcome {
	host := record.Host
	outcome := business.AuthRecoveryOutcome{Host: host, Attempted: true}
	originalType, originalMode := record.UpstreamType, record.AuthMode
	if detected, err := s.detectRecoveryPlatform(ctx, record); err == nil {
		record = detected
	}
	classificationChanged := !strings.EqualFold(originalType, record.UpstreamType) || originalMode != record.AuthMode
	var refreshReason string
	if record.RefreshToken != nil && strings.TrimSpace(*record.RefreshToken) != "" {
		rotated, refreshErr := s.authenticator.Refresh(ctx, record)
		if refreshErr == nil {
			if saveErr := s.commitRecoveredAuth(ctx, rotated, classificationChanged); saveErr != nil {
				return failedOutcome(outcome, "credential_commit_failed", "refresh 已复核但凭据保存失败："+saveErr.Error(), false, stringPointer("refresh 已复核"), stringPointer("refresh"))
			}
			return successfulOutcome(outcome, "recovered_by_refresh", "refresh token 续签并复核成功", "refresh")
		}
		refreshReason = safeReason(refreshErr.Error())
	}
	if entry == "" {
		reason := "未选择密码箱项"
		if refreshReason != "" {
			reason = "refresh_token 续签失败：" + refreshReason + "；未选择密码箱项"
		}
		return failedOutcome(outcome, "vault_entry_required", reason, false, optionalPointer(refreshReason), stringPointer("refresh"))
	}
	credential, err := s.private.VaultEntry(ctx, entry)
	if err != nil || credential == nil {
		return failedOutcome(outcome, "vault_entry_unavailable", errorOr(err, "所选密码箱项不存在"), false, optionalPointer(refreshReason), stringPointer("vault"))
	}
	loggedIn, err := s.authenticator.Login(ctx, record, *credential)
	if err != nil {
		var interaction *upstreamauth.InteractionError
		if errors.As(err, &interaction) {
			kind := interaction.Code
			return failedOutcome(outcome, interaction.Code, interaction.Detail, false, optionalPointer(refreshReason), &kind)
		}
		reason := "密码箱登录复核失败：" + safeReason(err.Error())
		if refreshReason != "" {
			reason = "refresh_token 续签失败：" + refreshReason + "；" + reason
		}
		return failedOutcome(outcome, "vault_login_failed", reason, false, optionalPointer(refreshReason), stringPointer("vault"))
	}
	if err := s.commitRecoveredAuth(ctx, loggedIn, classificationChanged); err != nil {
		return failedOutcome(outcome, "credential_commit_failed", "密码箱登录已复核但凭据保存失败："+safeReason(err.Error()), false, optionalPointer(refreshReason), stringPointer("vault"))
	}
	return successfulOutcome(outcome, "recovered_by_vault", "所选密码箱项登录并复核成功", "vault")
}

func (s *Service) detectRecoveryPlatform(ctx context.Context, record configstore.AuthRecord) (configstore.AuthRecord, error) {
	if s.detector == nil {
		return record, nil
	}
	result, err := s.detector.Detect(ctx, record.BaseURL)
	if err != nil || !result.TypeDetected || result.UpstreamType == nil {
		return record, err
	}
	detected := strings.ToLower(strings.TrimSpace(*result.UpstreamType))
	if detected == "" || strings.EqualFold(detected, record.UpstreamType) {
		return record, nil
	}
	mode := recoveryModeForPlatform(record.AuthMode, detected)
	if mode == "" {
		return record, nil
	}
	record.UpstreamType, record.AuthMode = detected, mode
	return record, nil
}

func recoveryModeForPlatform(current, platform string) string {
	login := strings.Contains(strings.ToLower(strings.TrimSpace(current)), "login")
	switch platform {
	case "sub2api":
		if login {
			return "sub2api_user_login"
		}
		return "sub2api_user_token"
	case "newapi", "oneapi":
		if login {
			return "newapi_user_login"
		}
		return "newapi_user_token"
	default:
		return ""
	}
}

func (s *Service) commitRecoveredAuth(ctx context.Context, record configstore.AuthRecord, classificationChanged bool) error {
	if classificationChanged {
		if committer, ok := s.configurator.(recoveredAuthCommitter); ok {
			return committer.CommitRecoveredAuth(ctx, record)
		}
		return errors.New("鉴权已复核，但服务不支持提交平台类型修复")
	}
	return s.private.SaveAuthRecord(ctx, record, allAuthFields())
}

func (s *Service) recoveryRecord(ctx context.Context, host string) (*configstore.AuthRecord, error) {
	record, err := s.private.AuthRecord(ctx, host)
	if err != nil || record != nil {
		return record, err
	}
	source, ok := s.repository.(HostMetadataSource)
	if !ok {
		return nil, errors.New("该 Host 缺少鉴权配置，且无法读取上游基础信息")
	}
	seed, err := source.UpstreamAuthSeed(ctx, host)
	if err != nil {
		return nil, err
	}
	if seed == nil {
		return nil, errors.New("上游 Host 不存在，无法创建鉴权候选")
	}
	platform := strings.ToLower(strings.TrimSpace(seed.UpstreamType))
	mode := recoveryLoginMode(platform)
	if mode == "" {
		return nil, errors.New("该上游类型不支持密码箱恢复，请在编辑上游中配置鉴权方式")
	}
	return &configstore.AuthRecord{
		Host: seed.Host, BaseURL: seed.BaseURL, UpstreamType: platform, AuthMode: mode,
		Headers: map[string]string{}, Cookies: map[string]string{},
	}, nil
}

func recoveryLoginMode(platform string) string {
	switch platform {
	case "sub2api":
		return "sub2api_user_login"
	case "newapi", "oneapi":
		return "newapi_user_login"
	default:
		return ""
	}
}

func successfulOutcome(value business.AuthRecoveryOutcome, code, reason, kind string) business.AuthRecoveryOutcome {
	value.Success, value.Code, value.Reason, value.RefreshKind = true, &code, &reason, &kind
	attempt := "鉴权恢复已完成"
	value.RefreshAttempt = &attempt
	return value
}

func failedOutcome(value business.AuthRecoveryOutcome, code, reason string, transient bool, attempt, kind *string) business.AuthRecoveryOutcome {
	value.Success, value.Transient, value.Code, value.RefreshAttempt, value.RefreshKind = false, transient, &code, attempt, kind
	reason = safeReason(reason)
	value.Reason = &reason
	if code == "image_captcha_required" || code == "browser_challenge_required" {
		value.InteractionKind = &code
	}
	return value
}

func balanceResult(value upstreamsync.HostResult, err error) BalanceResult {
	if err != nil {
		reason := safeReason(err.Error())
		return BalanceResult{Status: "failed", BalanceStatus: "读取失败", Reason: &reason}
	}
	result := BalanceResult{Status: value.Status, BalanceStatus: value.BalanceStatus, Balance: value.Balance, Reason: value.Reason}
	if value.Status != "succeeded" {
		result.Status = "failed"
	}
	return result
}

func defaultManualMode(record configstore.AuthRecord) string {
	if strings.EqualFold(record.UpstreamType, "newapi") || strings.EqualFold(record.UpstreamType, "oneapi") {
		return "newapi_admin_key"
	}
	return "sub2api_user_token"
}

func isManualAuthMode(mode string) bool {
	return mode == "sub2api_manual_login" || mode == "newapi_manual_login"
}

func directCredentialMode(mode string) bool {
	return mode == "newapi_admin_key" || mode == "newapi_user_token" || mode == "sub2api_user_token" || mode == "bearer_token"
}

func directAuthorizationWasProvided(mode string, input ManualInput) bool {
	if !directCredentialMode(mode) {
		return false
	}
	if mode == "newapi_admin_key" {
		return input.Present["admin_key"]
	}
	return input.Present["access_token"]
}

func setCredentialClears(input *upstreamconfig.Input, mode string) {
	switch mode {
	case "newapi_admin_key":
		input.AccessToken, input.RefreshToken = nil, nil
		input.Present["access_token"], input.Present["refresh_token"] = true, true
	case "newapi_user_token", "sub2api_user_token", "bearer_token":
		input.AdminKey, input.UserID = nil, nil
		input.Present["admin_key"], input.Present["user_id"] = true, true
	}
}

func withoutBearerAuthorization(values map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range values {
		if strings.EqualFold(key, "authorization") && strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "bearer ") {
			continue
		}
		result[key] = value
	}
	return result
}

func cloneMap(values map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneAuthRecord(value configstore.AuthRecord) configstore.AuthRecord {
	result := value
	result.Headers, result.Cookies = cloneMap(value.Headers), cloneMap(value.Cookies)
	return result
}

func allAuthFields() map[string]bool {
	return map[string]bool{
		"base_url": true, "upstream_type": true, "auth_mode": true, "access_token": true, "refresh_token": true,
		"admin_key": true, "user_id": true, "headers": true, "cookies": true,
	}
}

func safeReason(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "鉴权恢复失败"
	}
	value = redact.Secrets(value)
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}

func errorOr(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	return safeReason(err.Error())
}

func optionalPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringPointer(value string) *string { return &value }

func pointerOr(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}

func recoveryTaskID() (string, error) {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
