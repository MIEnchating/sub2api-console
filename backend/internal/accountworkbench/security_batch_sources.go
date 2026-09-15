package accountworkbench

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

type SecuritySourceReference struct {
	OAuthID            string `json:"oauth_id,omitempty"`
	OAuthBatchID       string `json:"oauth_batch_id,omitempty"`
	Index              *int   `json:"index,omitempty"`
	CheckpointID       string `json:"checkpoint_id,omitempty"`
	Revision           int64  `json:"revision,omitempty"`
	CheckpointRevision int64  `json:"checkpoint_revision,omitempty"`
}

func (source SecuritySourceReference) key() (string, error) {
	if source.OAuthID != "" && validExportID(source.OAuthID) && source.OAuthBatchID == "" && source.Index == nil && source.CheckpointID == "" && source.Revision == 0 && source.CheckpointRevision == 0 {
		return "oauth/" + source.OAuthID, nil
	}
	if source.OAuthBatchID != "" && validExportID(source.OAuthBatchID) && source.Index != nil && *source.Index >= 0 && *source.Index < maxInputItems && source.OAuthID == "" && source.CheckpointID == "" && source.Revision == 0 && source.CheckpointRevision == 0 {
		return "batch/" + source.OAuthBatchID + "/" + strconv.Itoa(*source.Index), nil
	}
	if source.CheckpointID != "" && validExportID(source.CheckpointID) && source.Revision > 0 && source.CheckpointRevision >= 0 && source.OAuthID == "" && source.OAuthBatchID == "" && source.Index == nil {
		return "checkpoint/" + source.CheckpointID, nil
	}
	return "", errors.New("安全操作来源参数无效，请重新选择授权结果或暂停检查点")
}

func cloneSecurityReference(source *SecuritySourceReference) *SecuritySourceReference {
	if source == nil {
		return nil
	}
	copy := *source
	if source.Index != nil {
		index := *source.Index
		copy.Index = &index
	}
	return &copy
}

func cloneSecurityRows(rows []SecurityBatchRow) []SecurityBatchRow {
	result := append([]SecurityBatchRow(nil), rows...)
	for i := range result {
		result[i].Source = cloneSecurityReference(result[i].Source)
	}
	return result
}

