package upstreamconfig

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
)

var (
	ErrNotFound = errors.New("上游 Host 不存在")
	ErrConflict = errors.New("上游 Host 已存在")
)

const postCommitOperationTimeout = 10 * time.Second

type InputError struct{ Err error }

func (e *InputError) Error() string { return e.Err.Error() }
func (e *InputError) Unwrap() error { return e.Err }

type Business interface {
	Upstreams(context.Context) (business.UpstreamSummary, error)
	UpstreamGroups(context.Context, string, bool) ([]business.UpstreamGroup, error)
	UpstreamExists(context.Context, string) (bool, error)
	CreateUpstreamConfiguration(context.Context, business.UpstreamConfigurationWrite) (business.UpstreamConfigurationWriteResult, error)
	UpdateUpstreamConfiguration(context.Context, business.UpstreamConfigurationWrite) (business.UpstreamConfigurationWriteResult, error)
	RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error)
}

type PrivateStore interface {
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
	SaveAuthRecord(context.Context, configstore.AuthRecord, map[string]bool) error
	DeleteAuthRecord(context.Context, string) (bool, error)
	VaultEntry(context.Context, string) (*configstore.VaultEntry, error)
	SaveVaultEntry(context.Context, configstore.VaultEntry, map[string]bool) error
	DeleteVaultEntry(context.Context, string) (bool, error)
}

type Verifier interface {
	Verify(context.Context, configstore.AuthRecord) error
	Login(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error)
}

type AccountRateSyncScheduler interface {
	EnqueueHostAccountRateSync(context.Context, string, string) (string, error)
	EnqueueHostAccountBaseURLSync(context.Context, string, string) (string, error)
}

type Service struct {
	business  Business
	private   PrivateStore
	verifier  Verifier
	scheduler AccountRateSyncScheduler
}

type classificationStore interface {
	UpdateUpstreamClassification(context.Context, string, string, string) error
}

type Input struct {
	Host           string
	Name           *string
	BaseURL        string
	AccountBaseURL string
	UpstreamType   string
	AuthMode       string
	RechargeRate   string
	AccessToken    *string
	RefreshToken   *string
	AdminKey       *string
	UserID         *string
	Headers        map[string]string
	Cookies        map[string]string
	Username       *string
	Password       *string
	SaveToVault    bool
	Entry          *string
	Present        map[string]bool
}

type Configuration struct {
	UpstreamID        string                   `json:"upstream_id"`
	Host              string                   `json:"host"`
	Name              string                   `json:"name"`
	BaseURL           string                   `json:"base_url"`
	AccountBaseURL    string                   `json:"account_base_url"`
	UpstreamType      string                   `json:"upstream_type"`
	AuthMode          string                   `json:"auth_mode"`
	RechargeRate      string                   `json:"recharge_rate"`
	RawBalance        *string                  `json:"raw_balance"`
	Balance           *string                  `json:"balance"`
	HasAccessToken    bool                     `json:"has_access_token"`
	HasRefreshToken   bool                     `json:"has_refresh_token"`
	HasAdminKey       bool                     `json:"has_admin_key"`
	HasUserID         bool                     `json:"has_user_id"`
	Headers           map[string]string        `json:"headers"`
	HeaderNames       []string                 `json:"header_names"`
	CookieNames       []string                 `json:"cookie_names"`
	Groups            []business.UpstreamGroup `json:"groups"`
	RateSyncTaskID    *string                  `json:"rate_sync_task_id,omitempty"`
	RateSyncError     *string                  `json:"rate_sync_error,omitempty"`
	BaseURLSyncTaskID *string                  `json:"base_url_sync_task_id,omitempty"`
	BaseURLSyncError  *string                  `json:"base_url_sync_error,omitempty"`
}

func New(businessStore Business, privateStore PrivateStore, verifier Verifier, schedulers ...AccountRateSyncScheduler) *Service {
	service := &Service{business: businessStore, private: privateStore, verifier: verifier}
	if len(schedulers) > 0 {
		service.scheduler = schedulers[0]
	}
	return service
}

