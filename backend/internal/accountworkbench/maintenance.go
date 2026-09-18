package accountworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type MaintenanceSettings struct {
	Enabled          bool     `json:"enabled"`
	IntervalMinutes  int      `json:"interval_minutes"`
	CooldownMinutes  int      `json:"cooldown_minutes"`
	CheckAfterRepair bool     `json:"check_after_repair"`
	GroupIDs         []string `json:"group_ids"`
}
type MaintenanceResult struct {
	AccountID      string         `json:"account_id"`
	Email          string         `json:"email"`
	Action         string         `json:"action"`
	Status         string         `json:"status"`
	Reason         string         `json:"reason"`
	NextEligibleAt *time.Time     `json:"next_eligible_at,omitempty"`
	Check          map[string]any `json:"check,omitempty"`
}
type Maintenance struct {
	MaintenanceSettings
	Revision    int64               `json:"revision"`
	Running     bool                `json:"running"`
	TaskID      string              `json:"task_id"`
	LastCheckAt *time.Time          `json:"last_check_at"`
	NextCheckAt *time.Time          `json:"next_check_at"`
	Message     string              `json:"message"`
	Results     []MaintenanceResult `json:"results"`
}
type MaintenancePreview struct {
	ID        string              `json:"id"`
	Revision  int64               `json:"revision"`
	ExpiresAt time.Time           `json:"expires_at"`
	Settings  MaintenanceSettings `json:"settings"`
	Accounts  []Account           `json:"accounts"`
}
type privateMaintenancePreview struct {
	Public         MaintenancePreview         `json:"public"`
	Owner          string                     `json:"owner"`
	Target         configstore.TargetSettings `json:"target"`
	ConfigRevision int64                      `json:"config_revision"`
	ScopeVersion   string                     `json:"scope_version"`
}
type maintenanceAttempt struct {
	Phase                      string         `json:"phase"`
	AccountVersion             string         `json:"account_version"`
	Credentials                map[string]any `json:"credentials,omitempty"`
	ExpiresAt                  time.Time      `json:"expires_at"`
	Workspace                  string         `json:"workspace"`
	User                       string         `json:"user"`
	SourceRunID                string         `json:"source_run_id,omitempty"`
	SourceRevision             int64          `json:"source_revision,omitempty"`
	ConfigurationVersion       string         `json:"configuration_version"`
	CredentialVersion          string         `json:"credential_version"`
	ProxyURL                   string         `json:"proxy_url,omitempty"`
	VerifiedCredentialsVersion string         `json:"verified_credentials_version,omitempty"`
}
type privateMaintenance struct {
	Public   Maintenance                    `json:"public"`
	Owner    string                         `json:"owner"`
	Target   configstore.TargetSettings     `json:"target"`
	States   map[string]MaintenanceResult   `json:"states"`
	Attempts map[string]*maintenanceAttempt `json:"attempts"`
}

