package accountworkbench

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func validateSecurityBatchInput(input SecurityBatchPreviewInput) error {
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return err
	}
	if len(input.Sources) > 0 {
		if len(input.AccountIDs) > 0 || len(input.Sources) > maxInputItems {
			return errors.New("请选择 1 至 500 个授权来源，不能同时指定线上账号")
		}
		seen := map[string]bool{}
		for _, source := range input.Sources {
			key, err := source.key()
			if err != nil {
				return err
			}
			if seen[key] {
				return errors.New("安全操作来源重复，请重新选择")
			}
			seen[key] = true
		}
		if err := browserlogin.ValidateProxyURL(input.ProxyURL); err != nil {
			return err
		}
		return validateSecurityOperation(input.Operation, input.Password)
	}
	if scope != ScopeManaged {
		return errors.New("独立安全批次需要选择授权结果或暂停检查点")
	}
	if len(input.AccountIDs) == 0 || len(input.AccountIDs) > maxInputItems {
		return errors.New("单批安全操作需要选择 1 至 500 个账号")
	}
	seen := make(map[string]bool, len(input.AccountIDs))
	for _, id := range input.AccountIDs {
		if !validTemplateSourceID(id) || seen[id] {
			return errors.New("账号 ID 无效或重复，请重新选择稳定账号 ID")
		}
		seen[id] = true
	}
	return validateSecurityInput(SecurityStartInput{AccountID: input.AccountIDs[0], Operation: input.Operation, Password: input.Password, ProxyURL: input.ProxyURL, Confirmed: true})
}

func securityBatchSnapshot(account map[string]any, target configstore.TargetSettings) (securityBatchItem, error) {
	identity, err := securityAccountIdentity(account)
	if err != nil {
		return securityBatchItem{}, err
	}
	accountID := stringValue(account["id"])
	credentials, extra := inputObject(account["credentials"]), inputObject(account["extra"])
	workspace := stringValue(credentials["chatgpt_account_id"])
	if alternate := stringValue(extra["chatgpt_account_id"]); alternate != "" {
		if workspace != "" && workspace != alternate {
			return securityBatchItem{}, browserlogin.ErrSecurityIdentity
		}
		workspace = alternate
	}
	secrets := templateSourceSecrets(account, target.AdminKey)
	if !validTemplateSourceID(accountID) || len(workspace) > 256 || strings.ContainsAny(workspace, "\r\n\x00") || templateTextHasSecret(workspace, secrets) || templateTextHasSecret(identity.Email, secrets) || templateTextHasSecret(identity.UserID, secrets) {
		return securityBatchItem{}, errors.New("账号身份元数据无效，请核对官方邮箱和用户 ID")
	}
	return securityBatchItem{accountID: accountID, identity: identity, workspaceID: workspace}, nil
}

func sameSecurityBatchItem(left, right securityBatchItem) bool {
	return left.accountID == right.accountID && left.workspaceID == right.workspaceID && sameSecurityIdentity(left.identity, right.identity)
}

