package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type loginProfileStore interface {
	WorkbenchLoginProfiles(context.Context, string) ([]configstore.WorkbenchLoginProfile, error)
	WorkbenchLoginProfile(context.Context, string, string) (configstore.WorkbenchLoginProfile, error)
	SaveWorkbenchLoginProfile(context.Context, configstore.WorkbenchLoginProfile) (configstore.WorkbenchLoginProfile, error)
	DeleteWorkbenchLoginProfile(context.Context, string, string, int64) error
}

func (s *Service) profileStore() (loginProfileStore, error) {
	store, ok := s.private.(loginProfileStore)
	if !ok {
		return nil, errors.New("登录资料私有存储尚未就绪")
	}
	return store, nil
}

func loginProfileView(value configstore.WorkbenchLoginProfile) LoginProfileView {
	return LoginProfileView{ID: value.ID, AccountID: value.AccountID, UserID: value.UserID, WorkspaceID: value.WorkspaceID, Email: value.Email, Revision: value.Revision, UpdatedAt: value.UpdatedAt, HasPassword: value.HasPassword, HasTOTP: value.HasTOTP, HasProxy: value.HasProxy, MailKind: value.MailKind, SMSProvider: value.SMSProvider}
}

func profileIdentity(value configstore.WorkbenchLoginProfile) browserlogin.SecurityIdentity {
	return browserlogin.SecurityIdentity{Email: value.Email, UserID: value.UserID}
}

func (s *Service) LoginProfiles(ctx context.Context) ([]LoginProfileView, error) {
	ctx, err := targetguard.Pin(ctx, s.private)
	if err != nil {
		return nil, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return nil, err
	}
	store, err := s.profileStore()
	if err != nil {
		return nil, err
	}
	values, err := store.WorkbenchLoginProfiles(ctx, executionTargetFingerprint(target))
	if err != nil {
		return nil, err
	}
	views := make([]LoginProfileView, 0, len(values))
	for _, value := range values {
		views = append(views, loginProfileView(value))
	}
	return views, nil
}

func (s *Service) SaveLoginProfile(ctx context.Context, owner string, input LoginProfileSaveInput) (LoginProfileView, error) {
	if owner == "" {
		return LoginProfileView{}, browserlogin.ErrSession
	}
	if !input.Confirmed {
		return LoginProfileView{}, errors.New("请明确确认将登录密码、验证码收取配置和 2FA 密钥保存至服务器私有资料")
	}
	if !validTemplateSourceID(input.AccountID) || input.Revision < 0 || (input.ID == "" && input.Revision != 0) || (input.ID != "" && input.Revision < 1) {
		return LoginProfileView{}, configstore.ErrWorkbenchLoginProfile
	}
	login, err := validateOAuthBatchLogin(input.Login)
	if err != nil {
		return LoginProfileView{}, err
	}
	store, err := s.profileStore()
	if err != nil {
		return LoginProfileView{}, err
	}
	ctx, err = targetguard.Capture(ctx, s.private)
	if err != nil {
		return LoginProfileView{}, err
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, mutationguard.Account(input.AccountID))
	if err != nil {
		return LoginProfileView{}, err
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return LoginProfileView{}, err
	}
	client, target, err := s.client(guarded)
	if err != nil {
		return LoginProfileView{}, err
	}
	account, err := client.Account(guarded, input.AccountID)
	if err != nil {
		return LoginProfileView{}, publicError(err)
	}
	identity, err := securityBatchSnapshot(account, target)
	if err != nil || identity.workspaceID == "" {
		return LoginProfileView{}, errors.New("保存登录资料需要完整的官方用户 ID、工作区 ID 和邮箱，请先更新账号身份")
	}
	if !strings.EqualFold(login.Email, identity.identity.Email) || (login.WorkspaceID != "" && login.WorkspaceID != identity.workspaceID) {
		return LoginProfileView{}, errors.New("登录资料邮箱或工作区与所选稳定账号不一致")
	}
	login.Email, login.WorkspaceID = identity.identity.Email, identity.workspaceID
	if login.SMS != nil {
		sms := *login.SMS
		sms.Confirmed = false
		login.SMS = &sms
	}
	raw, err := json.Marshal(login)
	if err != nil {
		return LoginProfileView{}, errors.New("登录资料无法保存，请检查输入")
	}
	id := input.ID
	if id == "" {
		id, err = randomID()
		if err != nil {
			return LoginProfileView{}, err
		}
	}
	value := configstore.WorkbenchLoginProfile{ID: id, TargetURL: target.BaseURL, TargetFingerprint: executionTargetFingerprint(target), AccountID: input.AccountID, UserID: identity.identity.UserID, WorkspaceID: identity.workspaceID, Email: identity.identity.Email, Revision: input.Revision, HasPassword: login.Password != "", HasTOTP: login.TOTPSecret != "", Login: raw}
	if login.Mailbox != nil {
		value.MailKind = login.Mailbox.Kind
	}
	value.HasProxy = login.ProxyURL != ""
	if login.SMS != nil {
		value.SMSProvider = login.SMS.Provider
	}
	saved, err := store.SaveWorkbenchLoginProfile(guarded, value)
	if err != nil {
		return LoginProfileView{}, err
	}
	return loginProfileView(saved), nil
}

func (s *Service) DeleteLoginProfile(ctx context.Context, owner, id string, revision int64, confirmed bool) error {
	if owner == "" {
		return browserlogin.ErrSession
	}
	if !confirmed {
		return errors.New("请确认删除服务器保存的登录资料")
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
	store, err := s.profileStore()
	if err != nil {
		return err
	}
	return store.DeleteWorkbenchLoginProfile(guarded, executionTargetFingerprint(target), id, revision)
}
