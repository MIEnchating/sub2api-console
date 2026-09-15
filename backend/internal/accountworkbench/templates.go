package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type TemplateInput struct {
	Name            string                              `json:"name"`
	Priority        int64                               `json:"priority"`
	Revision        int64                               `json:"revision"`
	Match           configstore.WorkbenchTemplateMatch  `json:"match"`
	Config          configstore.WorkbenchTemplateConfig `json:"config"`
	Preferred       *bool                               `json:"preferred,omitempty"`
	SourceAccountID string                              `json:"source_account_id,omitempty"`
	SourceRevision  string                              `json:"source_revision,omitempty"`
}

type TemplateSource struct {
	AccountID      string                              `json:"account_id"`
	AccountName    string                              `json:"account_name"`
	Config         configstore.WorkbenchTemplateConfig `json:"config"`
	Target         string                              `json:"target"`
	SourceRevision string                              `json:"source_revision"`
	SyncedAt       string                              `json:"synced_at"`
	Match          configstore.WorkbenchTemplateMatch  `json:"match"`
	Priority       int64                               `json:"priority"`
}

func (s *Service) Templates(ctx context.Context) ([]configstore.WorkbenchTemplate, error) {
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.private.WorkbenchTemplates(ctx, target.BaseURL)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Config = publicConfig(items[i].Config)
	}
	return items, nil
}

func (s *Service) TemplateFromAccount(ctx context.Context, id string) (TemplateSource, error) {
	if !validTemplateSourceID(id) {
		return TemplateSource{}, errors.New("请选择有效的稳定来源账号 ID")
	}
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return TemplateSource{}, err
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, mutationguard.Account(id))
	if err != nil {
		return TemplateSource{}, err
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return TemplateSource{}, err
	}
	target, err := targetguard.Settings(guarded, s.private)
	if err != nil {
		return TemplateSource{}, err
	}
	source, _, err := s.readTemplateSource(guarded, id, target)
	return source, err
}

func (s *Service) SaveTemplate(ctx context.Context, id string, input TemplateInput) (configstore.WorkbenchTemplate, error) {
	var result configstore.WorkbenchTemplate
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 120 || input.Priority < 0 || input.Priority > 1000000 || input.Revision < 0 {
		return result, errors.New("模板名称、优先级或版本无效")
	}
	if len(input.Match.PlanType) > 100 || len(input.Match.EmailDomain) > 253 || strings.ContainsAny(input.Match.EmailDomain, " /\\\r\n@") {
		return result, errors.New("模板匹配规则无效")
	}
	if id == "" {
		if input.Revision != 0 {
			return result, errors.New("新增模板不能指定旧版本")
		}
		var err error
		id, err = randomID()
		if err != nil {
			return result, err
		}
	} else if input.Revision <= 0 {
		return result, errors.New("修改模板必须提交当前版本")
	}
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return result, err
	}
	target, err := targetguard.Expected(ctx, s.private)
	if err != nil {
		return result, err
	}
	var previous *configstore.WorkbenchTemplate
	if input.Revision > 0 {
		current, err := s.private.WorkbenchTemplate(ctx, target.BaseURL, id)
		if err != nil {
			return result, err
		}
		if current.Revision != input.Revision {
			return result, configstore.ErrWorkbenchTemplateConflict
		}
		previous = &current
	}
	sourceID := input.SourceAccountID
	if sourceID == "" && previous != nil {
		sourceID = previous.SourceAccountID
	}
	verifySource := input.SourceRevision != "" || (sourceID != "" && (previous == nil || sourceID != previous.SourceAccountID))
	if sourceID != "" && !validTemplateSourceID(sourceID) {
		return result, errors.New("来源账号 ID 无效")
	}
	if verifySource && (sourceID == "" || input.SourceRevision == "") {
		return result, errors.New("请先提取并确认来源账号配置预览")
	}
	resources := []string{}
	if verifySource {
		resources = append(resources, mutationguard.Account(sourceID))
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return result, err
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return result, err
	}
	if previous != nil {
		current, err := s.private.WorkbenchTemplate(guarded, target.BaseURL, id)
		if err != nil {
			return result, err
		}
		if current.Revision != input.Revision {
			return result, configstore.ErrWorkbenchTemplateConflict
		}
		previous = &current
	}
	result = configstore.WorkbenchTemplate{ID: id, Revision: input.Revision, Name: input.Name, Priority: input.Priority, TargetURL: target.BaseURL, Match: input.Match, Config: input.Config}
	if previous != nil {
		result.Preferred = previous.Preferred
		result.SourceAccountID, result.SourceName = previous.SourceAccountID, previous.SourceName
		result.SourceRevision, result.SourceSyncedAt = previous.SourceRevision, previous.SourceSyncedAt
	}
	if input.Preferred != nil {
		result.Preferred = *input.Preferred
	}
	secrets := []string{target.AdminKey}
	if verifySource {
		source, sourceSecrets, err := s.readTemplateSource(guarded, sourceID, target)
		if err != nil {
			return result, err
		}
		if source.SourceRevision != input.SourceRevision {
			return result, errors.New("来源账号配置或管理目标已变化，请重新提取预览")
		}
		result.SourceAccountID, result.SourceName = source.AccountID, source.AccountName
		result.SourceRevision, result.SourceSyncedAt = source.SourceRevision, source.SyncedAt
		secrets = append(secrets, sourceSecrets...)
	}
	if err := validatePublicTemplate(input, secrets); err != nil {
		return configstore.WorkbenchTemplate{}, err
	}
	// Applying the same whitelist used by imports validates every submitted field.
	payload, err := ApplyTemplate(InputItem{Credentials: map[string]any{"access_token": "validation-only"}}, &result)
	if err != nil {
		return result, err
	}
	result.Config, err = ExtractTemplate(payload)
	if err != nil {
		return result, err
	}
	if _, err := targetguard.Pin(guarded, s.private); err != nil {
		return configstore.WorkbenchTemplate{}, err
	}
	if err := s.private.SaveWorkbenchTemplate(guarded, result); err != nil {
		return result, err
	}
	result.Revision++
	result.Config = publicConfig(result.Config)
	return result, nil
}