func (s *Service) Get(ctx context.Context, host string) (Configuration, error) {
	host = configstore.CanonicalHost(host)
	if host == "" {
		return Configuration{}, errors.New("上游 Host 不能为空")
	}
	summary, err := s.business.Upstreams(ctx)
	if err != nil {
		return Configuration{}, err
	}
	var public *business.UpstreamHost
	for index := range summary.Hosts {
		if summary.Hosts[index].Host == host {
			item := summary.Hosts[index]
			public = &item
			break
		}
	}
	if public == nil {
		return Configuration{}, ErrNotFound
	}
	groups, err := s.business.UpstreamGroups(ctx, host, true)
	if err != nil {
		return Configuration{}, err
	}
	record, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return Configuration{}, err
	}
	result := Configuration{
		UpstreamID: public.UpstreamID, Host: host, Name: public.Name, BaseURL: public.BaseURL, AccountBaseURL: public.AccountBaseURL, UpstreamType: public.UpstreamType,
		AuthMode: "custom_headers", RechargeRate: public.RechargeRate, RawBalance: public.RawBalance,
		Balance: public.Balance, Headers: map[string]string{}, HeaderNames: []string{}, CookieNames: []string{}, Groups: groups,
	}
	if record != nil {
		result.BaseURL, result.UpstreamType, result.AuthMode = record.BaseURL, record.UpstreamType, record.AuthMode
		result.HasAccessToken, result.HasRefreshToken = nonblank(record.AccessToken), nonblank(record.RefreshToken)
		result.HasAdminKey, result.HasUserID = nonblank(record.AdminKey), nonblank(record.UserID)
		result.Headers = cloneMap(record.Headers)
		result.HeaderNames, result.CookieNames = sortedKeys(record.Headers), sortedKeys(record.Cookies)
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, input Input, actor string) (Configuration, error) {
	host := configstore.CanonicalHost(input.Host)
	if host == "" || input.Name == nil || strings.TrimSpace(*input.Name) == "" {
		return Configuration{}, inputError(errors.New("上游 Host 和名称不能为空"))
	}
	resources := []string{mutationguard.UpstreamCatalog()}
	if vault := vaultMutationResource(host, input, true); vault != "" {
		resources = append(resources, vault)
	}
	guarded, release, err := s.acquireHostMutation(ctx, host, resources...)
	if err != nil {
		return Configuration{}, err
	}
	defer release()
	ctx = guarded
	exists, err := s.business.UpstreamExists(ctx, host)
	if err != nil {
		return Configuration{}, err
	}
	if exists {
		return Configuration{}, ErrConflict
	}
	existing, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return Configuration{}, err
	}
	if existing != nil {
		return Configuration{}, ErrConflict
	}
	record, vaultChange, err := s.prepareVerified(ctx, host, input, nil, true)
	if err != nil {
		return Configuration{}, inputError(err)
	}
	if err := normalizeAccountBaseURL(&input, record.BaseURL); err != nil {
		return Configuration{}, inputError(err)
	}
	if _, err := s.commitPrivateAndPublic(ctx, record, input, vaultChange, true); err != nil {
		return Configuration{}, err
	}
	release()
	eventCtx, cancelEvent := detachedOperationContext(ctx)
	if _, err := s.business.RecordRuntimeEvent(eventCtx, "upstream.created", "succeeded", "上游已添加并完成鉴权："+host, map[string]any{
		"actor": actorOrConsole(actor), "host": host, "upstream_type": record.UpstreamType, "auth_mode": record.AuthMode,
	}); err != nil {
		slog.Error("上游创建事件保存失败", "host", host, "error", err)
	}
	cancelEvent()
	readCtx, cancelRead := detachedOperationContext(ctx)
	defer cancelRead()
	return s.Get(readCtx, host)
}