func (s *Service) PreviewSecurityBatch(ctx context.Context, owner string, input SecurityBatchPreviewInput) (SecurityBatchPreview, error) {
	view := SecurityBatchPreview{Scope: ScopeManaged, Operation: input.Operation, Items: []SecurityBatchRow{}, Errors: []InputError{}}
	if owner == "" {
		return view, browserlogin.ErrSession
	}
	if err := validateSecurityBatchInput(input); err != nil {
		return view, err
	}
	if len(input.Sources) > 0 {
		return s.previewSecuritySources(ctx, owner, input)
	}
	ctx, err := targetguard.Pin(ctx, s.private)
	if err != nil {
		return view, err
	}
	client, target, err := s.client(ctx)
	if err != nil {
		return view, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return view, publicError(err)
	}
	byID := make(map[string]map[string]any, len(accounts))
	for _, account := range accounts {
		id := stringValue(account["id"])
		if byID[id] != nil {
			return view, errors.New("线上账号列表包含重复稳定 ID，请同步并核对账号")
		}
		byID[id] = account
	}
	items := make([]securityBatchItem, 0, len(input.AccountIDs))
	users := make(map[string]bool, len(input.AccountIDs))
	for index, id := range input.AccountIDs {
		account := byID[id]
		if account == nil {
			view.Errors = append(view.Errors, InputError{Index: index, Message: "账号不存在，请同步账号后重新选择"})
			continue
		}
		item, err := securityBatchSnapshot(account, target)
		if err != nil {
			view.Errors = append(view.Errors, InputError{Index: index, Message: err.Error()})
			continue
		}
		if users[item.identity.UserID] {
			view.Errors = append(view.Errors, InputError{Index: index, Message: "本批重复选择了同一官方用户，密码和 2FA 为用户级设置，请只保留一个账号"})
			continue
		}
		users[item.identity.UserID] = true
		items = append(items, item)
		view.Items = append(view.Items, SecurityBatchRow{Index: index, AccountID: id, UserID: item.identity.UserID, WorkspaceID: item.workspaceID, Email: item.identity.Email, Status: "queued", Message: "等待执行安全操作"})
	}
	if len(view.Errors) > 0 {
		view.Items = []SecurityBatchRow{}
		return view, nil
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return SecurityBatchPreview{}, err
	}
	view.ID, err = randomID()
	if err != nil {
		return view, err
	}
	expires := time.Now().Add(10 * time.Minute)
	view.Target, view.ExpiresAt = target.BaseURL, expires.UTC().Format(time.RFC3339Nano)
	prepared := &preparedSecurityBatch{owner: owner, target: target, expires: expires, password: input.Password, items: items, view: view}
	prepared.proxyURL = input.ProxyURL
	return s.saveSecurityBatchPreview(prepared)
}

func (s *Service) saveSecurityBatchPreview(prepared *preparedSecurityBatch) (SecurityBatchPreview, error) {
	owner, view, expires := prepared.owner, prepared.view, prepared.expires
	s.securityBatches.mu.Lock()
	for id, value := range s.securityBatches.previews {
		if value.owner == owner || !time.Now().Before(value.expires) {
			value.password = ""
			value.proxyURL = ""
			delete(s.securityBatches.previews, id)
		}
	}
	if len(s.securityBatches.previews) >= 20 {
		s.securityBatches.mu.Unlock()
		return SecurityBatchPreview{}, errors.New("安全操作预览已满，请稍后重试")
	}
	s.securityBatches.previews[view.ID] = prepared
	s.securityBatches.mu.Unlock()
	time.AfterFunc(time.Until(expires), func() { s.DeleteSecurityBatchPreview(owner, view.ID) })
	view.Items = cloneSecurityRows(view.Items)
	return view, nil
}

func (s *Service) DeleteSecurityBatchPreview(owner, id string) {
	s.securityBatches.mu.Lock()
	defer s.securityBatches.mu.Unlock()
	if value := s.securityBatches.previews[id]; value != nil && value.owner == owner {
		value.password = ""
		value.proxyURL = ""
		delete(s.securityBatches.previews, id)
	}
}

func (s *Service) validateSecurityBatch(ctx context.Context, target configstore.TargetSettings, items []securityBatchItem) error {
	ctx, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private)
	if err != nil {
		return err
	}
	client, err := s.clientFor(target)
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return publicError(err)
	}
	byID := make(map[string]map[string]any, len(accounts))
	for _, account := range accounts {
		id := stringValue(account["id"])
		if byID[id] != nil {
			return browserlogin.ErrSecurityIdentity
		}
		byID[id] = account
	}
	for _, wanted := range items {
		current, err := securityBatchSnapshot(byID[wanted.accountID], target)
		if err != nil || !sameSecurityBatchItem(current, wanted) {
			return errors.New("安全操作范围中的账号身份已变化，请重新预览并确认")
		}
	}
	_, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private)
	return err
}

func securityBatchMessage(operation string) string {
	if strings.EqualFold(operation, "password") {
		return "等待批量设置账号密码"
	}
	return "等待批量启用 TOTP 双重验证"
}
