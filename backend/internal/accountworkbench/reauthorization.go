package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type profileAuthorization struct {
	uploadSnapshot      json.RawMessage
	uploadMaintenance   *configstore.WorkbenchExecutionMaintenance
	maintenanceRevision int64
	maintenanceOwner    string
	id                  string
	revision            int64
	identity            securityBatchItem
}

func (s *Service) PreviewReauthorization(ctx context.Context, owner string, input ReauthorizationPreviewInput) (OAuthBatchPreview, error) {
	return s.previewReauthorization(ctx, owner, input, false)
}

func (s *Service) previewReauthorization(ctx context.Context, owner string, input ReauthorizationPreviewInput, noSMS bool) (OAuthBatchPreview, error) {
	if owner == "" {
		return OAuthBatchPreview{}, browserlogin.ErrSession
	}
	if !input.FreshLogin {
		return OAuthBatchPreview{}, errors.New("重新授权必须确认使用全新的官方登录会话")
	}
	ctx, err := targetguard.Pin(ctx, s.private)
	if err != nil {
		return OAuthBatchPreview{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return OAuthBatchPreview{}, err
	}
	ids, err := s.reauthorizationIDs(ctx, owner, target, input)
	if err != nil {
		return OAuthBatchPreview{}, err
	}
	if len(ids) == 0 || len(ids) > maxInputItems {
		return OAuthBatchPreview{}, errors.New("单批重新授权需要选择 1 至 500 个账号")
	}
	store, err := s.profileStore()
	if err != nil {
		return OAuthBatchPreview{}, err
	}
	profiles, err := store.WorkbenchLoginProfiles(ctx, executionTargetFingerprint(target))
	if err != nil {
		return OAuthBatchPreview{}, err
	}
	byID := make(map[string]configstore.WorkbenchLoginProfile, len(profiles))
	for _, profile := range profiles {
		byID[profile.AccountID] = profile
	}
	inputs := make([]OAuthLoginInput, 0, len(ids))
	bindings := make([]*profileAuthorization, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !validTemplateSourceID(id) || seen[id] {
			return OAuthBatchPreview{}, errors.New("重新授权账号 ID 无效或重复")
		}
		seen[id] = true
		metadata, found := byID[id]
		if !found {
			return OAuthBatchPreview{}, errors.New("所选账号缺少当前管理目标的登录资料，请先保存资料")
		}
		profile, err := store.WorkbenchLoginProfile(ctx, executionTargetFingerprint(target), metadata.ID)
		if err != nil {
			return OAuthBatchPreview{}, err
		}
		parsed, failures := ParseOAuthLogins("[" + string(profile.Login) + "]")
		if len(failures) != 0 || len(parsed) != 1 || !strings.EqualFold(parsed[0].Email, profile.Email) || parsed[0].WorkspaceID != profile.WorkspaceID {
			return OAuthBatchPreview{}, errors.New("登录资料内容与绑定身份不一致，请重新保存资料")
		}
		if noSMS {
			parsed[0].SMS = nil
		}
		inputs = append(inputs, parsed[0])
		bindings = append(bindings, &profileAuthorization{id: profile.ID, revision: profile.Revision, identity: securityBatchItem{accountID: id, identity: profileIdentity(profile), workspaceID: profile.WorkspaceID}})
	}
	if err := s.validateReauthorizationProfiles(ctx, target, bindings); err != nil {
		return OAuthBatchPreview{}, err
	}
	raw, err := json.Marshal(inputs)
	if err != nil {
		return OAuthBatchPreview{}, errors.New("登录资料无法创建授权预览")
	}
	preview, err := s.PreviewOAuthBatch(targetguard.Expect(ctx, target), owner, OAuthBatchPreviewInput{Content: string(raw)})
	if err != nil || preview.ID == "" {
		return preview, err
	}
	s.batches.mu.Lock()
	defer s.batches.mu.Unlock()
	prepared := s.batches.previews[preview.ID]
	if prepared == nil || prepared.owner != owner || len(prepared.inputs) != len(bindings) {
		return OAuthBatchPreview{}, ErrPreview
	}
	prepared.profiles = bindings
	prepared.view.FreshLogin = true
	for i, binding := range bindings {
		row := &prepared.view.Items[i]
		row.AccountID, row.UserID, row.ProfileID, row.ProfileRevision = binding.identity.accountID, binding.identity.identity.UserID, binding.id, binding.revision
	}
	preview = prepared.view
	preview.Items = append([]OAuthBatchRow(nil), preview.Items...)
	return preview, nil
}

func (s *Service) reauthorizationIDs(ctx context.Context, owner string, target configstore.TargetSettings, input ReauthorizationPreviewInput) ([]string, error) {
	if input.SourceTaskID != "" {
		if input.FailedBatchID != "" {
			return nil, errors.New("历史任务和当前失败批次不能同时作为重新授权来源")
		}
		return s.historicalReauthorizationIDs(ctx, target, input)
	}
	if input.FailedBatchID == "" {
		return input.AccountIDs, nil
	}
	job, err := s.oauthBatch(owner, input.FailedBatchID)
	if err != nil {
		return nil, err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.view.Status == "running" || job.view.Status == "queued" || executionTargetFingerprint(job.target) != executionTargetFingerprint(target) {
		return nil, errors.New("请等待原批次结束，并保持原管理目标后重新授权失败项")
	}
	failed := make(map[string]bool)
	ids := []string{}
	for _, row := range job.view.Items {
		if row.Status != "succeeded" && row.AccountID != "" {
			failed[row.AccountID] = true
			ids = append(ids, row.AccountID)
		}
	}
	if len(input.AccountIDs) == 0 {
		return ids, nil
	}
	for _, id := range input.AccountIDs {
		if !failed[id] {
			return nil, errors.New("选择范围包含原批次以外或已经成功的账号")
		}
	}
	return input.AccountIDs, nil
}

func (s *Service) validateReauthorizationProfiles(ctx context.Context, target configstore.TargetSettings, profiles []*profileAuthorization) error {
	if len(profiles) == 0 {
		return nil
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return err
	}
	store, err := s.profileStore()
	if err != nil {
		return err
	}
	items := make([]securityBatchItem, 0, len(profiles))
	for _, binding := range profiles {
		if binding == nil {
			continue
		}
		if binding.maintenanceRevision > 0 {
			if err := s.validateMaintenanceReauthorization(ctx, target, binding.maintenanceRevision); err != nil {
				return err
			}
			delegated, err := s.currentMaintenanceOwner(ctx, target, binding.maintenanceRevision)
			if err != nil || delegated.owner != binding.maintenanceOwner {
				return errors.New("自动重新授权的登录委托已失效，请重新启用维护")
			}
		}
		profile, err := store.WorkbenchLoginProfile(ctx, executionTargetFingerprint(target), binding.id)
		if err != nil || profile.Revision != binding.revision || profile.AccountID != binding.identity.accountID || profile.UserID != binding.identity.identity.UserID || profile.WorkspaceID != binding.identity.workspaceID || !strings.EqualFold(profile.Email, binding.identity.identity.Email) {
			return configstore.ErrWorkbenchLoginProfile
		}
		items = append(items, binding.identity)
	}
	if len(items) == 0 {
		return nil
	}
	return s.validateSecurityBatch(ctx, target, items)
}
