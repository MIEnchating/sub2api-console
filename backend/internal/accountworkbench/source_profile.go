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

func (s *Service) sourceProfileStore() (sourceProfileStore, error) {
	store, ok := s.private.(sourceProfileStore)
	if !ok {
		return nil, errors.New("本地登录资料存储尚未就绪")
	}
	return store, nil
}

func sourceProfileView(value configstore.WorkbenchSourceProfile) SourceProfileView {
	return SourceProfileView{ID: value.ID, Scope: ExportScope(value.Scope), UserID: value.UserID, WorkspaceID: value.WorkspaceID, Email: value.Email, Revision: value.Revision, UpdatedAt: value.UpdatedAt, HasPassword: value.HasPassword, HasTOTP: value.HasTOTP, HasProxy: value.HasProxy, MailKind: value.MailKind, SMSProvider: value.SMSProvider}
}

func validateSourceProfileScope(owner string, scope ExportScope) error {
	if owner == "" {
		return browserlogin.ErrSession
	}
	if scope != ScopeLocalExport {
		return errors.New("本地登录资料必须使用独立导出范围")
	}
	return nil
}

func (s *Service) SourceProfiles(ctx context.Context, owner string, scope ExportScope) ([]SourceProfileView, error) {
	if err := validateSourceProfileScope(owner, scope); err != nil {
		return nil, err
	}
	store, err := s.sourceProfileStore()
	if err != nil {
		return nil, err
	}
	profiles, err := store.WorkbenchSourceProfiles(ctx, exportHash(owner), string(scope))
	if err != nil {
		return nil, err
	}
	views := make([]SourceProfileView, 0, len(profiles))
	for _, profile := range profiles {
		views = append(views, sourceProfileView(profile))
	}
	return views, nil
}

func (s *Service) SaveSourceProfile(ctx context.Context, owner string, input SourceProfileSaveInput) (SourceProfileView, error) {
	if err := validateSourceProfileScope(owner, input.Scope); err != nil {
		return SourceProfileView{}, err
	}
	if !input.Confirmed || input.ID == "" && input.Revision != 0 || input.ID != "" && (!validExportID(input.ID) || input.Revision < 1) {
		return SourceProfileView{}, configstore.ErrWorkbenchSourceProfile
	}
	login, err := validateOAuthBatchLogin(input.Login)
	if err != nil {
		return SourceProfileView{}, err
	}
	store, err := s.sourceProfileStore()
	if err != nil {
		return SourceProfileView{}, err
	}
	guarded, release, err := mutationguard.Acquire(ctx, s.repository, "workbench-source-profiles/"+exportHash(owner))
	if err != nil {
		return SourceProfileView{}, err
	}
	defer func() { _ = release() }()
	value := configstore.WorkbenchSourceProfile{ID: input.ID, Revision: input.Revision, Scope: string(input.Scope), OwnerHash: exportHash(owner)}
	if input.ID != "" {
		if input.Source.SourceOAuthID != "" || input.Source.ArtifactID != "" || input.Source.Index != nil || input.SourceRevision != "" {
			return SourceProfileView{}, errors.New("替换资料不能修改已绑定的账号来源")
		}
		value, err = store.WorkbenchSourceProfile(guarded, exportHash(owner), string(input.Scope), input.ID)
		if err != nil || value.Revision != input.Revision {
			return SourceProfileView{}, configstore.ErrWorkbenchSourceProfile
		}
	} else {
		identity, err := s.SourceProfileIdentity(guarded, owner, SourceProfileIdentityInput{Scope: input.Scope, Source: input.Source})
		if err != nil {
			return SourceProfileView{}, err
		}
		if input.SourceRevision == "" || identity.SourceRevision != input.SourceRevision {
			return SourceProfileView{}, configstore.ErrWorkbenchSourceProfile
		}
		value.ID, err = randomID()
		if err != nil {
			return SourceProfileView{}, err
		}
		value.UserID, value.WorkspaceID, value.Email = identity.UserID, identity.WorkspaceID, identity.Email
	}
	if !strings.EqualFold(value.Email, login.Email) || login.WorkspaceID != "" && login.WorkspaceID != value.WorkspaceID {
		return SourceProfileView{}, browserlogin.ErrSecurityIdentity
	}
	login.Email, login.WorkspaceID = value.Email, value.WorkspaceID
	if login.SMS != nil {
		copy := *login.SMS
		copy.Confirmed = false
		login.SMS = &copy
	}
	value.Login, err = json.Marshal(login)
	if err != nil {
		return SourceProfileView{}, err
	}
	value.HasPassword, value.HasTOTP, value.HasProxy = login.Password != "", login.TOTPSecret != "", login.ProxyURL != ""
	value.MailKind, value.SMSProvider = "", ""
	if login.Mailbox != nil {
		value.MailKind = login.Mailbox.Kind
	}
	if login.SMS != nil {
		value.SMSProvider = login.SMS.Provider
	}
	saved, err := store.SaveWorkbenchSourceProfile(guarded, value)
	return sourceProfileView(saved), err
}

func (s *Service) DeleteSourceProfile(ctx context.Context, owner, id string, scope ExportScope, revision int64, confirmed bool) error {
	if err := validateSourceProfileScope(owner, scope); err != nil {
		return err
	}
	if !confirmed || !validExportID(id) || revision < 1 {
		return configstore.ErrWorkbenchSourceProfile
	}
	store, err := s.sourceProfileStore()
	if err != nil {
		return err
	}
	guarded, release, err := mutationguard.Acquire(ctx, s.repository, "workbench-source-profiles/"+exportHash(owner))
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	return store.DeleteWorkbenchSourceProfile(guarded, exportHash(owner), string(scope), id, revision)
}