func (s *Service) securityBatchSourceOrigin(ctx context.Context, owner, id string, index int) (securityOrigin, error) {
	job, err := s.oauthBatch(owner, id)
	if err != nil {
		return securityOrigin{}, err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.view.Status != "authorized" || !job.ready || !time.Now().Before(job.expires) {
		return securityOrigin{}, browserlogin.ErrSession
	}
	scope, err := normalizeWorkbenchScope(job.scope)
	if err != nil {
		return securityOrigin{}, err
	}
	if _, _, err := s.bindWorkbenchScope(ctx, scope, &job.target); err != nil {
		return securityOrigin{}, err
	}
	for _, result := range job.results {
		if result.Index != index {
			continue
		}
		identity, workspace, err := securityCredentialIdentity(result.Credentials, job.target)
		if err != nil {
			return securityOrigin{}, err
		}
		return securityOrigin{scope: scope, target: job.target, identity: identity, workspaceID: workspace, batchSource: job, batchSourceIndex: index}, nil
	}
	return securityOrigin{}, browserlogin.ErrSession
}

func (s *Service) validateSecurityBatchSource(ctx context.Context, value *securitySession) error {
	origin, err := s.securityBatchSourceOrigin(ctx, value.owner, value.view.SourceOAuthBatchID, value.batchSourceIndex)
	if err != nil {
		return err
	}
	if origin.batchSource != value.batchSource || origin.scope != value.view.Scope || origin.workspaceID != value.workspaceID || !sameSecurityIdentity(origin.identity, value.expected) || workbenchScopeFingerprint(origin.scope, origin.target) != workbenchScopeFingerprint(value.view.Scope, value.target) {
		return browserlogin.ErrSecurityIdentity
	}
	return nil
}

func (s *Service) prepareSecurityReference(ctx context.Context, owner string, scope ExportScope, source SecuritySourceReference) (securityOrigin, error) {
	if _, err := source.key(); err != nil {
		return securityOrigin{}, err
	}
	var origin securityOrigin
	var err error
	switch {
	case source.OAuthID != "":
		origin, err = s.securityOAuthOrigin(ctx, owner, source.OAuthID)
	case source.OAuthBatchID != "":
		origin, err = s.securityBatchSourceOrigin(ctx, owner, source.OAuthBatchID, *source.Index)
	default:
		origin, err = s.prepareSecurityCheckpointOrigin(ctx, owner, source.CheckpointID, OAuthCheckpointAction{Scope: scope, Revision: source.Revision, CheckpointRevision: source.CheckpointRevision, Confirmed: true})
	}
	if err != nil {
		return securityOrigin{}, err
	}
	if origin.scope != scope {
		return securityOrigin{}, errors.New("安全操作来源与本批范围不一致，请重新选择")
	}
	return origin, nil
}

func (s *Service) previewSecuritySources(ctx context.Context, owner string, input SecurityBatchPreviewInput) (SecurityBatchPreview, error) {
	scope, err := normalizeWorkbenchScope(input.Scope)
	view := SecurityBatchPreview{Scope: scope, Operation: input.Operation, Items: []SecurityBatchRow{}, Errors: []InputError{}}
	if err != nil {
		return view, err
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return view, err
	}
	items := make([]securityBatchItem, 0, len(input.Sources))
	users := map[string]bool{}
	expires := time.Now().Add(10 * time.Minute)
	for index, source := range input.Sources {
		origin, err := s.prepareSecurityReference(ctx, owner, scope, source)
		if err != nil {
			view.Errors = append(view.Errors, InputError{Index: index, Message: err.Error()})
			continue
		}
		if origin.identity.UserID != "" && users[origin.identity.UserID] {
			view.Errors = append(view.Errors, InputError{Index: index, Message: "同一官方用户不能在一批中重复执行安全设置"})
			continue
		}
		if origin.identity.UserID != "" {
			users[origin.identity.UserID] = true
		}
		if source.CheckpointID != "" && input.ProxyURL != "" {
			view.Errors = append(view.Errors, InputError{Index: index, Message: "检查点必须使用原登录代理，不能覆盖代理"})
			continue
		}
		if origin.source != nil && origin.source.expires.Before(expires) {
			expires = origin.source.expires
		}
		if origin.batchSource != nil && origin.batchSource.expires.Before(expires) {
			expires = origin.batchSource.expires
		}
		if origin.checkpoint != nil && origin.checkpoint.options.Checkpoint.ExpiresAt.Before(expires) {
			expires = origin.checkpoint.options.Checkpoint.ExpiresAt
		}
		items = append(items, securityBatchItem{source: cloneSecurityReference(&source), origin: origin, identity: origin.identity, workspaceID: origin.workspaceID})
		view.Items = append(view.Items, SecurityBatchRow{Index: index, Source: cloneSecurityReference(&source), Email: origin.identity.Email, UserID: origin.identity.UserID, WorkspaceID: origin.workspaceID, Status: "queued", Message: "等待执行安全操作"})
	}
	if len(view.Errors) > 0 {
		view.Items = []SecurityBatchRow{}
		return view, nil
	}
	view.ID, err = randomID()
	if err != nil {
		return view, err
	}
	view.Target, view.ExpiresAt = target.BaseURL, expires.UTC().Format(time.RFC3339Nano)
	prepared := &preparedSecurityBatch{owner: owner, target: target, expires: expires, password: input.Password, proxyURL: input.ProxyURL, items: items, view: view}
	return s.saveSecurityBatchPreview(prepared)
}

func (s *Service) validateSecuritySources(ctx context.Context, owner string, scope ExportScope, items []securityBatchItem) error {
	for _, item := range items {
		origin, err := s.prepareSecurityReference(ctx, owner, scope, *item.source)
		if err != nil {
			return err
		}
		if workbenchScopeFingerprint(origin.scope, origin.target) != workbenchScopeFingerprint(item.origin.scope, item.origin.target) {
			return browserlogin.ErrSecurityIdentity
		}
		if item.source.CheckpointID == "" && (origin.source != item.origin.source || origin.batchSource != item.origin.batchSource || !sameSecurityIdentity(origin.identity, item.identity) || origin.workspaceID != item.workspaceID) {
			return browserlogin.ErrSecurityIdentity
		}
	}
	return nil
}