func (s *Service) Update(ctx context.Context, host string, input Input, actor string) (Configuration, error) {
	host = configstore.CanonicalHost(host)
	if host == "" {
		return Configuration{}, inputError(errors.New("上游 Host 不能为空"))
	}
	resources := []string{}
	if vault := vaultMutationResource(host, input, false); vault != "" {
		resources = append(resources, vault)
	}
	guarded, release, err := s.acquireHostMutation(ctx, host, resources...)
	if err != nil {
		return Configuration{}, err
	}
	defer release()
	ctx = guarded
	previous, err := s.Get(ctx, host)
	if err != nil {
		return Configuration{}, err
	}
	exists, err := s.business.UpstreamExists(ctx, host)
	if err != nil {
		return Configuration{}, err
	}
	if !exists {
		return Configuration{}, ErrNotFound
	}
	existing, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return Configuration{}, err
	}
	record, vaultChange, err := s.prepareVerified(ctx, host, input, existing, false)
	if err != nil {
		return Configuration{}, inputError(err)
	}
	if err := normalizeAccountBaseURL(&input, record.BaseURL); err != nil {
		return Configuration{}, inputError(err)
	}
	writeResult, err := s.commitPrivateAndPublic(ctx, record, input, vaultChange, false)
	if err != nil {
		return Configuration{}, err
	}
	release()
	var rateSyncTaskID, rateSyncError, baseURLSyncTaskID, baseURLSyncError *string
	syncHost := configstore.CanonicalHost(writeResult.PrimaryHost)
	if syncHost == "" {
		syncHost = host
	}
	if s.scheduler != nil && decimalChanged(previous.RechargeRate, writeResult.RechargeRate) {
		scheduleCtx, cancelSchedule := detachedOperationContext(ctx)
		taskID, scheduleErr := s.scheduler.EnqueueHostAccountRateSync(scheduleCtx, syncHost, actorOrConsole(actor))
		cancelSchedule()
		if scheduleErr != nil {
			message := fmt.Sprintf("稳定上游 %s 的账号成本同步排队失败：%v", syncHost, scheduleErr)
			rateSyncError = &message
		} else if taskID != "" {
			rateSyncTaskID = &taskID
		}
	}
	if s.scheduler != nil && !sameBaseURL(previous.AccountBaseURL, input.AccountBaseURL) {
		scheduleCtx, cancelSchedule := detachedOperationContext(ctx)
		taskID, scheduleErr := s.scheduler.EnqueueHostAccountBaseURLSync(scheduleCtx, syncHost, actorOrConsole(actor))
		cancelSchedule()
		if scheduleErr != nil {
			message := fmt.Sprintf("稳定上游 %s 的账号 Base URL 同步排队失败：%v", syncHost, scheduleErr)
			baseURLSyncError = &message
		} else if taskID != "" {
			baseURLSyncTaskID = &taskID
		}
	}
	eventCtx, cancelEvent := detachedOperationContext(ctx)
	if _, err := s.business.RecordRuntimeEvent(eventCtx, "upstream.configuration.updated", "succeeded", "上游配置已更新："+host, map[string]any{
		"actor": actorOrConsole(actor), "host": host, "upstream_type": record.UpstreamType,
		"auth_mode": record.AuthMode, "recharge_rate": input.RechargeRate,
	}); err != nil {
		slog.Error("上游配置更新事件保存失败", "host", host, "error", err)
	}
	cancelEvent()
	readCtx, cancelRead := detachedOperationContext(ctx)
	result, err := s.Get(readCtx, host)
	cancelRead()
	if err != nil {
		return Configuration{}, fmt.Errorf("上游配置已保存，但读取保存结果失败：%w", err)
	}
	result.RateSyncTaskID = rateSyncTaskID
	result.RateSyncError = rateSyncError
	result.BaseURLSyncTaskID = baseURLSyncTaskID
	result.BaseURLSyncError = baseURLSyncError
	return result, nil
}

func detachedOperationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), postCommitOperationTimeout)
}

func decimalChanged(before, after string) bool {
	left, leftOK := new(big.Rat).SetString(strings.TrimSpace(before))
	right, rightOK := new(big.Rat).SetString(strings.TrimSpace(after))
	if leftOK && rightOK {
		return left.Cmp(right) != 0
	}
	return strings.TrimSpace(before) != strings.TrimSpace(after)
}

func sameBaseURL(left, right string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(left), "/"), strings.TrimRight(strings.TrimSpace(right), "/"))
}

