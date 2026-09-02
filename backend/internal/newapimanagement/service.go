package newapimanagement

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

const maximumResponseBytes = 4 << 20

type PrivateStore interface {
	NewAPIPlatforms(context.Context) ([]configstore.NewAPIPlatformSummary, error)
	NewAPIPlatform(context.Context, string) (*configstore.NewAPIPlatform, error)
	SaveNewAPIPlatform(context.Context, configstore.NewAPIPlatform) (configstore.NewAPIPlatformSummary, error)
	DeleteNewAPIPlatform(context.Context, string) (bool, error)
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type Repository interface {
	NewAPILocalGroups(context.Context) ([]business.NewAPILocalGroup, error)
	NewAPIGroupBindings(context.Context, string) ([]business.NewAPIGroupBinding, error)
	ReplaceNewAPIGroupBindings(context.Context, string, []business.NewAPIGroupBinding) error
	DeleteNewAPIGroupBindings(context.Context, string) error
}

type KeyManager interface {
	CreateKeyWithVerification(context.Context, configstore.AuthRecord, string, string, bool) (upstreamsync.CreatedKey, error)
	RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error)
}

type Service struct {
	private    PrivateStore
	repository Repository
	client     *http.Client
	keys       KeyManager
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
	Model           string `json:"model"`
	InputRatio      string `json:"input_ratio"`
	CompletionRatio string `json:"completion_ratio"`
}

type RemoteSnapshot struct {
	Groups      []RemoteGroup     `json:"groups"`
	Models      []ModelPrice      `json:"models"`
	References  []ModelPrice      `json:"references"`
	Differences []PriceDifference `json:"differences"`
	FetchedAt   string            `json:"fetched_at"`
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
	Models         []string `json:"models"`
	NewAPIGroups   []string `json:"newapi_groups"`
}

type ChannelModelsInput struct {
	Sub2APIGroupID string `json:"sub2api_group_id"`
	KeyID          string `json:"key_id"`
}

type ChannelKeyInput struct {
	Sub2APIGroupID string `json:"sub2api_group_id"`
}

type ChannelKey struct {
	KeyID   string `json:"key_id"`
	Name    string `json:"name"`
	GroupID string `json:"group_id"`
}

type ModelPriceInput struct {
	Model           string `json:"model"`
	InputRatio      string `json:"input_ratio"`
	CompletionRatio string `json:"completion_ratio"`
}

func New(private PrivateStore, repository Repository, client *http.Client, keys KeyManager) *Service {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Service{private: private, repository: repository, client: client, keys: keys}
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
			return configstore.NewAPIPlatformSummary{}, errors.New("New API 只允许配置一个主平台")
		}
	}
	item := configstore.NewAPIPlatform{ID: input.ID, Name: input.Name, BaseURL: input.BaseURL, AdminKey: input.AdminKey, UserID: input.UserID}
	if strings.TrimSpace(item.AdminKey) == "" && strings.TrimSpace(item.ID) != "" {
		current, err := s.private.NewAPIPlatform(ctx, item.ID)
		if err != nil {
			return configstore.NewAPIPlatformSummary{}, err
		}
		if current != nil {
			item.AdminKey = current.AdminKey
		}
	}
	if _, err := configstore.ValidateBaseURL(item.BaseURL); err != nil {
		return configstore.NewAPIPlatformSummary{}, errors.New("New API 平台地址无效")
	}
	if err := s.testConnection(ctx, item); err != nil {
		return configstore.NewAPIPlatformSummary{}, err
	}
	return s.private.SaveNewAPIPlatform(ctx, item)
}