func (s *Service) DeleteTemplate(ctx context.Context, id string, revision int64) error {
	if id == "" || revision <= 0 {
		return errors.New("模板 ID 或版本无效")
	}
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return err
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return err
	}
	target, err := targetguard.Settings(guarded, s.private)
	if err != nil {
		return err
	}
	return s.private.DeleteWorkbenchTemplate(guarded, target.BaseURL, id, revision)
}

func (s *Service) SetPreferredTemplate(ctx context.Context, id string, revision int64, preferred bool) (configstore.WorkbenchTemplate, error) {
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return configstore.WorkbenchTemplate{}, err
	}
	target, err := targetguard.Expected(ctx, s.private)
	if err != nil {
		return configstore.WorkbenchTemplate{}, err
	}
	current, err := s.private.WorkbenchTemplate(ctx, target.BaseURL, id)
	if err != nil {
		return configstore.WorkbenchTemplate{}, err
	}
	if current.Revision != revision {
		return configstore.WorkbenchTemplate{}, configstore.ErrWorkbenchTemplateConflict
	}
	return s.SaveTemplate(ctx, id, TemplateInput{Name: current.Name, Priority: current.Priority, Revision: revision, Match: current.Match, Config: current.Config, Preferred: &preferred})
}

func publicConfig(config configstore.WorkbenchTemplateConfig) configstore.WorkbenchTemplateConfig {
	result := make(configstore.WorkbenchTemplateConfig, len(config))
	for key, raw := range config {
		result[key] = append(json.RawMessage(nil), raw...)
	}
	for _, key := range []string{"rate_multiplier", "load_factor", "proxy_id"} {
		raw, ok := result[key]
		if !ok || string(raw) == "null" {
			continue
		}
		var value any
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if decoder.Decode(&value) == nil {
			result[key], _ = json.Marshal(stringValue(value))
		}
	}
	result["group_ids"], _ = json.Marshal(configIDs(config["group_ids"]))
	return result
}