func (s *Service) ConfigureAuthRecord(ctx context.Context, input Input) (string, error) {
	host := configstore.CanonicalHost(input.Host)
	if host == "" {
		return "", inputError(errors.New("上游 Host 不能为空"))
	}
	resources := []string{}
	if vault := vaultMutationResource(host, input, true); vault != "" {
		resources = append(resources, vault)
	}
	guarded, release, err := s.acquireHostMutation(ctx, host, resources...)
	if err != nil {
		return "", err
	}
	defer release()
	ctx = guarded
	exists, err := s.business.UpstreamExists(ctx, host)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", ErrNotFound
	}
	existing, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return "", err
	}
	record, change, err := s.prepareVerified(ctx, host, input, existing, existing == nil)
	if err != nil {
		return "", inputError(err)
	}
	if change != nil {
		if err := s.private.SaveVaultEntry(ctx, *change.entry, allVaultFields()); err != nil {
			return "", err
		}
	}
	if err := s.private.SaveAuthRecord(ctx, record, allAuthFields()); err != nil {
		rollbackCtx, cancelRollback := detachedOperationContext(ctx)
		defer cancelRollback()
		return "", errors.Join(err, rollbackFailure("密码箱", s.restoreVault(rollbackCtx, change)))
	}
	return host, nil
}

// CommitRecoveredAuth publishes a publicly fingerprinted platform correction
// only after the recovered credentials have passed an authenticated readback.
func (s *Service) CommitRecoveredAuth(ctx context.Context, record configstore.AuthRecord) error {
	record.Host = configstore.CanonicalHost(record.Host)
	if record.Host == "" {
		return inputError(errors.New("上游 Host 不能为空"))
	}
	guarded, release, err := s.acquireHostMutation(ctx, record.Host)
	if err != nil {
		return err
	}
	defer release()
	ctx = guarded
	classification, ok := s.business.(classificationStore)
	if !ok {
		return errors.New("业务存储不支持修复上游平台类型")
	}
	exists, err := s.business.UpstreamExists(ctx, record.Host)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	previous, err := s.private.AuthRecord(ctx, record.Host)
	if err != nil {
		return err
	}
	if err := s.private.SaveAuthRecord(ctx, record, allAuthFields()); err != nil {
		return err
	}
	if err := classification.UpdateUpstreamClassification(ctx, record.Host, record.UpstreamType, record.AuthMode); err != nil {
		rollbackCtx, cancelRollback := detachedOperationContext(ctx)
		defer cancelRollback()
		if previous == nil {
			_, rollbackErr := s.private.DeleteAuthRecord(rollbackCtx, record.Host)
			return errors.Join(err, rollbackFailure("鉴权记录", rollbackErr))
		}
		return errors.Join(err, rollbackFailure("鉴权记录", s.private.SaveAuthRecord(rollbackCtx, *previous, allAuthFields())))
	}
	return nil
}

func (s *Service) acquireHostMutation(ctx context.Context, host string, additionalResources ...string) (context.Context, func(), error) {
	host = configstore.CanonicalHost(host)
	resources := append([]string{}, additionalResources...)
	resources = append(resources, mutationguard.Upstream(host))
	guarded, release, err := mutationguard.Acquire(ctx, s.business, resources...)
	if err != nil {
		return nil, nil, err
	}
	released := false
	return guarded, func() {
		if released {
			return
		}
		released = true
		if err := release(); err != nil {
			slog.Error("上游配置租约释放失败", "host", host, "error", err)
		}
	}, nil
}

func vaultMutationResource(host string, input Input, creating bool) string {
	if !input.SaveToVault || !isManualLogin(strings.TrimSpace(input.AuthMode)) {
		return ""
	}
	if !creating && !input.Present["username"] && !input.Present["password"] {
		return ""
	}
	entry := strings.TrimSpace(host)
	if input.Entry != nil && strings.TrimSpace(*input.Entry) != "" {
		entry = strings.TrimSpace(*input.Entry)
	}
	return mutationguard.Vault(entry)
}

type vaultChange struct {
	entry    *configstore.VaultEntry
	previous *configstore.VaultEntry
}