func (s *Service) DeletePlatform(ctx context.Context, platformID string) (bool, error) {
	deleted, err := s.private.DeleteNewAPIPlatform(ctx, platformID)
	if err != nil || !deleted {
		return deleted, err
	}
	return true, s.repository.DeleteNewAPIGroupBindings(ctx, platformID)
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
	models, err := decodeModels(options["ModelRatio"], options["CompletionRatio"])
	if err != nil {
		return RemoteSnapshot{}, err
	}
	references := []ModelPrice{}
	if payload, requestErr := s.request(ctx, *platform, http.MethodGet, "/api/pricing", nil); requestErr == nil {
		references = decodePricingCatalog(payload)
	}
	return RemoteSnapshot{
		Groups: groups, Models: models, References: references, Differences: comparePrices(models, references),
		FetchedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) SaveBindings(ctx context.Context, platformID string, inputs []GroupBindingInput) ([]business.NewAPIGroupBinding, error) {
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
	items := make([]business.NewAPIGroupBinding, 0, len(inputs))
	for _, input := range inputs {
		if _, found := localByID[strings.TrimSpace(input.Sub2APIGroupID)]; !found {
			return nil, errors.New("分组绑定包含不存在的 Sub2API 分组")
		}
		items = append(items, business.NewAPIGroupBinding{PlatformID: platformID, NewAPIGroupID: input.NewAPIGroupID, NewAPIGroupName: input.NewAPIGroupName, Sub2APIGroupID: input.Sub2APIGroupID, SyncRatio: input.SyncRatio})
	}
	if err := s.syncGroupRatios(ctx, *platform, items, localByID); err != nil {
		return nil, err
	}
	if err := s.repository.ReplaceNewAPIGroupBindings(ctx, platformID, items); err != nil {
		return nil, err
	}
	return s.repository.NewAPIGroupBindings(ctx, platformID)
}

func (s *Service) CreateChannel(ctx context.Context, platformID string, input ChannelInput) (map[string]any, error) {
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	input.KeyID = strings.TrimSpace(input.KeyID)
	input.Sub2APIGroupID = strings.TrimSpace(input.Sub2APIGroupID)
	if input.KeyID == "" || input.Sub2APIGroupID == "" {
		return nil, errors.New("Sub2API 分组和密钥 ID 不能为空")
	}
	models := normalizeModels(input.Models)
	if len(models) == 0 {
		return nil, errors.New("渠道至少需要一个模型")
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
		return nil, errors.New("渠道目标不是已登记的 Sub2API 分组")
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
	baseURL, err := configstore.ValidateBaseURL(target.BaseURL)
	if err != nil {
		return nil, errors.New("Sub2API 管理平台地址无效")
	}
	body := map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"type": 59, "name": groupName, "base_url": baseURL, "key": serviceKey,
			"models": strings.Join(models, ","), "group": strings.Join(newAPIGroups, ","), "status": 1,
		},
	}
	payload, err := s.request(ctx, *platform, http.MethodPost, "/api/channel/", body)
	if err != nil {
		return nil, err
	}
	result, _ := payload.(map[string]any)
	if result == nil {
		result = map[string]any{"created": true}
	}
	return result, nil
}

func (s *Service) CreateChannelKey(ctx context.Context, platformID string, input ChannelKeyInput) (ChannelKey, error) {
	if _, err := s.requirePlatform(ctx, platformID); err != nil {
		return ChannelKey{}, err
	}
	input.Sub2APIGroupID = strings.TrimSpace(input.Sub2APIGroupID)
	group, err := s.localGroup(ctx, input.Sub2APIGroupID)
	if err != nil {
		return ChannelKey{}, err
	}
	if s.keys == nil {
		return ChannelKey{}, errors.New("Sub2API 密钥管理服务尚未就绪")
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return ChannelKey{}, err
	}
	created, err := s.keys.CreateKeyWithVerification(
		ctx,
		sub2APIAdminRecord(target),
		newChannelKeyName(group.Name),
		group.ID,
		true,
	)
	if err != nil {
		return ChannelKey{}, err
	}
	if strings.TrimSpace(created.KeyID) == "" || strings.TrimSpace(created.Secret) == "" || created.GroupID != group.ID {
		return ChannelKey{}, errors.New("Sub2API 密钥创建结果不可读")
	}
	return ChannelKey{KeyID: created.KeyID, Name: created.Name, GroupID: created.GroupID}, nil
}

