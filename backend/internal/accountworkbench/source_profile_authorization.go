package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

type sourceProfileAuthorization struct {
	view SourceProfileView
}

func (s *Service) selectedSourceProfiles(ctx context.Context, owner string, input SourceProfileSelectionInput) ([]configstore.WorkbenchSourceProfile, error) {
	if err := validateSourceProfileScope(owner, input.Scope); err != nil {
		return nil, err
	}
	if len(input.Items) == 0 || len(input.Items) > maxInputItems {
		return nil, errors.New("请选择 1 至 500 份本地登录资料")
	}
	store, err := s.sourceProfileStore()
	if err != nil {
		return nil, err
	}
	profiles := make([]configstore.WorkbenchSourceProfile, 0, len(input.Items))
	seen := make(map[string]bool, len(input.Items))
	for _, item := range input.Items {
		if !validExportID(item.ID) || item.Revision < 1 || seen[item.ID] {
			return nil, configstore.ErrWorkbenchSourceProfile
		}
		seen[item.ID] = true
		profile, err := store.WorkbenchSourceProfile(ctx, exportHash(owner), string(input.Scope), item.ID)
		if err != nil || profile.Revision != item.Revision {
			return nil, configstore.ErrWorkbenchSourceProfile
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (s *Service) PreviewSourceProfileAuthorization(ctx context.Context, owner string, input SourceProfileSelectionInput) (OAuthBatchPreview, error) {
	if !input.FreshLogin {
		return OAuthBatchPreview{}, errors.New("请确认使用本地登录资料发起全新的官方授权")
	}
	profiles, err := s.selectedSourceProfiles(ctx, owner, input)
	if err != nil {
		return OAuthBatchPreview{}, err
	}
	if err := s.validateSourceProfileRetry(owner, input); err != nil {
		return OAuthBatchPreview{}, err
	}
	logins := make([]OAuthLoginInput, 0, len(profiles))
	bindings := make([]sourceProfileAuthorization, 0, len(profiles))
	for _, profile := range profiles {
		inputs, failures := ParseOAuthLogins("[" + string(profile.Login) + "]")
		if len(failures) != 0 || len(inputs) != 1 || !strings.EqualFold(inputs[0].Email, profile.Email) || inputs[0].WorkspaceID != profile.WorkspaceID {
			return OAuthBatchPreview{}, configstore.ErrWorkbenchSourceProfile
		}
		logins = append(logins, inputs[0])
		bindings = append(bindings, sourceProfileAuthorization{view: sourceProfileView(profile)})
	}
	raw, err := json.Marshal(logins)
	if err != nil {
		return OAuthBatchPreview{}, err
	}
	preview, err := s.PreviewOAuthBatch(ctx, owner, OAuthBatchPreviewInput{Scope: ScopeLocalExport, Content: string(raw)})
	if err != nil || preview.ID == "" {
		return preview, err
	}
	s.batches.mu.Lock()
	defer s.batches.mu.Unlock()
	prepared := s.batches.previews[preview.ID]
	if prepared == nil || prepared.owner != owner {
		return OAuthBatchPreview{}, ErrPreview
	}
	prepared.sourceProfiles, prepared.view.FreshLogin = bindings, true
	for index, binding := range bindings {
		row := &prepared.view.Items[index]
		row.UserID, row.ProfileID, row.ProfileRevision = binding.view.UserID, binding.view.ID, binding.view.Revision
	}
	view := prepared.view
	view.Items = append([]OAuthBatchRow(nil), view.Items...)
	return view, nil
}

func (s *Service) validateSourceProfileRetry(owner string, input SourceProfileSelectionInput) error {
	if input.FailedBatchID == "" {
		return nil
	}
	job, err := s.oauthBatch(owner, input.FailedBatchID)
	if err != nil {
		return err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.scope != ScopeLocalExport || job.view.Status == "running" || job.view.Status == "queued" || len(job.sourceProfiles) == 0 {
		return configstore.ErrWorkbenchSourceProfile
	}
	failed := make(map[string]bool)
	for _, row := range job.view.Items {
		if row.Status != "succeeded" {
			failed[row.ProfileID] = true
		}
	}
	for _, item := range input.Items {
		if !failed[item.ID] {
			return errors.New("重新登录范围包含原批次以外或已成功的资料")
		}
	}
	return nil
}

func (s *Service) validateSourceProfileAuthorizations(ctx context.Context, owner string, bindings []sourceProfileAuthorization) error {
	if len(bindings) == 0 {
		return nil
	}
	store, err := s.sourceProfileStore()
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		current, err := store.WorkbenchSourceProfile(ctx, exportHash(owner), string(ScopeLocalExport), binding.view.ID)
		if err != nil || sourceProfileView(current) != binding.view {
			return configstore.ErrWorkbenchSourceProfile
		}
	}
	return nil
}

func (s *Service) sourceProfileCallbacks(job *oauthBatch, index int, previous oauthCallbacks) oauthCallbacks {
	if index >= len(job.sourceProfiles) {
		return previous
	}
	binding := job.sourceProfiles[index]
	withProfile := func(ctx context.Context, operation func(context.Context) error) error {
		guarded, release, err := mutationguard.Acquire(ctx, s.repository, "workbench-source-profiles/"+exportHash(job.owner))
		if err != nil {
			return err
		}
		defer func() { _ = release() }()
		if err := s.validateSourceProfileAuthorizations(guarded, job.owner, []sourceProfileAuthorization{binding}); err != nil {
			return err
		}
		return operation(guarded)
	}
	return oauthCallbacks{
		beforeLaunch: func(ctx context.Context, view OAuthView) error {
			return withProfile(ctx, func(ctx context.Context) error {
				if previous.beforeLaunch != nil {
					return previous.beforeLaunch(ctx, view)
				}
				return nil
			})
		},
		onAuthorized: func(ctx context.Context, credentials map[string]any) error {
			return withProfile(ctx, func(ctx context.Context) error {
				identity := inputIdentity(credentials)
				if identity.user != binding.view.UserID || identity.workspace != binding.view.WorkspaceID || !strings.EqualFold(stringValue(credentials["email"]), binding.view.Email) {
					return browserlogin.ErrSecurityIdentity
				}
				if previous.onAuthorized != nil {
					return previous.onAuthorized(ctx, credentials)
				}
				return nil
			})
		},
	}
}