func defaultMaintenance() privateMaintenance {
	return privateMaintenance{Public: Maintenance{MaintenanceSettings: MaintenanceSettings{IntervalMinutes: 5, CooldownMinutes: 10, CheckAfterRepair: true, GroupIDs: []string{}}, Results: []MaintenanceResult{}}, States: map[string]MaintenanceResult{}, Attempts: map[string]*maintenanceAttempt{}}
}
func (s *Service) readMaintenance(ctx context.Context, target configstore.TargetSettings) (privateMaintenance, error) {
	value := defaultMaintenance()
	raw, revision, err := s.private.WorkbenchDocument(ctx, "maintenance:"+targetKey(target))
	if err != nil {
		return value, err
	}
	if len(raw) > 0 {
		if err = decodePrivate(raw, &value); err != nil {
			return value, errors.New("自动维护配置无法读取")
		}
	}
	value.Public.Revision = revision
	return value, nil
}
func decodePrivate(raw json.RawMessage, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(value)
}
func (s *Service) saveMaintenance(ctx context.Context, value *privateMaintenance) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	revision, err := s.private.SaveWorkbenchDocument(ctx, "maintenance:"+targetKey(value.Target), value.Public.Revision, raw)
	if err == nil {
		value.Public.Revision = revision
	}
	return err
}
func (s *Service) Maintenance(ctx context.Context, owner string) (Maintenance, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return Maintenance{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return Maintenance{}, err
	}
	value, err := s.readMaintenance(ctx, target)
	return value.Public, err
}
func validateMaintenance(input MaintenanceSettings) error {
	if input.IntervalMinutes < 1 || input.IntervalMinutes > 1440 || input.CooldownMinutes < 0 || input.CooldownMinutes > 1440 || len(input.GroupIDs) > 500 {
		return errors.New("检查间隔须为 1–1440 分钟，冷却须为 0–1440 分钟")
	}
	seen := map[string]bool{}
	for _, id := range input.GroupIDs {
		v, err := strconv.ParseUint(id, 10, 63)
		if err != nil || v == 0 || seen[id] {
			return errors.New("维护分组 ID 无效或重复")
		}
		seen[id] = true
	}
	return nil
}
func (s *Service) maintenanceScope(ctx context.Context, target configstore.TargetSettings, settings MaintenanceSettings) ([]Account, string, error) {
	client, err := s.client(target)
	if err != nil {
		return nil, "", err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, "", errors.New("维护范围读取失败，请检查站点连接")
	}
	result := []Account{}
	versions := map[string]string{}
	for _, account := range accounts {
		if !maintenanceEligible(account, settings.GroupIDs) {
			continue
		}
		id := text(account["id"])
		if _, err := strconv.ParseUint(id, 10, 63); err != nil || id == "0" || versions[id] != "" {
			return nil, "", errors.New("维护账号缺少有效且唯一的稳定 ID")
		}
		result = append(result, publicAccount(account))
		versions[id] = accountVersion(account)
	}
	if len(result) > 500 {
		return nil, "", errors.New("维护范围超过 500 个账号，请按分组缩小范围")
	}
	if _, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return nil, "", err
	}
	return result, digest(versions), nil
}
func (s *Service) PreviewMaintenance(ctx context.Context, owner string, input MaintenanceSettings) (MaintenancePreview, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return MaintenancePreview{}, err
	}
	if err := validateMaintenance(input); err != nil {
		return MaintenancePreview{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return MaintenancePreview{}, err
	}
	current, err := s.readMaintenance(ctx, target)
	if err != nil {
		return MaintenancePreview{}, err
	}
	accounts, version, err := s.maintenanceScope(ctx, target, input)
	if err != nil {
		return MaintenancePreview{}, err
	}
	preview := MaintenancePreview{ID: newID(), Settings: input, Accounts: accounts, ExpiresAt: time.Now().UTC().Add(previewLifetime)}
	value := privateMaintenancePreview{Public: preview, Owner: owner, Target: target, ConfigRevision: current.Public.Revision, ScopeVersion: version}
	key := "maintenance-preview:" + owner
	_, revision, err := s.private.WorkbenchDocument(ctx, key)
	if err != nil {
		return MaintenancePreview{}, err
	}
	raw, _ := json.Marshal(value)
	preview.Revision, err = s.private.SaveWorkbenchDocument(ctx, key, revision, raw)
	return preview, err
}
func (s *Service) ConfigureMaintenance(ctx context.Context, owner string, input RunConfirmation) (Maintenance, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.checkOwner(ctx, owner); err != nil {
		return Maintenance{}, err
	}
	raw, revision, err := s.private.WorkbenchDocument(ctx, "maintenance-preview:"+owner)
	if err != nil {
		return Maintenance{}, err
	}
	var preview privateMaintenancePreview
	if decodePrivate(raw, &preview) != nil || preview.Owner != owner || revision != input.Revision || preview.Public.ID != input.ID || !preview.Public.ExpiresAt.After(time.Now().UTC()) {
		return Maintenance{}, errors.New("维护预览已失效，请重新预览")
	}
	key := "maintenance:" + targetKey(preview.Target)
	s.activeMu.Lock()
	active := s.active[key] != nil
	s.activeMu.Unlock()
	if active {
		return Maintenance{}, errors.New("维护正在运行，请先停止或等待结束")
	}
	value, err := s.readMaintenance(ctx, preview.Target)
	if err != nil {
		return Maintenance{}, err
	}
	if value.Public.Revision != preview.ConfigRevision {
		return Maintenance{}, configstore.ErrWorkbenchVersion
	}
	_, scope, err := s.maintenanceScope(ctx, preview.Target, preview.Public.Settings)
	if err != nil {
		return Maintenance{}, err
	}
	if scope != preview.ScopeVersion {
		return Maintenance{}, errors.New("维护账号范围或配置已变化，请重新预览")
	}
	value.Owner = owner
	value.Target = preview.Target
	value.Public.MaintenanceSettings = preview.Public.Settings
	value.States = map[string]MaintenanceResult{}
	value.Public.Results = []MaintenanceResult{}
	value.Public.Message = "维护设置已保存"
	value.Public.NextCheckAt = nil
	if value.Public.Enabled {
		next := time.Now().UTC().Add(time.Duration(value.Public.IntervalMinutes) * time.Minute)
		value.Public.NextCheckAt = &next
	}
	if err = s.saveMaintenance(ctx, &value); err != nil {
		return Maintenance{}, err
	}
	_ = s.private.DeleteWorkbenchDocument(ctx, "maintenance-preview:"+owner, revision)
	return value.Public, nil
}