func (s *Service) prepareVerified(ctx context.Context, host string, input Input, existing *configstore.AuthRecord, creating bool) (configstore.AuthRecord, *vaultChange, error) {
	baseURL, err := configstore.ValidateBaseURL(input.BaseURL)
	if err != nil {
		return configstore.AuthRecord{}, nil, err
	}
	record := configstore.AuthRecord{Host: host, BaseURL: baseURL, UpstreamType: strings.ToLower(strings.TrimSpace(input.UpstreamType)), AuthMode: strings.TrimSpace(input.AuthMode), Headers: map[string]string{}, Cookies: map[string]string{}}
	if existing != nil {
		record = cloneRecord(*existing)
		record.Host, record.BaseURL, record.UpstreamType, record.AuthMode = host, baseURL, strings.ToLower(strings.TrimSpace(input.UpstreamType)), strings.TrimSpace(input.AuthMode)
	}
	applyInput(&record, input)
	mode := record.AuthMode
	var credential *configstore.VaultEntry
	var change *vaultChange
	if isManualLogin(mode) && (input.Present["username"] || input.Present["password"] || creating) {
		if !nonblank(input.Username) || !nonblank(input.Password) {
			return configstore.AuthRecord{}, nil, errors.New("自定义账号密码登录必须填写用户名和密码")
		}
		entryName := host
		if input.Entry != nil && strings.TrimSpace(*input.Entry) != "" {
			entryName = strings.TrimSpace(*input.Entry)
		}
		credential = &configstore.VaultEntry{Entry: entryName, Username: input.Username, Password: input.Password, Hosts: []string{host}, Headers: cloneMap(record.Headers)}
		if input.SaveToVault {
			previous, err := s.private.VaultEntry(ctx, entryName)
			if err != nil {
				return configstore.AuthRecord{}, nil, err
			}
			change = &vaultChange{entry: credential, previous: previous}
		}
	} else if isVaultLogin(mode) && (input.Present["entry"] || creating) {
		if input.Entry == nil || strings.TrimSpace(*input.Entry) == "" {
			return configstore.AuthRecord{}, nil, errors.New("请选择一个密码箱项")
		}
		credential, err = s.private.VaultEntry(ctx, strings.TrimSpace(*input.Entry))
		if err != nil {
			return configstore.AuthRecord{}, nil, err
		}
		if credential == nil {
			return configstore.AuthRecord{}, nil, errors.New("所选密码箱项不存在")
		}
		if input.Present["headers"] {
			candidate := *credential
			candidate.Headers = cloneMap(credential.Headers)
			for key, value := range record.Headers {
				candidate.Headers[key] = value
			}
			credential = &candidate
		}
	}
	if credential != nil {
		record, err = s.verifier.Login(ctx, record, *credential)
	} else {
		err = upstreamauth.ValidateRecord(record)
		if err == nil {
			err = s.verifier.Verify(ctx, record)
		}
	}
	if err != nil {
		return configstore.AuthRecord{}, nil, fmt.Errorf("鉴权配置未通过上游复核：%w", err)
	}
	return record, change, nil
}

func (s *Service) commitPrivateAndPublic(
	ctx context.Context,
	record configstore.AuthRecord,
	input Input,
	change *vaultChange,
	creating bool,
) (business.UpstreamConfigurationWriteResult, error) {
	var oldAuth *configstore.AuthRecord
	if !creating {
		var err error
		oldAuth, err = s.private.AuthRecord(ctx, record.Host)
		if err != nil {
			return business.UpstreamConfigurationWriteResult{}, err
		}
	}
	if change != nil {
		if err := s.private.SaveVaultEntry(ctx, *change.entry, allVaultFields()); err != nil {
			return business.UpstreamConfigurationWriteResult{}, err
		}
	}
	if err := s.private.SaveAuthRecord(ctx, record, allAuthFields()); err != nil {
		rollbackCtx, cancelRollback := detachedOperationContext(ctx)
		defer cancelRollback()
		return business.UpstreamConfigurationWriteResult{}, errors.Join(err, rollbackFailure("密码箱", s.restoreVault(rollbackCtx, change)))
	}
	write := business.UpstreamConfigurationWrite{
		Host: record.Host, Name: input.Name, BaseURL: record.BaseURL, AccountBaseURL: input.AccountBaseURL, UpstreamType: record.UpstreamType,
		AuthMode: record.AuthMode, RechargeRate: input.RechargeRate,
	}
	var result business.UpstreamConfigurationWriteResult
	var err error
	if creating {
		result, err = s.business.CreateUpstreamConfiguration(ctx, write)
	} else {
		result, err = s.business.UpdateUpstreamConfiguration(ctx, write)
	}
	if err == nil {
		return result, nil
	}
	rollbackCtx, cancelRollback := detachedOperationContext(ctx)
	defer cancelRollback()
	rollbackErrors := []error{err}
	if oldAuth == nil {
		_, rollbackErr := s.private.DeleteAuthRecord(rollbackCtx, record.Host)
		rollbackErrors = append(rollbackErrors, rollbackFailure("新鉴权记录", rollbackErr))
	} else {
		rollbackErrors = append(rollbackErrors, rollbackFailure("原鉴权记录", s.private.SaveAuthRecord(rollbackCtx, *oldAuth, allAuthFields())))
	}
	rollbackErrors = append(rollbackErrors, rollbackFailure("密码箱", s.restoreVault(rollbackCtx, change)))
	return business.UpstreamConfigurationWriteResult{}, errors.Join(rollbackErrors...)
}