func (s *Service) FetchChannelModels(ctx context.Context, platformID string, input ChannelModelsInput) ([]string, error) {
	if _, err := s.requirePlatform(ctx, platformID); err != nil {
		return nil, err
	}
	input.Sub2APIGroupID = strings.TrimSpace(input.Sub2APIGroupID)
	input.KeyID = strings.TrimSpace(input.KeyID)
	if input.Sub2APIGroupID == "" || input.KeyID == "" {
		return nil, errors.New("请先选择 Sub2API 分组并创建密钥")
	}
	groups, err := s.repository.NewAPILocalGroups(ctx)
	if err != nil {
		return nil, err
	}
	if !localGroupExists(groups, input.Sub2APIGroupID) {
		return nil, errors.New("模型获取目标不是已登记的 Sub2API 分组")
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	serviceKey, err := s.revealChannelKey(ctx, target, input.KeyID, input.Sub2APIGroupID)
	if err != nil {
		return nil, err
	}
	return s.fetchSub2APIModels(ctx, target.BaseURL, serviceKey)
}

func (s *Service) localGroup(ctx context.Context, groupID string) (business.NewAPILocalGroup, error) {
	if groupID == "" {
		return business.NewAPILocalGroup{}, errors.New("请选择 Sub2API 分组")
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
	return business.NewAPILocalGroup{}, errors.New("密钥目标不是已登记的 Sub2API 分组")
}

func (s *Service) revealChannelKey(ctx context.Context, target configstore.TargetSettings, keyID, groupID string) (string, error) {
	if s.keys == nil {
		return "", errors.New("Sub2API 密钥管理服务尚未就绪")
	}
	key, err := s.keys.RevealKey(ctx, sub2APIAdminRecord(target), keyID, groupID)
	if err != nil {
		return "", fmt.Errorf("Sub2API 密钥读取失败：%w", err)
	}
	if strings.TrimSpace(key.Secret) == "" || key.GroupID != groupID {
		return "", errors.New("Sub2API 密钥与所选分组不匹配")
	}
	return strings.TrimSpace(key.Secret), nil
}

func sub2APIAdminRecord(target configstore.TargetSettings) configstore.AuthRecord {
	return configstore.AuthRecord{
		Host: configstore.CanonicalHost(target.BaseURL), BaseURL: target.BaseURL,
		UpstreamType: "sub2api", AuthMode: "custom_headers",
		Headers: map[string]string{"X-API-Key": strings.TrimSpace(target.AdminKey)}, Cookies: map[string]string{},
	}
}

func newChannelKeyName(groupName string) string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("NewAPI-%s-%d", strings.TrimSpace(groupName), time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("NewAPI-%s-%s-%s", strings.TrimSpace(groupName), time.Now().UTC().Format("20060102150405"), hex.EncodeToString(random))
}

func (s *Service) validateNewAPIGroups(ctx context.Context, platform configstore.NewAPIPlatform, values []string) ([]string, error) {
	groups := normalizeModels(values)
	if len(groups) == 0 {
		return nil, errors.New("渠道至少需要一个 New API 分组")
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
			return nil, fmt.Errorf("New API 分组 %s 不存在", group)
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
		return nil, errors.New("Sub2API 管理平台地址无效")
	}
	endpoint, err := url.Parse(normalized)
	if err != nil {
		return nil, errors.New("Sub2API 管理平台地址无效")
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
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return RemoteSnapshot{}, err
	}
	options, err := s.readOptions(ctx, *platform)
	if err != nil {
		return RemoteSnapshot{}, err
	}
	modelRatios, err := decodeDecimalMap(options["ModelRatio"])
	if err != nil {
		return RemoteSnapshot{}, errors.New("New API 当前模型倍率不可读")
	}
	completionRatios, err := decodeDecimalMap(options["CompletionRatio"])
	if err != nil {
		return RemoteSnapshot{}, errors.New("New API 当前补全倍率不可读")
	}
	for _, input := range inputs {
		model := strings.TrimSpace(input.Model)
		if model == "" || !validDecimal(input.InputRatio) || !validDecimal(input.CompletionRatio) {
			return RemoteSnapshot{}, errors.New("模型价格包含空模型或无效倍率")
		}
		modelRatios[model] = strings.TrimSpace(input.InputRatio)
		completionRatios[model] = strings.TrimSpace(input.CompletionRatio)
	}
	if err := s.writeOption(ctx, *platform, "ModelRatio", modelRatios); err != nil {
		return RemoteSnapshot{}, err
	}
	if err := s.writeOption(ctx, *platform, "CompletionRatio", completionRatios); err != nil {
		return RemoteSnapshot{}, err
	}
	return s.Refresh(ctx, platformID)
}

func (s *Service) requirePlatform(ctx context.Context, id string) (*configstore.NewAPIPlatform, error) {
	platform, err := s.private.NewAPIPlatform(ctx, id)
	if err != nil {
		return nil, err
	}
	if platform == nil {
		return nil, errors.New("New API 平台不存在")
	}
	return platform, nil
}

func (s *Service) testConnection(ctx context.Context, item configstore.NewAPIPlatform) error {
	if strings.TrimSpace(item.AdminKey) == "" || strings.TrimSpace(item.UserID) == "" {
		return errors.New("New API Admin Key 和 User ID 不能为空")
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

func (s *Service) request(ctx context.Context, platform configstore.NewAPIPlatform, method, path string, body map[string]any) (any, error) {
	base, err := url.Parse(platform.BaseURL)
	if err != nil {
		return nil, errors.New("New API 平台地址无效")
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
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
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("New API 请求失败（HTTP %d%s）", response.StatusCode, remoteDetail(payload))
	}
	if object, ok := payload.(map[string]any); ok {
		if success, present := object["success"].(bool); present && !success {
			return nil, fmt.Errorf("New API 拒绝操作%s", remoteDetail(payload))
		}
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
		completion := firstDecimal(item, "completion_ratio", "output_ratio")
		if completion == "" {
			completion = "1"
		}
		if model == "" || input == "" || !validDecimal(input) || !validDecimal(completion) {
			continue
		}
		if _, duplicate := seen[model]; duplicate {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, ModelPrice{Model: model, InputRatio: input, CompletionRatio: completion})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result
}

func comparePrices(configured, references []ModelPrice) []PriceDifference {
	configuredByModel := make(map[string]ModelPrice, len(configured))
	referenceByModel := make(map[string]ModelPrice, len(references))
	models := map[string]struct{}{}
	for _, item := range configured {
		configuredByModel[item.Model] = item
		models[item.Model] = struct{}{}
	}
	for _, item := range references {
		referenceByModel[item.Model] = item
		models[item.Model] = struct{}{}
	}
	result := []PriceDifference{}
	for model := range models {
		configuredItem, hasConfigured := configuredByModel[model]
		referenceItem, hasReference := referenceByModel[model]
		kind := ""
		switch {
		case !hasConfigured:
			kind = "missing_in_newapi"
		case !hasReference:
			kind = "only_in_newapi"
		case configuredItem.InputRatio != referenceItem.InputRatio || configuredItem.CompletionRatio != referenceItem.CompletionRatio:
			kind = "ratio_mismatch"
		}
		if kind == "" {
			continue
		}
		difference := PriceDifference{Model: model, Kind: kind}
		if hasConfigured {
			copy := configuredItem
			difference.Configured = &copy
		}
		if hasReference {
			copy := referenceItem
			difference.Reference = &copy
		}
		result = append(result, difference)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Model < result[right].Model })
	return result
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
