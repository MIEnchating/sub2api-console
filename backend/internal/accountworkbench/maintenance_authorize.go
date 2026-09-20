package accountworkbench

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func (s *Service) savedMaintenanceLogin(ctx context.Context, value *privateMaintenance, account map[string]any) (storedInput, string, int64, time.Time, error) {
	records, err := s.private.WorkbenchDocuments(ctx, "run:"+value.Owner+":", 501)
	if err != nil {
		return storedInput{}, "", 0, time.Time{}, err
	}
	for _, record := range records {
		run, err := decodeRun(record.Payload)
		if err != nil {
			return storedInput{}, "", 0, time.Time{}, err
		}
		if run.Public.Action != "import" || targetKey(run.Target) != targetKey(value.Target) || !run.Public.ExpiresAt.After(time.Now().UTC()) {
			continue
		}
		for i, row := range run.Public.Items {
			if i >= len(run.Items) || row.AccountID != text(account["id"]) || row.Status != "completed" || run.Items[i].LoginSource == "" {
				continue
			}
			input := run.Items[i]
			if !sameAccountIdentity(account, input.Credentials) {
				continue
			}
			return input, run.Public.ID, record.Revision, run.Public.ExpiresAt, nil
		}
	}
	return storedInput{}, "", 0, time.Time{}, errors.New("没有当前会话授权使用的有效登录资料，请重新导入账号")
}
func (s *Service) reauthorizeMaintenance(ctx context.Context, value *privateMaintenance, account map[string]any, attempt *maintenanceAttempt) error {
	input, sourceID, sourceRevision, expires, err := s.savedMaintenanceLogin(ctx, value, account)
	if err != nil {
		return err
	}
	if expires.Before(attempt.ExpiresAt) {
		attempt.ExpiresAt = expires
	}
	attempt.Phase = "reauthorizing"
	attempt.SourceRunID = sourceID
	attempt.SourceRevision = sourceRevision
	attempt.ProxyURL = input.ProxyURL
	attempt.AccountVersion = accountVersion(account)
	if err = s.persistMaintenance(value); err != nil {
		return err
	}
	library, err := s.readTemplates(ctx, value.Target)
	if err != nil {
		return err
	}
	input.Item.Kind = "login"
	input.Credentials = map[string]any{}
	now := time.Now().UTC()
	login := privateRun{Owner: value.Owner, Target: value.Target, Automatic: true, TemplateRevision: library.Revision, Settings: PreviewInput{Model: "gpt-5.6-sol"}, Items: []storedInput{input}, Exports: make([]map[string]any, 1), AccountVersions: map[string]string{}, Phases: map[string]string{}, Public: Run{ID: newID(), Action: "maintenance", Status: "running", CreatedAt: now, ExpiresAt: attempt.ExpiresAt, Items: []RunItem{{InputItem: input.Item, Status: "authorizing", AccountID: text(account["id"])}}}}
	login.Scope = "managed"
	if err = s.persistRun(&login); err != nil {
		return err
	}
	active := &activeRun{done: make(chan struct{}), owner: value.Owner, taskID: value.Public.TaskID}
	s.activeMu.Lock()
	s.active[login.Public.ID] = active
	s.activeMu.Unlock()
	defer func() { s.activeMu.Lock(); delete(s.active, login.Public.ID); close(active.done); s.activeMu.Unlock() }()
	authorizeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	credentials, err := s.authorizeItem(authorizeCtx, &login, 0, active, input.ProxyURL)
	if len(credentials) > 0 {
		login.Items[0].Credentials = credentials
		login.Phases[input.Item.ID] = "materialized"
		if saveErr := s.persistRun(&login); saveErr != nil {
			return saveErr
		}
		attempt.Credentials = credentials
		attempt.Phase = "pending_identity"
		if saveErr := s.persistMaintenance(value); saveErr != nil {
			return saveErr
		}
	}
	if err != nil {
		login.Public.Status = "interrupted"
		_ = s.persistRun(&login)
		return err
	}
	login.Public.Status = "completed"
	if err = s.persistRun(&login); err != nil {
		return err
	}
	return s.verifyMaintenanceAuthorization(ctx, value, account, attempt)
}

func (s *Service) verifyMaintenanceAuthorization(ctx context.Context, value *privateMaintenance, account map[string]any, attempt *maintenanceAttempt) error {
	if !attempt.ExpiresAt.After(time.Now().UTC()) {
		return ErrRun
	}
	if err := s.maintenanceAllowed(ctx, value); err != nil {
		return err
	}
	credentials := attempt.Credentials
	identity, err := s.VerifyCredential(ctx, credentials, attempt.ProxyURL)
	if err != nil || identity.WorkspaceID != attempt.Workspace || identity.UserID != attempt.User || !equalEmail(identity.Email, publicAccount(account).Email) {
		return ErrIdentity
	}
	credentials["chatgpt_account_id"] = identity.WorkspaceID
	credentials["chatgpt_user_id"] = identity.UserID
	credentials["email"] = identity.Email
	merged := maps.Clone(object(account["credentials"]))
	maps.Copy(merged, credentials)
	attempt.Credentials = merged
	attempt.Phase = "pending_upload"
	return s.persistMaintenance(value)
}
func (s *Service) uploadMaintenanceAuthorization(ctx context.Context, value *privateMaintenance, client *adminclient.Client, account map[string]any, attempt *maintenanceAttempt) (map[string]any, error) {
	if !attempt.ExpiresAt.After(time.Now().UTC()) || len(attempt.Credentials) == 0 {
		return nil, ErrRun
	}
	if err := s.maintenanceAllowed(ctx, value); err != nil {
		return nil, err
	}
	id := text(account["id"])
	current, err := client.Account(ctx, id)
	if err != nil {
		return nil, errors.New("授权上传前账号读取失败")
	}
	if !sameAccountIdentity(current, attempt.Credentials) || maintenanceConfiguration(current) != attempt.ConfigurationVersion {
		return nil, errors.New("授权上传目标已变化")
	}
	body := map[string]any{"credentials": attempt.Credentials}
	if attempt.Phase == "uploading" {
		if err = verifyApplied(current, body); err != nil {
			return nil, errors.New("上次授权上传尚未核对，未重复写入")
		}
	} else {
		if accountVersion(current) != attempt.AccountVersion {
			return nil, errors.New("授权期间账号已被修改，未上传")
		}
		_, revision, err := s.private.WorkbenchDocument(ctx, runKey(value.Owner, attempt.SourceRunID))
		if err != nil || revision != attempt.SourceRevision {
			return nil, errors.New("登录资料已变化，未上传")
		}
		attempt.Phase = "uploading"
		if err = s.persistMaintenance(value); err != nil {
			return nil, err
		}
		ctx = adminclient.WithMutationAuthorization(ctx, func(ctx context.Context) error { return s.maintenanceAllowed(ctx, value) })
		if _, err = client.UpdateAccount(ctx, id, body); err != nil {
			return nil, errors.New("授权上传结果未确认")
		}
		current, err = client.Account(ctx, id)
		if err != nil {
			return nil, errors.New("授权上传后读取失败")
		}
		if err = verifyApplied(current, body); err != nil {
			return nil, err
		}
	}
	attempt.AccountVersion = accountVersion(current)
	attempt.Phase = "uploaded"
	attempt.Credentials = nil
	if err = s.persistMaintenance(value); err != nil {
		return nil, err
	}
	return current, nil
}