func normalizeAccountBaseURL(input *Input, fallback string) error {
	value := strings.TrimSpace(input.AccountBaseURL)
	if value == "" {
		value = fallback
	}
	normalized, err := configstore.ValidateBaseURL(value)
	if err != nil {
		return fmt.Errorf("账号 Base URL 无效：%w", err)
	}
	input.AccountBaseURL = normalized
	return nil
}

func (s *Service) restoreVault(ctx context.Context, change *vaultChange) error {
	if change == nil || change.entry == nil {
		return nil
	}
	if change.previous == nil {
		_, err := s.private.DeleteVaultEntry(ctx, change.entry.Entry)
		return err
	}
	return s.private.SaveVaultEntry(ctx, *change.previous, allVaultFields())
}

func rollbackFailure(target string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s补偿回滚失败：%w", target, err)
}

func applyInput(record *configstore.AuthRecord, input Input) {
	if input.Present["access_token"] {
		record.AccessToken = input.AccessToken
	}
	if input.Present["refresh_token"] {
		record.RefreshToken = input.RefreshToken
	}
	if input.Present["admin_key"] {
		record.AdminKey = input.AdminKey
	}
	if input.Present["user_id"] {
		record.UserID = input.UserID
	}
	if input.Present["headers"] {
		record.Headers = cloneMap(input.Headers)
	}
	if input.Present["cookies"] {
		record.Cookies = cloneMap(input.Cookies)
	}
}

func allAuthFields() map[string]bool {
	return map[string]bool{"base_url": true, "upstream_type": true, "auth_mode": true, "access_token": true, "refresh_token": true, "admin_key": true, "user_id": true, "headers": true, "cookies": true}
}
func allVaultFields() map[string]bool {
	return map[string]bool{"username": true, "password": true, "hosts": true, "headers": true}
}
func isManualLogin(mode string) bool {
	return mode == "sub2api_manual_login" || mode == "newapi_manual_login"
}
func isVaultLogin(mode string) bool {
	return mode == "sub2api_user_login" || mode == "newapi_user_login"
}
func nonblank(value *string) bool { return value != nil && strings.TrimSpace(*value) != "" }
func actorOrConsole(value string) string {
	if strings.TrimSpace(value) == "" {
		return "console"
	}
	return strings.TrimSpace(value)
}
func cloneRecord(value configstore.AuthRecord) configstore.AuthRecord {
	result := value
	result.Headers, result.Cookies = cloneMap(value.Headers), cloneMap(value.Cookies)
	return result
}
func cloneMap(value map[string]string) map[string]string {
	result := map[string]string{}
	for key, item := range value {
		result[key] = item
	}
	return result
}
func sortedKeys(value map[string]string) []string {
	result := make([]string, 0, len(value))
	for key := range value {
		result = append(result, key)
	}
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j] < result[j-1]; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	return result
}

func inputError(err error) error {
	if err == nil {
		return nil
	}
	var existing *InputError
	if errors.As(err, &existing) {
		return err
	}
	return &InputError{Err: err}
}
