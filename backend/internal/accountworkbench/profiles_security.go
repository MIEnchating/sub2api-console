package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"golang.org/x/sys/unix"
)

func (s *Service) ApplySecurityToLoginProfile(ctx context.Context, owner string, input LoginProfileSecurityInput) (LoginProfileView, error) {
	if !input.Confirmed {
		return LoginProfileView{}, errors.New("请确认使用已完成的账号安全结果更新保存的登录资料")
	}
	view, target, identity, storage, err := s.profileSecurityResult(owner, input)
	if err != nil {
		return LoginProfileView{}, err
	}
	if view.Status != "succeeded" || view.ArtifactID == "" {
		return LoginProfileView{}, errors.New("仅已确认成功且具有私有结果的安全操作可更新登录资料")
	}
	if view.SourceOAuthID != "" || view.SourceCheckpointID != "" || !validTemplateSourceID(view.AccountID) {
		return LoginProfileView{}, errors.New("该安全结果未绑定线上账号，不能更新线上登录资料")
	}
	ctx, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private)
	if err != nil {
		return LoginProfileView{}, err
	}
	store, err := s.profileStore()
	if err != nil {
		return LoginProfileView{}, err
	}
	profile, err := store.WorkbenchLoginProfile(ctx, executionTargetFingerprint(target), input.ProfileID)
	if err != nil {
		return LoginProfileView{}, err
	}
	if profile.Revision != input.Revision || profile.AccountID != view.AccountID || !sameSecurityIdentity(profileIdentity(profile), identity) {
		return LoginProfileView{}, configstore.ErrWorkbenchLoginProfile
	}
	artifact, err := readSecurityProfileArtifact(storage, view.ArtifactID)
	if err != nil {
		return LoginProfileView{}, err
	}
	if artifact.SourceOAuthID != "" || artifact.SourceOAuthBatchID != "" || artifact.SourceCheckpointID != "" || artifact.Scope == ScopeLocalExport || artifact.AccountID != profile.AccountID || artifact.UserID != profile.UserID || artifact.Operation != view.Operation {
		return LoginProfileView{}, configstore.ErrWorkbenchLoginProfile
	}
	items, failures := ParseOAuthLogins("[" + string(profile.Login) + "]")
	if len(failures) != 0 || len(items) != 1 {
		return LoginProfileView{}, errors.New("保存的登录资料已损坏，请重新填写")
	}
	login := items[0]
	switch view.Operation {
	case "password":
		if artifact.Password == "" {
			return LoginProfileView{}, ErrSecurityStorage
		}
		login.Password = artifact.Password
	case "totp":
		if artifact.Secret == "" {
			return LoginProfileView{}, ErrSecurityStorage
		}
		login.TOTPSecret = artifact.Secret
	default:
		return LoginProfileView{}, ErrSecurityStorage
	}
	return s.SaveLoginProfile(ctx, owner, LoginProfileSaveInput{ID: profile.ID, AccountID: profile.AccountID, Revision: profile.Revision, Confirmed: true, Login: login})
}

func (s *Service) profileSecurityResult(owner string, input LoginProfileSecurityInput) (SecurityView, configstore.TargetSettings, browserlogin.SecurityIdentity, *securityStorage, error) {
	fail := func(err error) (SecurityView, configstore.TargetSettings, browserlogin.SecurityIdentity, *securityStorage, error) {
		return SecurityView{}, configstore.TargetSettings{}, browserlogin.SecurityIdentity{}, nil, err
	}
	if input.BatchID == "" {
		if input.AccountID != "" {
			return fail(errors.New("单账号安全结果不接受批次账号参数"))
		}
		security, err := s.securitySession(owner, input.SecurityID)
		if err != nil {
			return fail(err)
		}
		security.mu.Lock()
		defer security.mu.Unlock()
		return security.view, security.target, security.expected, security.storage, nil
	}
	if input.SecurityID != "" || !validTemplateSourceID(input.AccountID) {
		return fail(errors.New("批次安全结果必须提供批次 ID 和稳定账号 ID"))
	}
	s.securityBatches.mu.Lock()
	job := s.securityBatches.active[input.BatchID]
	s.securityBatches.mu.Unlock()
	if job == nil || job.owner != owner || !time.Now().Before(job.expires) {
		return fail(browserlogin.ErrSession)
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	for i, row := range job.view.Items {
		if row.AccountID != input.AccountID {
			continue
		}
		s.securityMu.Lock()
		storage := s.securityStorage
		s.securityMu.Unlock()
		if storage == nil {
			return fail(ErrSecurityStorage)
		}
		return SecurityView{AccountID: row.AccountID, Operation: job.view.Operation, Email: row.Email, Status: row.Status, ArtifactID: row.ArtifactID}, job.target, job.items[i].identity, storage, nil
	}
	return fail(browserlogin.ErrSession)
}

func readSecurityProfileArtifact(storage *securityStorage, id string) (privateSecurityArtifact, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	if storage.closed {
		return privateSecurityArtifact{}, ErrSecurityStorage
	}
	fd, err := unix.Openat(int(storage.directory.Fd()), id+".json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return privateSecurityArtifact{}, ErrSecurityStorage
	}
	file := os.NewFile(uintptr(fd), id)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return privateSecurityArtifact{}, ErrSecurityStorage
	}
	raw, err := io.ReadAll(io.LimitReader(file, 64<<10+1))
	var artifact privateSecurityArtifact
	if err != nil || len(raw) > 64<<10 || json.Unmarshal(raw, &artifact) != nil || artifact.Version != 1 {
		return privateSecurityArtifact{}, ErrSecurityStorage
	}
	return artifact, nil
}
