package newapimanagement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

const maximumResponseBytes = 4 << 20

const defaultSub2APIPricingURL = "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json"

var remoteLongContextPricePattern = regexp.MustCompile(`^(input|output)_cost_per_token_above_(\d+)k_tokens$`)

type PrivateStore interface {
	NewAPIPlatforms(context.Context) ([]configstore.NewAPIPlatformSummary, error)
	NewAPIPlatform(context.Context, string) (*configstore.NewAPIPlatform, error)
	SaveNewAPIPlatform(context.Context, configstore.NewAPIPlatform) (configstore.NewAPIPlatformSummary, error)
	DeleteNewAPIPlatform(context.Context, string) (bool, error)
	TargetSettings(context.Context) (configstore.TargetSettings, error)
	VaultEntry(context.Context, string) (*configstore.VaultEntry, error)
	UpstreamKeySecret(context.Context, string, string, string) (*configstore.UpstreamKeySecret, error)
	SaveUpstreamKeySecret(context.Context, configstore.UpstreamKeySecret) error
}

type upstreamAuthReader interface {
	AuthRecordIndex(context.Context) ([]configstore.AuthRecordSummary, error)
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
}

type Repository interface {
	NewAPILocalGroups(context.Context) ([]business.NewAPILocalGroup, error)
	NewAPIGroupBindings(context.Context, string) ([]business.NewAPIGroupBinding, error)
	ReplaceNewAPIGroupBindings(context.Context, string, []business.NewAPIGroupBinding) error
	DeleteNewAPIGroupBindings(context.Context, string) error
}

type KeyManager interface {
	CreateKeyWithVerification(context.Context, configstore.AuthRecord, string, string, bool) (upstreamsync.CreatedKey, error)
	ReconcileCreatedKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, bool, error)
}

type Authenticator interface {
	Login(context.Context, configstore.AuthRecord, configstore.VaultEntry) (configstore.AuthRecord, error)
}

type Service struct {
	private      PrivateStore
	repository   Repository
	client       *http.Client
	keys         KeyManager
	auth         Authenticator
	managementMu sync.Mutex
	pricingMu    sync.Mutex
	pricingAt    time.Time
	pricing      []Sub2APIModelPrice
	pricingRaw   RemotePricingSource
}

type Workspace struct {
	Platforms      []configstore.NewAPIPlatformSummary `json:"platforms"`
	LocalGroups    []business.NewAPILocalGroup         `json:"local_groups"`
	Bindings       []business.NewAPIGroupBinding       `json:"bindings"`
	Sub2APIBaseURL string                              `json:"sub2api_base_url"`
}

type PlatformInput struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	AdminKey string `json:"admin_key"`
	UserID   string `json:"user_id"`
}

type RemoteGroup struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Ratio *string `json:"ratio"`
}

type ModelPrice struct {
	Model                string `json:"model"`
	ModelPrice           string `json:"model_price,omitempty"`
	InputRatio           string `json:"input_ratio"`
	CompletionRatio      string `json:"completion_ratio"`
	InputPrice           string `json:"input_price,omitempty"`
	CompletionPrice      string `json:"completion_price,omitempty"`
	CacheCreatePrice     string `json:"cache_create_price,omitempty"`
	CacheReadPrice       string `json:"cache_read_price,omitempty"`
	BillingMode          string `json:"billing_mode,omitempty"`
	BillingExpr          string `json:"billing_expr,omitempty"`
	CacheRatio           string `json:"cache_ratio,omitempty"`
	CreateCacheRatio     string `json:"create_cache_ratio,omitempty"`
	CreateCache1hRatio   string `json:"create_cache_1h_ratio,omitempty"`
	ImageRatio           string `json:"image_ratio,omitempty"`
	AudioRatio           string `json:"audio_ratio,omitempty"`
	AudioCompletionRatio string `json:"audio_completion_ratio,omitempty"`
}

type ToolPrice struct {
	Tool  string `json:"tool"`
	Price string `json:"price"`
}

// Sub2APIModelPrice is one entry from Sub2API's loaded billing catalog and its
// corresponding New API ratios. InputPrice/OutputPrice are USD per token.
type Sub2APIModelPrice struct {
	Model                         string `json:"model"`
	InputPrice                    string `json:"input_price"`
	OutputPrice                   string `json:"output_price"`
	ImageInputPrice               string `json:"image_input_price,omitempty"`
	ImageOutputPrice              string `json:"image_output_price,omitempty"`
	Provider                      string `json:"provider,omitempty"`
	Mode                          string `json:"mode,omitempty"`
	CacheWritePrice               string `json:"cache_write_price,omitempty"`
	CacheWrite1hPrice             string `json:"cache_write_1h_price,omitempty"`
	CacheReadPrice                string `json:"cache_read_price,omitempty"`
	ModelRatio                    string `json:"model_ratio"`
	CompletionRatio               string `json:"completion_ratio"`
	CacheRatio                    string `json:"cache_ratio,omitempty"`
	CreateCacheRatio              string `json:"create_cache_ratio,omitempty"`
	CreateCache1hRatio            string `json:"create_cache_1h_ratio,omitempty"`
	ImageRatio                    string `json:"image_ratio,omitempty"`
	LongContextThreshold          int    `json:"long_context_threshold,omitempty"`
	LongContextThresholdInclusive bool   `json:"long_context_threshold_inclusive,omitempty"`
	LongContextInputPrice         string `json:"long_context_input_price,omitempty"`
	LongContextOutputPrice        string `json:"long_context_output_price,omitempty"`
	LongContextCacheWritePrice    string `json:"long_context_cache_write_price,omitempty"`
	LongContextCacheWrite1hPrice  string `json:"long_context_cache_write_1h_price,omitempty"`
	LongContextCacheReadPrice     string `json:"long_context_cache_read_price,omitempty"`
}

type RemotePricingSource struct {
	SourceURL string `json:"source_url"`
	Content   string `json:"content"`
	FetchedAt string `json:"fetched_at"`
	SizeBytes int    `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type RemoteSnapshot struct {
	Groups      []RemoteGroup `json:"groups"`
	Models      []ModelPrice  `json:"models"`
	UnsetModels []ModelPrice  `json:"unset_models"`
	ToolPrices  []ToolPrice   `json:"tool_prices"`
	References  []ModelPrice  `json:"references"`
	// NewAPIModels and References are retained for response compatibility. Both
	// are derived from the selected New API site's managed options; neither is
	// populated from the public model-plaza endpoint.
	NewAPIModels []ModelPrice `json:"newapi_models"`
	// Sub2APIModels is the locally maintained Sub2API model-price catalog.
	Sub2APIModels  []Sub2APIModelPrice    `json:"sub2api_models"`
	UpstreamPrices []UpstreamPriceCatalog `json:"upstream_prices,omitempty"`
	Differences    []PriceDifference      `json:"differences"`
	FetchedAt      string                 `json:"fetched_at"`
}

type UpstreamPriceCatalog struct {
	Host         string       `json:"host"`
	Name         string       `json:"name"`
	UpstreamType string       `json:"upstream_type"`
	Models       []ModelPrice `json:"models"`
}

type PriceDifference struct {
	Model      string      `json:"model"`
	Kind       string      `json:"kind"`
	Configured *ModelPrice `json:"configured"`
	Reference  *ModelPrice `json:"reference"`
}

type GroupBindingInput struct {
	NewAPIGroupID   string `json:"newapi_group_id"`
	NewAPIGroupName string `json:"newapi_group_name"`
	Sub2APIGroupID  string `json:"sub2api_group_id"`
	SyncRatio       bool   `json:"sync_ratio"`
}

type ChannelInput struct {
	Sub2APIGroupID string   `json:"sub2api_group_id"`
	KeyID          string   `json:"key_id"`
	BaseURL        string   `json:"base_url"`
	Models         []string `json:"models"`
	NewAPIGroups   []string `json:"newapi_groups"`
}

type ChannelModelsInput struct {
	Sub2APIGroupID string `json:"sub2api_group_id"`
	KeyID          string `json:"key_id"`
	BaseURL        string `json:"base_url"`
}

type ChannelKeyInput struct {
	Sub2APIGroupID   string `json:"sub2api_group_id"`
	CredentialSource string `json:"credential_source"`
	VaultEntry       string `json:"vault_entry"`
	Username         string `json:"username"`
	Password         string `json:"password"`
}

type ChannelKey struct {
	KeyID     string            `json:"key_id"`
	Name      string            `json:"name"`
	GroupID   string            `json:"group_id"`
	Endpoints []ChannelEndpoint `json:"endpoints"`
}

type ChannelEndpoint struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Default bool   `json:"default"`
}

type ModelPriceInput struct {
	Model                string `json:"model"`
	ModelPrice           string `json:"model_price,omitempty"`
	InputRatio           string `json:"input_ratio"`
	CompletionRatio      string `json:"completion_ratio"`
	BillingMode          string `json:"billing_mode,omitempty"`
	BillingExpr          string `json:"billing_expr,omitempty"`
	CacheRatio           string `json:"cache_ratio,omitempty"`
	CreateCacheRatio     string `json:"create_cache_ratio,omitempty"`
	ImageRatio           string `json:"image_ratio,omitempty"`
	AudioRatio           string `json:"audio_ratio,omitempty"`
	AudioCompletionRatio string `json:"audio_completion_ratio,omitempty"`
}

type ErrorKind string

const (
	ErrorValidation  ErrorKind = "validation"
	ErrorNotFound    ErrorKind = "not_found"
	ErrorConflict    ErrorKind = "conflict"
	ErrorUnavailable ErrorKind = "unavailable"
	ErrorUpstream    ErrorKind = "upstream"
)

type ServiceError struct {
	Kind ErrorKind
	Err  error
}

func (e *ServiceError) Error() string { return e.Err.Error() }

func (e *ServiceError) Unwrap() error { return e.Err }

func KindOf(err error) ErrorKind {
	var serviceError *ServiceError
	if errors.As(err, &serviceError) {
		return serviceError.Kind
	}
	return ""
}

func serviceError(kind ErrorKind, message string) error {
	return &ServiceError{Kind: kind, Err: errors.New(message)}
}

func wrapServiceError(kind ErrorKind, err error) error {
	if err == nil || KindOf(err) != "" {
		return err
	}
	return &ServiceError{Kind: kind, Err: err}
}

func New(private PrivateStore, repository Repository, client *http.Client, keys KeyManager, auth Authenticator) *Service {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if clientCopy.Timeout <= 0 {
		clientCopy.Timeout = 20 * time.Second
	}
	return &Service{private: private, repository: repository, client: &clientCopy, keys: keys, auth: auth}
}

func (s *Service) Workspace(ctx context.Context, platformID string) (Workspace, error) {
	platforms, err := s.private.NewAPIPlatforms(ctx)
	if err != nil {
		return Workspace{}, err
	}
	groups, err := s.repository.NewAPILocalGroups(ctx)
	if err != nil {
		return Workspace{}, err
	}
	bindings := []business.NewAPIGroupBinding{}
	if strings.TrimSpace(platformID) != "" {
		bindings, err = s.repository.NewAPIGroupBindings(ctx, platformID)
		if err != nil {
			return Workspace{}, err
		}
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{Platforms: platforms, LocalGroups: groups, Bindings: bindings, Sub2APIBaseURL: target.BaseURL}, nil
}

func (s *Service) SavePlatform(ctx context.Context, input PlatformInput) (configstore.NewAPIPlatformSummary, error) {
	platforms, err := s.private.NewAPIPlatforms(ctx)
	if err != nil {
		return configstore.NewAPIPlatformSummary{}, err
	}
	if len(platforms) > 0 {
		if strings.TrimSpace(input.ID) == "" {
			input.ID = platforms[0].ID
		} else if strings.TrimSpace(input.ID) != platforms[0].ID {
			return configstore.NewAPIPlatformSummary{}, serviceError(ErrorConflict, "New API 只允许配置一个主平台")
		}
	}
	item := configstore.NewAPIPlatform{ID: input.ID, Name: input.Name, BaseURL: input.BaseURL, AdminKey: input.AdminKey, UserID: input.UserID}
	if strings.TrimSpace(item.AdminKey) == "" && strings.TrimSpace(item.ID) != "" {
		current, err := s.private.NewAPIPlatform(ctx, item.ID)
		if err != nil {
			return configstore.NewAPIPlatformSummary{}, err
		}
		if current != nil && sameOriginBaseURL(current.BaseURL, item.BaseURL) {
			item.AdminKey = current.AdminKey
			if strings.TrimSpace(item.UserID) == "" {
				item.UserID = current.UserID
			}
		}
	}
	if _, err := configstore.ValidateBaseURL(item.BaseURL); err != nil {
		return configstore.NewAPIPlatformSummary{}, serviceError(ErrorValidation, "New API 平台地址无效")
	}
	if err := s.testConnection(ctx, item); err != nil {
		return configstore.NewAPIPlatformSummary{}, err
	}
	return s.private.SaveNewAPIPlatform(ctx, item)
}

func (s *Service) DeletePlatform(ctx context.Context, platformID string) (bool, error) {
	platformID = strings.TrimSpace(platformID)
	if platformID == "" {
		return false, serviceError(ErrorValidation, "New API 平台 ID 不能为空")
	}
	bindings, err := s.repository.NewAPIGroupBindings(ctx, platformID)
	if err != nil {
		return false, err
	}
	// Remove dependent local state first so a failed platform delete cannot
	// leave bindings pointing at a non-existent platform.
	if err := s.repository.DeleteNewAPIGroupBindings(ctx, platformID); err != nil {
		return false, fmt.Errorf("New API 分组绑定清理失败：%w", err)
	}
	deleted, deleteErr := s.private.DeleteNewAPIPlatform(ctx, platformID)
	if deleteErr != nil || !deleted {
		rollbackErr := s.repository.ReplaceNewAPIGroupBindings(ctx, platformID, bindings)
		if deleteErr != nil {
			return false, errors.Join(deleteErr, rollbackErr)
		}
		return false, rollbackErr
	}
	return true, nil
}

func (s *Service) Refresh(ctx context.Context, platformID string) (RemoteSnapshot, error) {
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return RemoteSnapshot{}, err
	}
	options, err := s.readOptions(ctx, *platform)
	if err != nil {
		return RemoteSnapshot{}, err
	}
	groups, err := decodeGroups(options["GroupRatio"])
	if err != nil {
		return RemoteSnapshot{}, err
	}
	models, err := decodeConfiguredModels(options)
	if err != nil {
		return RemoteSnapshot{}, err
	}
	configuredModels := models
	enabledModelNames := []string{}
	if payload, requestErr := s.request(ctx, *platform, http.MethodGet, "/api/channel/models_enabled", nil); requestErr == nil {
		enabledModelNames = decodeModelNames(payload)
	}
	unsetModels := findUnsetModelPrices(configuredModels, enabledModelNames)

	// Retain the public pricing catalog for the separate comparison view. It
	// must not affect the three model-pricing categories above.
	pricingCatalog := []ModelPrice{}
	if payload, requestErr := s.request(ctx, *platform, http.MethodGet, "/api/pricing", nil); requestErr == nil {
		pricingCatalog = decodePricingCatalog(payload)
	}
	references := mergeModelPriceRows(configuredModels, unsetModels)
	// Compare configured ratios against the remote pricing catalog. The
	// configured+unset projection is useful for display, but comparing it with
	// itself can never reveal a mismatch.
	differences := compareCatalogPrices(configuredModels, pricingCatalog)
	toolPrices := decodeToolPrices(options["tool_price_setting.prices"])
	return RemoteSnapshot{
		Groups: groups, Models: configuredModels, UnsetModels: unsetModels, ToolPrices: toolPrices,
		References: references, NewAPIModels: pricingCatalog,
		UpstreamPrices: s.readUpstreamPriceCatalogs(ctx), Differences: differences,
		FetchedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func sameOriginBaseURL(left, right string) bool {
	leftURL, leftErr := url.Parse(strings.TrimRight(strings.TrimSpace(left), "/"))
	rightURL, rightErr := url.Parse(strings.TrimRight(strings.TrimSpace(right), "/"))
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(leftURL.Scheme, rightURL.Scheme) && strings.EqualFold(leftURL.Host, rightURL.Host) && leftURL.Path == rightURL.Path
}

// ManagementModelPrices reads Sub2API's default remote billing catalog. The
// catalog is cached in the Console process so opening the preview does not
// repeatedly download the same JSON file.
func (s *Service) ManagementModelPrices(ctx context.Context, platformID string) ([]Sub2APIModelPrice, error) {
	if _, err := s.requirePlatform(ctx, platformID); err != nil {
		return nil, err
	}
	s.pricingMu.Lock()
	defer s.pricingMu.Unlock()
	if err := s.ensureRemotePricingCacheLocked(ctx); err != nil {
		return nil, err
	}
	return append([]Sub2APIModelPrice(nil), s.pricing...), nil
}

func (s *Service) RemoteModelPricingSource(ctx context.Context, platformID string) (RemotePricingSource, error) {
	if _, err := s.requirePlatform(ctx, platformID); err != nil {
		return RemotePricingSource{}, err
	}
	s.pricingMu.Lock()
	defer s.pricingMu.Unlock()
	if err := s.ensureRemotePricingCacheLocked(ctx); err != nil {
		return RemotePricingSource{}, err
	}
	return s.pricingRaw, nil
}

func (s *Service) ensureRemotePricingCacheLocked(ctx context.Context) error {
	if len(s.pricing) > 0 && s.pricingRaw.Content != "" && time.Since(s.pricingAt) < 24*time.Hour {
		return nil
	}
	prices, raw, err := s.fetchRemotePricingCatalog(ctx)
	if err != nil {
		return err
	}
	fetchedAt := time.Now().UTC()
	s.pricing = prices
	s.pricingAt = fetchedAt
	s.pricingRaw = buildRemotePricingSource(raw, fetchedAt)
	return nil
}

func (s *Service) readUpstreamPriceCatalogs(ctx context.Context) []UpstreamPriceCatalog {
	reader, ok := s.private.(upstreamAuthReader)
	if !ok {
		return nil
	}
	index, err := reader.AuthRecordIndex(ctx)
	if err != nil {
		return nil
	}
	jobs := make(chan configstore.AuthRecordSummary)
	items := make(chan UpstreamPriceCatalog, len(index))
	workers := min(4, len(index))
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for summary := range jobs {
				record, err := reader.AuthRecord(ctx, summary.Host)
				if err != nil || record == nil {
					continue
				}
				models, err := s.fetchUpstreamPriceCatalog(ctx, *record)
				if err == nil && len(models) > 0 {
					items <- UpstreamPriceCatalog{Host: summary.Host, Name: summary.Host, UpstreamType: record.UpstreamType, Models: models}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, summary := range index {
			select {
			case jobs <- summary:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wait.Wait(); close(items) }()
	result := make([]UpstreamPriceCatalog, 0, len(index))
	for item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Host < result[j].Host })
	return result
}

func (s *Service) fetchUpstreamPriceCatalog(ctx context.Context, record configstore.AuthRecord) ([]ModelPrice, error) {
	path := "/api/v1/model-plaza"
	if strings.EqualFold(record.UpstreamType, "newapi") || strings.EqualFold(record.UpstreamType, "oneapi") {
		path = "/api/pricing"
	}
	payload, err := s.requestUpstream(ctx, record, path)
	if err != nil {
		return nil, err
	}
	if path == "/api/v1/model-plaza" {
		return sub2APIModelRatios(decodeSub2APIModelPlaza(payload)), nil
	}
	return decodePricingCatalog(payload), nil
}

func (s *Service) requestUpstream(ctx context.Context, record configstore.AuthRecord, path string) (any, error) {
	base, err := configstore.ValidateBaseURL(record.BaseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for key, value := range record.Headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("Authorization") == "" {
		var token *string
		if record.AuthMode == "newapi_admin_key" {
			token = record.AdminKey
		} else {
			token = record.AccessToken
		}
		if token != nil && strings.TrimSpace(*token) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(*token))
		}
	}
	if record.UserID != nil {
		req.Header.Set("New-Api-User", strings.TrimSpace(*record.UserID))
	}
	for key, value := range record.Cookies {
		req.AddCookie(&http.Cookie{Name: key, Value: value})
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maximumResponseBytes {
		return nil, errors.New("上游价格响应超过大小限制")
	}
	var payload any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(dec); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("上游价格获取失败（HTTP %d）", resp.StatusCode)
	}
	if err := responseBusinessError(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Service) SaveBindings(ctx context.Context, platformID string, inputs []GroupBindingInput) ([]business.NewAPIGroupBinding, error) {
	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	localGroups, err := s.repository.NewAPILocalGroups(ctx)
	if err != nil {
		return nil, err
	}
	localByID := make(map[string]business.NewAPILocalGroup, len(localGroups))
	for _, group := range localGroups {
		localByID[group.ID] = group
	}
	previous, err := s.repository.NewAPIGroupBindings(ctx, platformID)
	if err != nil {
		return nil, err
	}
	items := make([]business.NewAPIGroupBinding, 0, len(inputs))
	for _, input := range inputs {
		if _, found := localByID[strings.TrimSpace(input.Sub2APIGroupID)]; !found {
			return nil, serviceError(ErrorValidation, "分组绑定包含不存在的 Sub2API 分组")
		}
		items = append(items, business.NewAPIGroupBinding{PlatformID: platformID, NewAPIGroupID: input.NewAPIGroupID, NewAPIGroupName: input.NewAPIGroupName, Sub2APIGroupID: input.Sub2APIGroupID, SyncRatio: input.SyncRatio})
	}
	if err := s.repository.ReplaceNewAPIGroupBindings(ctx, platformID, items); err != nil {
		return nil, err
	}
	if err := s.syncGroupRatios(ctx, *platform, items, localByID); err != nil {
		rollbackErr := s.repository.ReplaceNewAPIGroupBindings(ctx, platformID, previous)
		if rollbackErr != nil {
			return nil, errors.Join(err, fmt.Errorf("本地分组绑定回滚失败：%w", rollbackErr))
		}
		return nil, err
	}
	return s.repository.NewAPIGroupBindings(ctx, platformID)
}

func (s *Service) CreateChannel(ctx context.Context, platformID string, input ChannelInput) (map[string]any, error) {
	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	input.KeyID = strings.TrimSpace(input.KeyID)
	input.Sub2APIGroupID = strings.TrimSpace(input.Sub2APIGroupID)
	if input.KeyID == "" || input.Sub2APIGroupID == "" {
		return nil, serviceError(ErrorValidation, "Sub2API 分组和密钥 ID 不能为空")
	}
	models := normalizeModels(input.Models)
	if len(models) == 0 {
		return nil, serviceError(ErrorValidation, "渠道至少需要一个模型")
	}
	localGroups, err := s.repository.NewAPILocalGroups(ctx)
	if err != nil {
		return nil, err
	}
	groupName := ""
	for _, group := range localGroups {
		if group.ID == input.Sub2APIGroupID {
			groupName = group.Name
			break
		}
	}
	if groupName == "" {
		return nil, serviceError(ErrorValidation, "渠道目标不是已登记的 Sub2API 分组")
	}
	newAPIGroups, err := s.validateNewAPIGroups(ctx, *platform, input.NewAPIGroups)
	if err != nil {
		return nil, err
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	serviceKey, err := s.revealChannelKey(ctx, target, input.KeyID, input.Sub2APIGroupID)
	if err != nil {
		return nil, err
	}
	baseURL, err := configstore.ValidateBaseURL(input.BaseURL)
	if err != nil {
		return nil, serviceError(ErrorValidation, "渠道 API 地址无效")
	}
	channelName := stableOperationName(groupName, "channel", platformID, input.Sub2APIGroupID, input.KeyID, baseURL, strings.Join(models, ","), strings.Join(newAPIGroups, ","))
	if existing, found, err := s.findChannelByName(ctx, *platform, channelName); err != nil {
		return nil, fmt.Errorf("New API 渠道幂等对账失败：%w", err)
	} else if found {
		return publicChannelResult(existing, channelName), nil
	}
	body := map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"type": 59, "name": channelName, "base_url": baseURL, "key": serviceKey,
			"models": strings.Join(models, ","), "group": strings.Join(newAPIGroups, ","), "status": 1,
		},
	}
	payload, err := s.request(ctx, *platform, http.MethodPost, "/api/channel/", body)
	if err != nil {
		reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if existing, found, reconcileErr := s.findChannelByName(reconcileCtx, *platform, channelName); reconcileErr == nil && found {
			return publicChannelResult(existing, channelName), nil
		}
		return nil, fmt.Errorf("New API 渠道创建结果不确定（marker %s）：%w", channelName, err)
	}
	result, _ := payload.(map[string]any)
	return publicChannelResult(result, channelName), nil
}

func (s *Service) CreateChannelKey(ctx context.Context, platformID string, input ChannelKeyInput) (ChannelKey, error) {
	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	if _, err := s.requirePlatform(ctx, platformID); err != nil {
		return ChannelKey{}, err
	}
	input.Sub2APIGroupID = strings.TrimSpace(input.Sub2APIGroupID)
	group, err := s.localGroup(ctx, input.Sub2APIGroupID)
	if err != nil {
		return ChannelKey{}, err
	}
	if s.keys == nil {
		return ChannelKey{}, serviceError(ErrorUnavailable, "Sub2API 密钥管理服务尚未就绪")
	}
	if s.auth == nil {
		return ChannelKey{}, serviceError(ErrorUnavailable, "Sub2API 普通账号登录服务尚未就绪")
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return ChannelKey{}, err
	}
	credential, err := s.channelCredential(ctx, input)
	if err != nil {
		return ChannelKey{}, err
	}
	authenticated, err := s.auth.Login(ctx, sub2APIUserLoginRecord(target), credential)
	if err != nil {
		return ChannelKey{}, wrapServiceError(ErrorUpstream, fmt.Errorf("Sub2API 普通账号登录失败：%w", err))
	}
	credentialIdentity := strings.TrimSpace(input.VaultEntry)
	if strings.TrimSpace(input.CredentialSource) == "custom" {
		credentialIdentity = strings.TrimSpace(input.Username)
	}
	marker := stableOperationName(group.Name, "key", platformID, group.ID, strings.TrimSpace(input.CredentialSource), credentialIdentity)
	created, found, err := s.keys.ReconcileCreatedKey(ctx, authenticated, marker, group.ID)
	if err != nil {
		return ChannelKey{}, wrapServiceError(ErrorUpstream, fmt.Errorf("Sub2API 密钥幂等对账失败：%w", err))
	}
	if !found {
		created, err = s.keys.CreateKeyWithVerification(ctx, authenticated, marker, group.ID, true)
		if err != nil {
			return ChannelKey{}, wrapServiceError(ErrorUpstream, err)
		}
	}
	if strings.TrimSpace(created.KeyID) == "" || strings.TrimSpace(created.Secret) == "" || created.GroupID != group.ID {
		return ChannelKey{}, errors.New("Sub2API 密钥创建结果不可读")
	}
	if err := s.private.SaveUpstreamKeySecret(ctx, configstore.UpstreamKeySecret{
		Host: configstore.CanonicalHost(target.BaseURL), KeyID: created.KeyID,
		GroupID: created.GroupID, Secret: created.Secret,
	}); err != nil {
		return ChannelKey{}, fmt.Errorf("Sub2API 密钥已创建，但本地安全保存失败：%w", err)
	}
	return ChannelKey{
		KeyID: created.KeyID, Name: created.Name, GroupID: created.GroupID,
		Endpoints: s.channelEndpoints(ctx, target),
	}, nil
}

func (s *Service) channelCredential(ctx context.Context, input ChannelKeyInput) (configstore.VaultEntry, error) {
	switch strings.TrimSpace(input.CredentialSource) {
	case "vault":
		entryName := strings.TrimSpace(input.VaultEntry)
		if entryName == "" {
			return configstore.VaultEntry{}, serviceError(ErrorValidation, "请选择密码箱账号")
		}
		entry, err := s.private.VaultEntry(ctx, entryName)
		if err != nil {
			return configstore.VaultEntry{}, fmt.Errorf("密码箱账号读取失败：%w", err)
		}
		if entry == nil {
			return configstore.VaultEntry{}, serviceError(ErrorNotFound, "所选密码箱账号不存在")
		}
		if blankText(entry.Username) || blankText(entry.Password) {
			return configstore.VaultEntry{}, serviceError(ErrorValidation, "所选密码箱账号缺少完整账号或密码")
		}
		return *entry, nil
	case "custom":
		username := strings.TrimSpace(input.Username)
		password := input.Password
		if username == "" || strings.TrimSpace(password) == "" {
			return configstore.VaultEntry{}, serviceError(ErrorValidation, "请输入完整的 Sub2API 账号和密码")
		}
		return configstore.VaultEntry{Username: &username, Password: &password, Headers: map[string]string{}}, nil
	default:
		return configstore.VaultEntry{}, serviceError(ErrorValidation, "请选择密码箱账号或自定义账号密码")
	}
}

func blankText(value *string) bool {
	return value == nil || strings.TrimSpace(*value) == ""
}

func (s *Service) FetchChannelModels(ctx context.Context, platformID string, input ChannelModelsInput) ([]string, error) {
	if _, err := s.requirePlatform(ctx, platformID); err != nil {
		return nil, err
	}
	input.Sub2APIGroupID = strings.TrimSpace(input.Sub2APIGroupID)
	input.KeyID = strings.TrimSpace(input.KeyID)
	if input.Sub2APIGroupID == "" || input.KeyID == "" {
		return nil, serviceError(ErrorValidation, "请先选择 Sub2API 分组并创建密钥")
	}
	groups, err := s.repository.NewAPILocalGroups(ctx)
	if err != nil {
		return nil, err
	}
	if !localGroupExists(groups, input.Sub2APIGroupID) {
		return nil, serviceError(ErrorValidation, "模型获取目标不是已登记的 Sub2API 分组")
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	serviceKey, err := s.revealChannelKey(ctx, target, input.KeyID, input.Sub2APIGroupID)
	if err != nil {
		return nil, err
	}
	models, err := s.fetchSub2APIModels(ctx, input.BaseURL, serviceKey)
	return models, wrapServiceError(ErrorUpstream, err)
}

func (s *Service) channelEndpoints(ctx context.Context, target configstore.TargetSettings) []ChannelEndpoint {
	fallbackURL, fallbackErr := configstore.ValidateBaseURL(target.BaseURL)
	if fallbackErr != nil {
		return []ChannelEndpoint{}
	}
	fallback := []ChannelEndpoint{{Name: "管理平台地址", BaseURL: fallbackURL, Default: true}}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fallbackURL+"/api/v1/settings/public",
		nil,
	)
	if err != nil {
		return fallback
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Sub2API-Console/0.1")
	response, err := s.client.Do(request)
	if err != nil {
		return fallback
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fallback
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil || len(raw) > maximumResponseBytes {
		return fallback
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return fallback
	}
	settings := payload
	if data, ok := payload["data"].(map[string]any); ok {
		settings = data
	}
	endpoints := decodeChannelEndpoints(settings)
	if len(endpoints) == 0 {
		return fallback
	}
	return endpoints
}

func decodeChannelEndpoints(settings map[string]any) []ChannelEndpoint {
	result := []ChannelEndpoint{}
	seen := map[string]struct{}{}
	appendEndpoint := func(name, rawURL string, isDefault bool) {
		baseURL, err := configstore.ValidateBaseURL(rawURL)
		if err != nil {
			return
		}
		if _, found := seen[baseURL]; found {
			return
		}
		seen[baseURL] = struct{}{}
		name = strings.TrimSpace(name)
		if name == "" {
			name = baseURL
		}
		result = append(result, ChannelEndpoint{Name: name, BaseURL: baseURL, Default: isDefault})
	}
	if raw, ok := settings["api_base_url"].(string); ok {
		appendEndpoint("API 端点", raw, true)
	}
	if rawEndpoints, ok := settings["custom_endpoints"].([]any); ok {
		for _, rawEndpoint := range rawEndpoints {
			endpoint, ok := rawEndpoint.(map[string]any)
			if !ok {
				continue
			}
			name, _ := endpoint["name"].(string)
			baseURL, _ := endpoint["endpoint"].(string)
			appendEndpoint(name, baseURL, false)
		}
	}
	return result
}

func (s *Service) localGroup(ctx context.Context, groupID string) (business.NewAPILocalGroup, error) {
	if groupID == "" {
		return business.NewAPILocalGroup{}, serviceError(ErrorValidation, "请选择 Sub2API 分组")
	}
	groups, err := s.repository.NewAPILocalGroups(ctx)
	if err != nil {
		return business.NewAPILocalGroup{}, err
	}
	for _, group := range groups {
		if group.ID == groupID {
			return group, nil
		}
	}
	return business.NewAPILocalGroup{}, serviceError(ErrorValidation, "密钥目标不是已登记的 Sub2API 分组")
}

func (s *Service) revealChannelKey(ctx context.Context, target configstore.TargetSettings, keyID, groupID string) (string, error) {
	key, err := s.private.UpstreamKeySecret(ctx, target.BaseURL, keyID, groupID)
	if err != nil {
		return "", fmt.Errorf("Sub2API 本地密钥读取失败：%w", err)
	}
	if key == nil || strings.TrimSpace(key.Secret) == "" || key.GroupID != groupID {
		return "", errors.New("本地未找到与所选分组匹配的 Sub2API 密钥，请重新创建密钥")
	}
	return strings.TrimSpace(key.Secret), nil
}

func sub2APIUserLoginRecord(target configstore.TargetSettings) configstore.AuthRecord {
	return configstore.AuthRecord{
		Host: configstore.CanonicalHost(target.BaseURL), BaseURL: target.BaseURL,
		UpstreamType: "sub2api", AuthMode: "sub2api_user_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}
}

func stableOperationName(groupName, kind string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(append([]string{kind}, parts...), "\x00")))
	name := strings.TrimSpace(groupName)
	if len([]rune(name)) > 72 {
		name = string([]rune(name)[:72])
	}
	return fmt.Sprintf("NewAPI-%s-console-%s", name, hex.EncodeToString(digest[:12]))
}

func (s *Service) findChannelByName(ctx context.Context, platform configstore.NewAPIPlatform, name string) (map[string]any, bool, error) {
	var match map[string]any
	for page := 0; page < 1000; page++ {
		payload, err := s.request(ctx, platform, http.MethodGet, fmt.Sprintf("/api/channel/?p=%d&page_size=100", page), nil)
		if err != nil {
			return nil, false, err
		}
		rows := newAPIChannelRows(payload)
		for _, row := range rows {
			if firstText(row, "name") != name {
				continue
			}
			if match != nil {
				return nil, false, errors.New("远端存在多个同 marker 渠道")
			}
			match = row
		}
		if len(rows) < 100 {
			return match, match != nil, nil
		}
	}
	return nil, false, errors.New("远端渠道目录超过分页上限")
}

func newAPIChannelRows(payload any) []map[string]any {
	var values []any
	switch item := payload.(type) {
	case []any:
		values = item
	case map[string]any:
		for _, key := range []string{"items", "channels", "data"} {
			if rows, ok := item[key].([]any); ok {
				values = rows
				break
			}
		}
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if row, ok := value.(map[string]any); ok {
			result = append(result, row)
		}
	}
	return result
}

func publicChannelResult(value map[string]any, name string) map[string]any {
	result := map[string]any{"created": true, "name": name}
	if value == nil {
		return result
	}
	if id := firstText(value, "id", "channel_id"); id != "" {
		result["id"] = id
	}
	return result
}

func (s *Service) validateNewAPIGroups(ctx context.Context, platform configstore.NewAPIPlatform, values []string) ([]string, error) {
	groups := normalizeModels(values)
	if len(groups) == 0 {
		return nil, serviceError(ErrorValidation, "渠道至少需要一个 New API 分组")
	}
	options, err := s.readOptions(ctx, platform)
	if err != nil {
		return nil, err
	}
	remoteGroups, err := decodeGroups(options["GroupRatio"])
	if err != nil {
		return nil, errors.New("New API 当前分组不可读")
	}
	available := make(map[string]struct{}, len(remoteGroups))
	for _, group := range remoteGroups {
		available[group.ID] = struct{}{}
	}
	for _, group := range groups {
		if _, found := available[group]; !found {
			return nil, serviceError(ErrorValidation, fmt.Sprintf("New API 分组 %s 不存在", group))
		}
	}
	return groups, nil
}

func localGroupExists(groups []business.NewAPILocalGroup, groupID string) bool {
	for _, group := range groups {
		if group.ID == groupID {
			return true
		}
	}
	return false
}

func (s *Service) fetchSub2APIModels(ctx context.Context, baseURL, serviceKey string) ([]string, error) {
	normalized, err := configstore.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, serviceError(ErrorValidation, "Sub2API 管理平台地址无效")
	}
	endpoint, err := url.Parse(normalized)
	if err != nil {
		return nil, serviceError(ErrorValidation, "Sub2API 管理平台地址无效")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/models"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+serviceKey)
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求 Sub2API 模型失败：%w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maximumResponseBytes {
		return nil, errors.New("Sub2API 模型响应超过大小限制")
	}
	var payload any
	if json.Unmarshal(raw, &payload) != nil {
		return nil, fmt.Errorf("Sub2API 模型响应不可读（HTTP %d）", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Sub2API 模型获取失败（HTTP %d%s）", response.StatusCode, remoteDetail(payload))
	}
	models := channelModelIDs(payload)
	if len(models) == 0 {
		return nil, errors.New("Sub2API 未返回可用模型")
	}
	return models, nil
}

func channelModelIDs(payload any) []string {
	models := []string{}
	var walk func(any)
	walk = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			for _, key := range []string{"id", "model_id"} {
				if model, ok := item[key].(string); ok {
					models = append(models, model)
					break
				}
			}
			for _, key := range []string{"data", "models", "items"} {
				if nested, found := item[key]; found {
					walk(nested)
				}
			}
		case []any:
			for _, nested := range item {
				walk(nested)
			}
		case string:
			models = append(models, item)
		}
	}
	walk(payload)
	return normalizeModels(models)
}

func (s *Service) SaveModelPrices(ctx context.Context, platformID string, inputs []ModelPriceInput) (RemoteSnapshot, error) {
	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return RemoteSnapshot{}, err
	}
	options, err := s.readOptions(ctx, *platform)
	if err != nil {
		return RemoteSnapshot{}, err
	}
	numericKeys := []string{
		"ModelPrice", "ModelRatio", "CompletionRatio", "CacheRatio", "CreateCacheRatio",
		"ImageRatio", "AudioRatio", "AudioCompletionRatio",
	}
	numericOptions := make(map[string]map[string]string, len(numericKeys))
	for _, key := range numericKeys {
		values, decodeErr := decodeDecimalMap(options[key])
		if decodeErr != nil {
			return RemoteSnapshot{}, fmt.Errorf("New API 当前 %s 配置不可读", key)
		}
		numericOptions[key] = values
	}
	billingModes, err := decodeStringMap(options["billing_setting.billing_mode"])
	if err != nil {
		return RemoteSnapshot{}, errors.New("New API 当前计费模式配置不可读")
	}
	billingExprs, err := decodeStringMap(options["billing_setting.billing_expr"])
	if err != nil {
		return RemoteSnapshot{}, errors.New("New API 当前计费表达式配置不可读")
	}
	for _, input := range inputs {
		model := strings.TrimSpace(input.Model)
		if model == "" || len(model) > 256 {
			return RemoteSnapshot{}, serviceError(ErrorValidation, "模型价格包含空模型或过长模型名称")
		}
		for _, key := range numericKeys {
			delete(numericOptions[key], model)
		}
		delete(billingModes, model)
		delete(billingExprs, model)

		if strings.TrimSpace(input.ModelPrice) != "" {
			if strings.TrimSpace(input.BillingMode) != "" || strings.TrimSpace(input.BillingExpr) != "" {
				return RemoteSnapshot{}, serviceError(ErrorValidation, fmt.Sprintf("%s 不能同时使用固定价格和计费表达式", model))
			}
			if !validDecimal(input.ModelPrice) {
				return RemoteSnapshot{}, serviceError(ErrorValidation, "模型价格包含无效固定价格")
			}
			numericOptions["ModelPrice"][model] = strings.TrimSpace(input.ModelPrice)
			continue
		}
		if !validDecimal(input.InputRatio) || !validDecimal(input.CompletionRatio) {
			return RemoteSnapshot{}, serviceError(ErrorValidation, "模型价格包含无效输入或输出价格")
		}
		numericOptions["ModelRatio"][model] = strings.TrimSpace(input.InputRatio)
		numericOptions["CompletionRatio"][model] = strings.TrimSpace(input.CompletionRatio)
		optionalValues := map[string]string{
			"CacheRatio": input.CacheRatio, "CreateCacheRatio": input.CreateCacheRatio,
			"ImageRatio": input.ImageRatio, "AudioRatio": input.AudioRatio,
			"AudioCompletionRatio": input.AudioCompletionRatio,
		}
		for key, raw := range optionalValues {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			if !validDecimal(value) {
				return RemoteSnapshot{}, serviceError(ErrorValidation, fmt.Sprintf("%s 的 %s 配置无效", model, key))
			}
			numericOptions[key][model] = value
		}

		billingMode := strings.TrimSpace(input.BillingMode)
		billingExpr := strings.TrimSpace(input.BillingExpr)
		if billingMode == "" && billingExpr != "" {
			return RemoteSnapshot{}, serviceError(ErrorValidation, fmt.Sprintf("%s 缺少计费模式", model))
		}
		if billingMode != "" {
			if billingMode != "tiered_expr" {
				return RemoteSnapshot{}, serviceError(ErrorValidation, fmt.Sprintf("%s 的计费模式不受支持", model))
			}
			if billingExpr == "" || len(billingExpr) > 16384 {
				return RemoteSnapshot{}, serviceError(ErrorValidation, fmt.Sprintf("%s 的计费表达式无效", model))
			}
			billingModes[model] = billingMode
			billingExprs[model] = billingExpr
		}
	}
	writtenKeys := make([]string, 0, len(numericKeys)+2)
	for _, key := range numericKeys {
		if err := s.writeOption(ctx, *platform, key, numericOptions[key]); err != nil {
			return RemoteSnapshot{}, errors.Join(err, s.rollbackOptions(ctx, *platform, options, writtenKeys))
		}
		writtenKeys = append(writtenKeys, key)
	}
	if err := s.writeStringOption(ctx, *platform, "billing_setting.billing_mode", billingModes); err != nil {
		return RemoteSnapshot{}, errors.Join(err, s.rollbackOptions(ctx, *platform, options, writtenKeys))
	}
	writtenKeys = append(writtenKeys, "billing_setting.billing_mode")
	if err := s.writeStringOption(ctx, *platform, "billing_setting.billing_expr", billingExprs); err != nil {
		return RemoteSnapshot{}, errors.Join(err, s.rollbackOptions(ctx, *platform, options, writtenKeys))
	}
	return s.Refresh(ctx, platformID)
}

func (s *Service) requirePlatform(ctx context.Context, id string) (*configstore.NewAPIPlatform, error) {
	platform, err := s.private.NewAPIPlatform(ctx, id)
	if err != nil {
		return nil, err
	}
	if platform == nil {
		return nil, serviceError(ErrorNotFound, "New API 平台不存在")
	}
	return platform, nil
}

func (s *Service) testConnection(ctx context.Context, item configstore.NewAPIPlatform) error {
	if strings.TrimSpace(item.AdminKey) == "" || strings.TrimSpace(item.UserID) == "" {
		return serviceError(ErrorValidation, "New API Admin Key 和 User ID 不能为空")
	}
	_, err := s.request(ctx, item, http.MethodGet, "/api/option/", nil)
	if err != nil {
		return fmt.Errorf("New API 管理凭据验证失败：%w", err)
	}
	return nil
}

func (s *Service) readOptions(ctx context.Context, item configstore.NewAPIPlatform) (map[string]string, error) {
	payload, err := s.request(ctx, item, http.MethodGet, "/api/option/", nil)
	if err != nil {
		return nil, err
	}
	return optionValues(payload)
}

func (s *Service) syncGroupRatios(ctx context.Context, platform configstore.NewAPIPlatform, bindings []business.NewAPIGroupBinding, local map[string]business.NewAPILocalGroup) error {
	needsSync := false
	for _, binding := range bindings {
		needsSync = needsSync || binding.SyncRatio
	}
	if !needsSync {
		return nil
	}
	options, err := s.readOptions(ctx, platform)
	if err != nil {
		return err
	}
	ratios, err := decodeDecimalMap(options["GroupRatio"])
	if err != nil {
		return errors.New("New API 当前分组倍率不可读")
	}
	for _, binding := range bindings {
		if !binding.SyncRatio {
			continue
		}
		group := local[binding.Sub2APIGroupID]
		if group.Ratio == nil || !validDecimal(*group.Ratio) {
			return fmt.Errorf("Sub2API 分组 %s 没有可同步的倍率", group.Name)
		}
		ratios[binding.NewAPIGroupID] = *group.Ratio
	}
	return s.writeOption(ctx, platform, "GroupRatio", ratios)
}

func (s *Service) writeOption(ctx context.Context, platform configstore.NewAPIPlatform, key string, value map[string]string) error {
	numericValues := make(map[string]json.RawMessage, len(value))
	for itemKey, raw := range value {
		if !validDecimal(raw) {
			return fmt.Errorf("%s 的倍率无效", itemKey)
		}
		numericValues[itemKey] = json.RawMessage(strings.TrimSpace(raw))
	}
	encoded, err := json.Marshal(numericValues)
	if err != nil {
		return err
	}
	_, err = s.request(ctx, platform, http.MethodPut, "/api/option/", map[string]any{"key": key, "value": string(encoded)})
	return err
}

func (s *Service) writeStringOption(ctx context.Context, platform configstore.NewAPIPlatform, key string, value map[string]string) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.request(ctx, platform, http.MethodPut, "/api/option/", map[string]any{"key": key, "value": string(encoded)})
	return err
}

func (s *Service) rollbackOptions(ctx context.Context, platform configstore.NewAPIPlatform, previous map[string]string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	var result error
	for index := len(keys) - 1; index >= 0; index-- {
		key := keys[index]
		_, err := s.request(rollbackCtx, platform, http.MethodPut, "/api/option/", map[string]any{"key": key, "value": previous[key]})
		if err != nil {
			result = errors.Join(result, fmt.Errorf("%s 回滚失败：%w", key, err))
		}
	}
	return result
}

func (s *Service) request(ctx context.Context, platform configstore.NewAPIPlatform, method, path string, body map[string]any) (any, error) {
	payload, err := s.requestRaw(ctx, platform, method, path, body)
	return payload, wrapServiceError(ErrorUpstream, err)
}

func (s *Service) requestRaw(ctx context.Context, platform configstore.NewAPIPlatform, method, path string, body map[string]any) (any, error) {
	validatedBase, err := configstore.ValidateBaseURL(platform.BaseURL)
	if err != nil {
		return nil, errors.New("New API 平台地址无效")
	}
	base, err := url.Parse(validatedBase)
	if err != nil {
		return nil, errors.New("New API 平台地址无效")
	}
	relative, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(relative.Path, "/") {
		return nil, errors.New("New API 请求路径无效")
	}
	base.Path = strings.TrimRight(base.Path, "/") + relative.Path
	base.RawQuery = relative.RawQuery
	var reader io.Reader
	if body != nil {
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			return nil, encodeErr
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, base.String(), reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(platform.AdminKey))
	request.Header.Set("New-Api-User", strings.TrimSpace(platform.UserID))
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求 New API 失败：%w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maximumResponseBytes {
		return nil, errors.New("New API 响应超过大小限制")
	}
	var payload any
	if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &payload) != nil {
		return nil, fmt.Errorf("New API 返回不可读内容（HTTP %d）", response.StatusCode)
	}
	if len(bytes.TrimSpace(raw)) == 0 && response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil, errors.New("New API 返回空内容")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("New API 请求失败（HTTP %d%s）", response.StatusCode, remoteDetail(payload))
	}
	if err := responseBusinessError(payload); err != nil {
		return nil, err
	}
	if object, ok := payload.(map[string]any); ok {
		if data, present := object["data"]; present {
			return data, nil
		}
	}
	return payload, nil
}

func optionValues(payload any) (map[string]string, error) {
	result := map[string]string{}
	switch value := payload.(type) {
	case map[string]any:
		for key, raw := range value {
			if text, ok := raw.(string); ok {
				result[key] = text
			}
		}
	case []any:
		for _, raw := range value {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			key, _ := item["key"].(string)
			text, _ := item["value"].(string)
			if key != "" {
				result[key] = text
			}
		}
	default:
		return nil, errors.New("New API 配置返回格式不可读")
	}
	return result, nil
}

func decodeGroups(raw string) ([]RemoteGroup, error) {
	values, err := decodeDecimalMap(raw)
	if err != nil {
		return nil, errors.New("New API 分组倍率配置不可读")
	}
	result := make([]RemoteGroup, 0, len(values))
	for name, ratio := range values {
		copy := ratio
		result = append(result, RemoteGroup{ID: name, Name: name, Ratio: &copy})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Name < result[right].Name })
	return result, nil
}

func decodeModels(modelRaw, completionRaw string) ([]ModelPrice, error) {
	models, err := decodeDecimalMap(modelRaw)
	if err != nil {
		return nil, errors.New("New API 模型倍率配置不可读")
	}
	completion, err := decodeDecimalMap(completionRaw)
	if err != nil {
		return nil, errors.New("New API 补全倍率配置不可读")
	}
	result := make([]ModelPrice, 0, len(models))
	for model, ratio := range models {
		completionRatio := "1"
		if configured, found := completion[model]; found {
			completionRatio = configured
		}
		result = append(result, ModelPrice{Model: model, InputRatio: ratio, CompletionRatio: completionRatio})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result, nil
}

func decodeConfiguredModels(options map[string]string) ([]ModelPrice, error) {
	keys := []string{"ModelPrice", "ModelRatio", "CompletionRatio", "CacheRatio", "CreateCacheRatio", "ImageRatio", "AudioRatio", "AudioCompletionRatio"}
	values := make(map[string]map[string]string, len(keys))
	billingModes, err := decodeStringMap(options["billing_setting.billing_mode"])
	if err != nil {
		return nil, errors.New("New API 计费模式配置不可读")
	}
	billingExprs, err := decodeStringMap(options["billing_setting.billing_expr"])
	if err != nil {
		return nil, errors.New("New API 计费表达式配置不可读")
	}
	modelNames := map[string]struct{}{}
	for _, key := range keys {
		decoded, err := decodeDecimalMap(options[key])
		if err != nil {
			return nil, fmt.Errorf("New API %s 配置不可读", key)
		}
		values[key] = decoded
		for model := range decoded {
			modelNames[model] = struct{}{}
		}
	}
	for model := range billingModes {
		modelNames[model] = struct{}{}
	}
	for model := range billingExprs {
		modelNames[model] = struct{}{}
	}
	createCache1hRatios, err := decodeDecimalMap(options["CreateCache1hRatio"])
	if err != nil {
		return nil, errors.New("New API CreateCache1hRatio 配置不可读")
	}
	models := make([]ModelPrice, 0, len(modelNames))
	for modelName := range modelNames {
		completion := values["CompletionRatio"][modelName]
		billingMode := billingModes[modelName]
		if billingMode == "tiered_expr" {
			billingMode = "tiered_expr"
		} else if billingMode == "per_second" {
			billingMode = "per-second"
		} else if values["ModelPrice"][modelName] != "" {
			billingMode = "per-request"
		} else {
			billingMode = "per-token"
		}
		models = append(models, ModelPrice{
			Model: modelName, ModelPrice: values["ModelPrice"][modelName], InputRatio: values["ModelRatio"][modelName], CompletionRatio: completion,
			InputPrice: ratioToTokenPrice(values["ModelRatio"][modelName]), CompletionPrice: multiplyDecimal(ratioToTokenPrice(values["ModelRatio"][modelName]), completion),
			CacheCreatePrice: multiplyDecimal(ratioToTokenPrice(values["ModelRatio"][modelName]), values["CreateCacheRatio"][modelName]),
			CacheReadPrice:   multiplyDecimal(ratioToTokenPrice(values["ModelRatio"][modelName]), values["CacheRatio"][modelName]),
			BillingMode:      billingMode, BillingExpr: billingExprs[modelName],
			CacheRatio: values["CacheRatio"][modelName], CreateCacheRatio: values["CreateCacheRatio"][modelName], CreateCache1hRatio: createCache1hRatios[modelName],
			ImageRatio: values["ImageRatio"][modelName], AudioRatio: values["AudioRatio"][modelName], AudioCompletionRatio: values["AudioCompletionRatio"][modelName],
		})
	}
	sort.Slice(models, func(left, right int) bool { return models[left].Model < models[right].Model })
	return models, nil
}

func findUnsetModelPrices(configured []ModelPrice, candidateNames []string) []ModelPrice {
	configuredByModel := make(map[string]ModelPrice, len(configured))
	for _, item := range configured {
		configuredByModel[item.Model] = item
	}
	result := make([]ModelPrice, 0)
	for _, name := range normalizeModels(candidateNames) {
		item, found := configuredByModel[name]
		if !found {
			result = append(result, ModelPrice{Model: name, BillingMode: "per-token"})
			continue
		}
		if item.BillingMode != "tiered_expr" && item.ModelPrice == "" && item.InputRatio == "" {
			result = append(result, item)
		}
	}
	return result
}

func mergeModelPriceRows(groups ...[]ModelPrice) []ModelPrice {
	byModel := map[string]ModelPrice{}
	for _, group := range groups {
		for _, item := range group {
			if _, found := byModel[item.Model]; !found {
				byModel[item.Model] = item
			}
		}
	}
	result := make([]ModelPrice, 0, len(byModel))
	for _, item := range byModel {
		result = append(result, item)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result
}

func decodeStringMap(raw string) (map[string]string, error) {
	result := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return result, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	var values map[string]any
	if err := decoder.Decode(&values); err != nil || values == nil {
		return nil, errors.New("配置不是 JSON 对象")
	}
	for key, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(key) != "" {
			result[strings.TrimSpace(key)] = strings.TrimSpace(text)
		}
	}
	return result, nil
}

func decodeModelNames(payload any) []string {
	result := []string{}
	var walk func(any)
	walk = func(value any) {
		switch item := value.(type) {
		case string:
			result = append(result, item)
		case []any:
			for _, nested := range item {
				walk(nested)
			}
		case map[string]any:
			if data, ok := item["data"]; ok {
				walk(data)
			}
		}
	}
	walk(payload)
	return normalizeModels(result)
}

var defaultNewAPIToolPrices = []ToolPrice{
	{Tool: "web_search", Price: "10"},
	{Tool: "web_search_preview", Price: "10"},
	{Tool: "web_search_preview:gpt-4o*", Price: "25"},
	{Tool: "web_search_preview:gpt-4.1*", Price: "25"},
	{Tool: "web_search_preview:gpt-4o-mini*", Price: "25"},
	{Tool: "web_search_preview:gpt-4.1-mini*", Price: "25"},
	{Tool: "file_search", Price: "2.5"},
	{Tool: "google_search", Price: "14"},
	{Tool: "image_generation", Price: "150"},
}

func decodeToolPrices(raw string) []ToolPrice {
	prices := make(map[string]string, len(defaultNewAPIToolPrices))
	for _, item := range defaultNewAPIToolPrices {
		prices[item.Tool] = item.Price
	}
	if strings.TrimSpace(raw) != "" {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		overrides := map[string]any{}
		if decoder.Decode(&overrides) == nil {
			for tool, value := range overrides {
				price := firstDecimal(map[string]any{"value": value}, "value")
				if strings.TrimSpace(tool) != "" && price != "" && validDecimal(price) {
					prices[tool] = price
				}
			}
		}
	}

	result := make([]ToolPrice, 0, len(prices))
	for _, item := range defaultNewAPIToolPrices {
		result = append(result, ToolPrice{Tool: item.Tool, Price: prices[item.Tool]})
		delete(prices, item.Tool)
	}
	extraTools := make([]string, 0, len(prices))
	for tool := range prices {
		extraTools = append(extraTools, tool)
	}
	sort.Strings(extraTools)
	for _, tool := range extraTools {
		result = append(result, ToolPrice{Tool: tool, Price: prices[tool]})
	}
	return result
}

func ratioToTokenPrice(ratio string) string {
	if strings.TrimSpace(ratio) == "" || !validDecimal(ratio) {
		return ""
	}
	value, ok := new(big.Rat).SetString(strings.TrimSpace(ratio))
	if !ok {
		return ""
	}
	// New API's model pricing editor represents token prices per 1M tokens;
	// its base input price is model ratio × 2.
	return formatRatio(new(big.Rat).Mul(value, big.NewRat(2, 1)))
}

func multiplyDecimal(left, right string) string {
	if left == "" || right == "" || !validDecimal(left) || !validDecimal(right) {
		return ""
	}
	a, okA := new(big.Rat).SetString(left)
	b, okB := new(big.Rat).SetString(right)
	if !okA || !okB {
		return ""
	}
	return formatRatio(new(big.Rat).Mul(a, b))
}

func decodePricingCatalog(payload any) []ModelPrice {
	rows := []any{}
	switch value := payload.(type) {
	case []any:
		rows = value
	case map[string]any:
		for _, key := range []string{"models", "items", "data"} {
			if values, ok := value[key].([]any); ok {
				rows = values
				break
			}
		}
	}
	result := []ModelPrice{}
	seen := map[string]struct{}{}
	for _, raw := range rows {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		model := firstText(item, "model", "model_name", "name")
		input := firstDecimal(item, "model_ratio", "input_ratio", "ratio")
		inputPrice := firstDecimal(item, "input_price")
		completion := firstDecimal(item, "completion_ratio", "output_ratio")
		fixedPrice := firstDecimal(item, "model_price", "price")
		if completion == "" {
			completion = "1"
		}
		if model == "" || (input == "" && fixedPrice == "" && inputPrice == "") || (input != "" && !validDecimal(input)) || (inputPrice != "" && !validDecimal(inputPrice)) || !validDecimal(completion) {
			continue
		}
		if _, duplicate := seen[model]; duplicate {
			continue
		}
		seen[model] = struct{}{}
		if inputPrice == "" {
			inputPrice = ratioToTokenPrice(input)
		}
		completionPrice := firstDecimal(item, "output_price", "completion_price")
		if completionPrice == "" {
			completionPrice = multiplyDecimal(inputPrice, completion)
		}
		cacheRatio := firstDecimal(item, "cache_ratio", "cache_read_ratio")
		createCacheRatio := firstDecimal(item, "create_cache_ratio", "cache_write_ratio")
		result = append(result, ModelPrice{
			Model: model, ModelPrice: fixedPrice, InputRatio: input, CompletionRatio: completion,
			InputPrice: inputPrice, CompletionPrice: completionPrice,
			CacheCreatePrice: multiplyDecimal(inputPrice, createCacheRatio), CacheReadPrice: multiplyDecimal(inputPrice, cacheRatio),
			BillingMode: firstText(item, "billing_mode"), BillingExpr: firstText(item, "billing_expr"),
			CacheRatio:           cacheRatio,
			CreateCacheRatio:     firstDecimal(item, "create_cache_ratio", "cache_write_ratio"),
			CreateCache1hRatio:   firstDecimal(item, "create_cache_1h_ratio", "cache_write_1h_ratio"),
			ImageRatio:           firstDecimal(item, "image_ratio", "image_input_ratio"),
			AudioRatio:           firstDecimal(item, "audio_ratio", "audio_input_ratio"),
			AudioCompletionRatio: firstDecimal(item, "audio_completion_ratio", "audio_output_ratio"),
		})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result
}

func (s *Service) requestSub2APIModelPlaza(ctx context.Context, rawBaseURL string) (any, error) {
	base, err := configstore.ValidateBaseURL(rawBaseURL)
	if err != nil {
		return nil, errors.New("Sub2API 管理平台地址无效")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/model-plaza", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Sub2API-Console/0.1")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maximumResponseBytes {
		return nil, errors.New("Sub2API 模型广场响应超过大小限制")
	}
	var payload any
	if json.Unmarshal(raw, &payload) != nil {
		return nil, fmt.Errorf("Sub2API 模型广场响应不可读（HTTP %d）", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Sub2API 模型广场获取失败（HTTP %d%s）", response.StatusCode, remoteDetail(payload))
	}
	if object, ok := payload.(map[string]any); ok {
		if success, present := object["success"].(bool); present && !success {
			return nil, fmt.Errorf("Sub2API 模型广场拒绝访问%s", remoteDetail(payload))
		}
	}
	return payload, nil
}

func (s *Service) fetchRemotePricingCatalog(ctx context.Context) ([]Sub2APIModelPrice, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultSub2APIPricingURL, nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Sub2API-Console/0.1")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("请求 Sub2API 远程价卡失败：%w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > maximumResponseBytes {
		return nil, nil, errors.New("Sub2API 远程价卡响应超过大小限制")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("Sub2API 远程价卡获取失败（HTTP %d）", response.StatusCode)
	}
	prices, err := decodeSub2APIPricingJSON(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("Sub2API 远程价卡解析失败：%w", err)
	}
	return prices, raw, nil
}

func buildRemotePricingSource(raw []byte, fetchedAt time.Time) RemotePricingSource {
	digest := sha256.Sum256(raw)
	return RemotePricingSource{
		SourceURL: defaultSub2APIPricingURL,
		Content:   string(raw),
		FetchedAt: fetchedAt.UTC().Format(time.RFC3339Nano),
		SizeBytes: len(raw),
		SHA256:    hex.EncodeToString(digest[:]),
	}
}

func decodeSub2APIModelPlaza(payload any) []Sub2APIModelPrice {
	root, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	if data, found := root["data"].(map[string]any); found {
		root = data
	}
	groups, ok := root["groups"].([]any)
	if !ok {
		return nil
	}
	byModel := map[string]Sub2APIModelPrice{}
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			continue
		}
		models, ok := group["models"].([]any)
		if !ok {
			continue
		}
		for _, rawModel := range models {
			model, ok := rawModel.(map[string]any)
			if !ok {
				continue
			}
			name := firstText(model, "name", "model", "model_name")
			pricing, _ := model["official_pricing"].(map[string]any)
			if pricing == nil {
				pricing, _ = model["pricing"].(map[string]any)
			}
			input := firstDecimal(pricing, "input_price")
			output := firstDecimal(pricing, "output_price")
			if name == "" || input == "" || output == "" || !validDecimal(input) || !validDecimal(output) {
				continue
			}
			modelRatio, completionRatio, ok := sub2APIRatios(input, output)
			if !ok {
				continue
			}
			item := Sub2APIModelPrice{Model: name, InputPrice: input, OutputPrice: output, ModelRatio: modelRatio, CompletionRatio: completionRatio}
			item.CacheWritePrice = firstDecimal(pricing, "cache_write_price")
			item.CacheWrite1hPrice = firstDecimal(pricing, "cache_write_1h_price")
			item.CacheReadPrice = firstDecimal(pricing, "cache_read_price")
			item.CacheRatio = priceRatio(input, item.CacheReadPrice)
			item.CreateCacheRatio = priceRatio(input, item.CacheWritePrice)
			item.CreateCache1hRatio = priceRatio(input, item.CacheWrite1hPrice)
			if _, exists := byModel[name]; !exists {
				byModel[name] = item
			}
		}
	}
	result := make([]Sub2APIModelPrice, 0, len(byModel))
	for _, item := range byModel {
		result = append(result, item)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result
}

func decodeSub2APIPricingJSON(raw []byte) ([]Sub2APIModelPrice, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var entries map[string]any
	if err := decoder.Decode(&entries); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	result := make([]Sub2APIModelPrice, 0, len(entries))
	for name, rawEntry := range entries {
		if name == "sample_spec" {
			continue
		}
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		input := firstDecimal(entry, "input_cost_per_token")
		output := firstDecimal(entry, "output_cost_per_token")
		imageInput := firstDecimal(entry, "input_cost_per_image_token")
		imageOutputPerToken := firstDecimal(entry, "output_cost_per_image_token")
		imageOutput := firstDecimal(entry, "output_cost_per_image")
		if imageOutput == "" {
			imageOutput = imageOutputPerToken
		}
		if input == "" && output == "" && imageInput == "" && imageOutput == "" {
			continue
		}
		item := Sub2APIModelPrice{
			Model: name, InputPrice: input, OutputPrice: output,
			ImageInputPrice: imageInput, ImageOutputPrice: imageOutput,
			Provider: firstText(entry, "litellm_provider"), Mode: firstText(entry, "mode"),
			CacheWritePrice:   firstDecimal(entry, "cache_creation_input_token_cost"),
			CacheWrite1hPrice: firstDecimal(entry, "cache_creation_input_token_cost_above_1hr"),
			CacheReadPrice:    firstDecimal(entry, "cache_read_input_token_cost"),
		}
		ratioOutput := output
		if ratioOutput == "" {
			ratioOutput = imageOutputPerToken
		}
		if input != "" && validDecimal(input) {
			if ratioOutput == "" {
				ratioOutput = "0"
			}
			item.ModelRatio, item.CompletionRatio, _ = sub2APIRatios(input, ratioOutput)
			item.CacheRatio = priceRatio(input, item.CacheReadPrice)
			item.CreateCacheRatio = priceRatio(input, item.CacheWritePrice)
			item.CreateCache1hRatio = priceRatio(input, item.CacheWrite1hPrice)
			item.ImageRatio = priceRatio(input, imageInput)
		}
		applyRemoteLongContextPrices(entry, &item)
		result = append(result, item)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result, nil
}

func applyRemoteLongContextPrices(entry map[string]any, item *Sub2APIModelPrice) {
	type tierPrices struct {
		input  string
		output string
	}
	tiers := map[int]tierPrices{}
	for key := range entry {
		match := remoteLongContextPricePattern.FindStringSubmatch(key)
		if match == nil {
			continue
		}
		thousands, err := strconv.Atoi(match[2])
		value := firstDecimal(entry, key)
		if err != nil || thousands <= 0 || !positiveDecimal(value) {
			continue
		}
		threshold := thousands * 1000
		prices := tiers[threshold]
		if match[1] == "input" {
			prices.input = value
		} else {
			prices.output = value
		}
		tiers[threshold] = prices
	}
	threshold := 0
	for candidate := range tiers {
		if threshold == 0 || candidate < threshold {
			threshold = candidate
		}
	}
	if threshold == 0 {
		return
	}
	prices := tiers[threshold]
	inputMultiplier := priceRatio(item.InputPrice, prices.input)
	outputMultiplier := priceRatio(item.OutputPrice, prices.output)
	if prices.input == "" {
		prices.input = item.InputPrice
	}
	if prices.output == "" {
		prices.output = item.OutputPrice
	}
	if inputMultiplier == "" {
		inputMultiplier = "1"
	}
	if outputMultiplier == "" {
		outputMultiplier = "1"
	}
	if inputMultiplier == "1" && outputMultiplier == "1" {
		return
	}
	item.LongContextThreshold = threshold
	item.LongContextThresholdInclusive = strings.EqualFold(item.Provider, "xai")
	item.LongContextInputPrice = prices.input
	item.LongContextOutputPrice = prices.output
	item.LongContextCacheWritePrice = remoteTierPrice(
		item.CacheWritePrice,
		inputMultiplier,
	)
	item.LongContextCacheWrite1hPrice = remoteTierPrice(
		item.CacheWrite1hPrice,
		inputMultiplier,
	)
	item.LongContextCacheReadPrice = remoteTierPrice(
		item.CacheReadPrice,
		inputMultiplier,
	)
}

func remoteTierPrice(basePrice, multiplier string) string {
	// Sub2API bills cache long-context usage as the base cache price multiplied
	// by the input long-context multiplier. The raw *_above_* cache field is an
	// integrity signal only and remains available through the raw-source view.
	return multiplyDecimal(basePrice, multiplier)
}

func positiveDecimal(value string) bool {
	number, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	return ok && number.Sign() > 0
}

func sub2APIRatios(inputPrice, outputPrice string) (string, string, bool) {
	input, ok := new(big.Rat).SetString(strings.TrimSpace(inputPrice))
	if !ok || input.Sign() < 0 {
		return "", "", false
	}
	output, ok := new(big.Rat).SetString(strings.TrimSpace(outputPrice))
	if !ok || output.Sign() < 0 {
		return "", "", false
	}
	if input.Sign() == 0 {
		if output.Sign() > 0 {
			return "", "", false
		}
		return "0", "1", true
	}
	modelRatio := new(big.Rat).Mul(input, big.NewRat(500000, 1))
	completionRatio := new(big.Rat).Quo(output, input)
	return formatRatio(modelRatio), formatRatio(completionRatio), true
}

func priceRatio(basePrice, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	base, ok := new(big.Rat).SetString(strings.TrimSpace(basePrice))
	if !ok || base.Sign() <= 0 {
		return ""
	}
	amount, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok || amount.Sign() < 0 {
		return ""
	}
	return formatRatio(new(big.Rat).Quo(amount, base))
}

func formatRatio(value *big.Rat) string {
	text := value.FloatString(12)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func sub2APIModelRatios(models []Sub2APIModelPrice) []ModelPrice {
	result := make([]ModelPrice, 0, len(models))
	for _, model := range models {
		result = append(result, ModelPrice{
			Model: model.Model, InputRatio: model.ModelRatio, CompletionRatio: model.CompletionRatio,
			CacheRatio: model.CacheRatio, CreateCacheRatio: model.CreateCacheRatio,
			CreateCache1hRatio: model.CreateCache1hRatio,
		})
	}
	return result
}

func comparePrices(configured, references []ModelPrice) []PriceDifference {
	referenceByModel := make(map[string]ModelPrice, len(references))
	for _, item := range references {
		referenceByModel[item.Model] = item
	}
	result := make([]PriceDifference, 0)
	for _, configuredItem := range configured {
		referenceItem, hasReference := referenceByModel[configuredItem.Model]
		if !hasReference {
			copy := configuredItem
			result = append(result, PriceDifference{
				Model: configuredItem.Model, Kind: "missing_in_model_plaza", Configured: &copy,
			})
			continue
		}
		if samePrice(configuredItem, referenceItem) {
			continue
		}
		configuredCopy, referenceCopy := configuredItem, referenceItem
		result = append(result, PriceDifference{
			Model: configuredItem.Model, Kind: "ratio_mismatch", Configured: &configuredCopy, Reference: &referenceCopy,
		})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result
}

func mergeModelPrices(catalog, configured []ModelPrice) []ModelPrice {
	configuredByModel := make(map[string]ModelPrice, len(configured))
	for _, item := range configured {
		configuredByModel[item.Model] = item
	}
	result := make([]ModelPrice, 0, len(catalog))
	for _, item := range catalog {
		if local, found := configuredByModel[item.Model]; found {
			if local.ModelPrice != "" {
				item.ModelPrice = local.ModelPrice
			}
			if local.InputRatio != "" {
				item.InputRatio = local.InputRatio
			}
			if local.CompletionRatio != "" {
				item.CompletionRatio = local.CompletionRatio
			}
			if local.CacheRatio != "" {
				item.CacheRatio = local.CacheRatio
			}
			if local.CreateCacheRatio != "" {
				item.CreateCacheRatio = local.CreateCacheRatio
			}
			if local.CreateCache1hRatio != "" {
				item.CreateCache1hRatio = local.CreateCache1hRatio
			}
			if local.ImageRatio != "" {
				item.ImageRatio = local.ImageRatio
			}
			if local.AudioRatio != "" {
				item.AudioRatio = local.AudioRatio
			}
			if local.AudioCompletionRatio != "" {
				item.AudioCompletionRatio = local.AudioCompletionRatio
			}
		}
		result = append(result, item)
	}
	return result
}

func compareCatalogPrices(configured, references []ModelPrice) []PriceDifference {
	configuredByModel := make(map[string]ModelPrice, len(configured))
	for _, item := range configured {
		configuredByModel[item.Model] = item
	}
	result := make([]PriceDifference, 0)
	for _, reference := range references {
		configured, found := configuredByModel[reference.Model]
		if !found {
			copy := reference
			result = append(result, PriceDifference{Model: reference.Model, Kind: "missing_in_platform", Reference: &copy})
			continue
		}
		if samePrice(configured, reference) {
			continue
		}
		configuredCopy, referenceCopy := configured, reference
		result = append(result, PriceDifference{Model: reference.Model, Kind: "ratio_mismatch", Configured: &configuredCopy, Reference: &referenceCopy})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result
}

func samePrice(left, right ModelPrice) bool {
	return left.ModelPrice == right.ModelPrice && left.InputRatio == right.InputRatio && left.CompletionRatio == right.CompletionRatio &&
		left.InputPrice == right.InputPrice && left.CompletionPrice == right.CompletionPrice && left.BillingMode == right.BillingMode && left.BillingExpr == right.BillingExpr &&
		left.CacheCreatePrice == right.CacheCreatePrice && left.CacheReadPrice == right.CacheReadPrice &&
		left.CacheRatio == right.CacheRatio && left.CreateCacheRatio == right.CreateCacheRatio &&
		left.CreateCache1hRatio == right.CreateCache1hRatio && left.ImageRatio == right.ImageRatio &&
		left.AudioRatio == right.AudioRatio && left.AudioCompletionRatio == right.AudioCompletionRatio
}

func firstText(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstDecimal(item map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := item[key].(type) {
		case json.Number:
			return value.String()
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func decodeDecimalMap(raw string) (map[string]string, error) {
	result := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return result, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var values map[string]any
	if err := decoder.Decode(&values); err != nil || values == nil {
		return nil, errors.New("倍率配置不是 JSON 对象")
	}
	for key, rawValue := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		text := ""
		switch value := rawValue.(type) {
		case json.Number:
			text = value.String()
		case string:
			text = strings.TrimSpace(value)
		}
		if !validDecimal(text) {
			return nil, fmt.Errorf("%s 的倍率无效", key)
		}
		result[key] = text
	}
	return result, nil
}

func validDecimal(value string) bool {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && parsed >= 0
}

func normalizeModels(values []string) []string {
	result := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 256 {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func remoteDetail(payload any) string {
	object, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"message", "detail", "error"} {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return "：" + redact.Secrets(strings.TrimSpace(value))
		}
	}
	return ""
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("响应包含尾随 JSON 数据")
		}
		return fmt.Errorf("响应包含无效尾随数据：%w", err)
	}
	return nil
}

func responseBusinessError(payload any) error {
	object, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	if success, present := object["success"].(bool); present && !success {
		return fmt.Errorf("上游业务请求失败%s", remoteDetail(payload))
	}
	if rawCode, present := object["code"]; present {
		code := strings.TrimSpace(fmt.Sprint(rawCode))
		if code != "" && code != "0" && !strings.EqualFold(code, "ok") && !strings.EqualFold(code, "success") {
			return fmt.Errorf("上游业务请求失败（code=%s）%s", redact.Secrets(code), remoteDetail(payload))
		}
	}
	for _, key := range []string{"error", "errors"} {
		if value, present := object[key]; present && !emptyResponseError(value) {
			return fmt.Errorf("上游业务请求失败%s", remoteDetail(payload))
		}
	}
	return nil
}

func emptyResponseError(value any) bool {
	switch item := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(item) == ""
	case []any:
		return len(item) == 0
	case map[string]any:
		return len(item) == 0
	default:
		return false
	}
}
