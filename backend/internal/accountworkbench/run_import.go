package accountworkbench

import (
	"context"
	"errors"
	"maps"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func (s *Service) importItem(ctx context.Context, value *privateRun, index int, payload map[string]any) error {
	row := &value.Public.Items[index]
	if err := s.executionAllowed(ctx, value); err != nil {
		return errors.New("目标或模板已变化，未执行导入")
	}
	client, err := s.client(value.Target)
	if err != nil {
		return errors.New("目标配置无效")
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return errors.New("线上账号读取失败，未执行导入")
	}
	item := value.Items[index].Item
	item.Credentials = value.Items[index].Credentials
	existing, err := FindExistingAccount(item, accounts)
	if err != nil {
		return err
	}
	resources := []string{mutationguard.AccountCatalog()}
	if existing != nil {
		resources = append(resources, mutationguard.Account(text(existing["id"])))
	}
	ctx, release, err := targetguard.Acquire(targetguard.Expect(ctx, value.Target), s.private, resources...)
	if err != nil {
		return errors.New("账号目录正在变更，请稍后重试")
	}
	defer release()
	if err = s.executionAllowed(ctx, value); err != nil {
		return errors.New("目标或模板已变化，未执行导入")
	}
	confirmed, err := client.Accounts(ctx)
	if err != nil {
		return errors.New("导入前目录复核失败")
	}
	matched, err := FindExistingAccount(item, confirmed)
	if err != nil {
		return err
	}
	if text(matched["id"]) != text(existing["id"]) {
		return errors.New("账号目录已变化，请重新处理")
	}
	body := maps.Clone(payload)
	body["status"] = "inactive"
	body["schedulable"] = false
	ctx = adminclient.WithMutationAuthorization(ctx, func(ctx context.Context) error { return s.executionAllowed(ctx, value) })
	var account map[string]any
	if existing != nil {
		id := text(existing["id"])
		current, readErr := client.Account(ctx, id)
		if readErr != nil || accountVersion(current) != accountVersion(existing) {
			return errors.New("线上账号已变化，请刷新后重新导入")
		}
		if !sameAccountIdentity(current, item.Credentials) {
			return errors.New("线上账号身份与导入内容不符")
		}
		if current["schedulable"] != false {
			// PUT /accounts/:id ignores schedulable. Persist the expected
			// isolation state before using the dedicated scheduling endpoint.
			expected := maps.Clone(current)
			expected["schedulable"] = false
			value.AccountVersions[id] = accountVersion(expected)
			value.Phases[row.ID] = "isolating"
			row.AccountID = id
			if err = s.persistRun(value); err != nil {
				return errors.New("停止调度前记录保存失败")
			}
			current, err = pauseImportedAccount(ctx, client, id, item.Credentials)
			if err != nil {
				return err
			}
			if accountVersion(current) != value.AccountVersions[id] {
				return errors.New("停止调度期间账号配置已变化，请核对后重新导入")
			}
		}
		value.Phases[row.ID] = "updating"
		row.AccountID = id
		if err = s.persistRun(value); err != nil {
			return errors.New("更新前记录保存失败")
		}
		account, err = client.UpdateAccount(ctx, id, body)
		row.ImportAction = "updated"
	} else {
		body["group_ids"] = []int{}
		body["skip_default_group_bind"] = true
		value.Phases[row.ID] = "creating"
		if err = s.persistRun(value); err != nil {
			return errors.New("创建前记录保存失败")
		}
		account, err = client.CreateAccount(ctx, body)
		row.ImportAction = "created"
	}
	if err != nil {
		return errors.New("账号写入未确认，请在站点核对")
	}
	id := text(account["id"])
	if id == "" {
		return errors.New("账号写入未返回稳定 ID")
	}
	row.AccountID = id
	current, err := client.Account(ctx, id)
	if err != nil {
		return errors.New("账号写入后读取失败，请核对站点状态")
	}
	if !sameAccountIdentity(current, item.Credentials) {
		return errors.New("账号身份与导入内容不符，已停止后续写入")
	}
	if current["schedulable"] != false {
		current, err = pauseImportedAccount(ctx, client, id, item.Credentials)
		if err != nil {
			return err
		}
	}
	if row.ImportAction == "created" && len(publicAccount(current).Groups) > 0 {
		// Some management versions bind a default group even when creation
		// requests no groups. Clear it only after confirming isolation.
		current, err = client.UpdateAccountGroups(ctx, id, []int64{})
		if err != nil {
			return errors.New("新账号默认分组清除未确认，已停止启用")
		}
		if current["schedulable"] != false || !sameAccountIdentity(current, item.Credentials) {
			return errors.New("新账号隔离状态或身份已变化，已停止启用")
		}
	}
	if err = verifyModelMappings(current, object(value.Exports[index]["credentials"])); err != nil {
		return err
	}
	if err = verifyAppliedWhileIsolated(current, body); err != nil {
		return err
	}
	value.AccountVersions[id] = accountVersion(current)
	value.Phases[row.ID] = "isolated"
	if err = s.persistRun(value); err != nil {
		return errors.New("隔离账号记录保存失败")
	}
	return nil
}

func pauseImportedAccount(ctx context.Context, client *adminclient.Client, id string, credentials map[string]any) (map[string]any, error) {
	if _, err := client.SetAccountSchedulable(ctx, id, false); err != nil {
		return nil, errors.New("停止调度请求未确认，请核对站点状态")
	}
	current, err := client.Account(ctx, id)
	if err != nil {
		return nil, errors.New("停止调度后读取失败，请核对站点状态")
	}
	if current["schedulable"] != false || !sameAccountIdentity(current, credentials) {
		return nil, errors.New("账号停止调度未生效或身份已变化，未继续写入配置")
	}
	return current, nil
}
func accountVersion(account map[string]any) string {
	fields := map[string]any{}
	for _, key := range []string{"id", "name", "platform", "type", "credentials", "extra", "status", "schedulable", "proxy_id", "concurrency", "priority", "rate_multiplier", "load_factor", "group_ids", "groups", "account_groups", "notes", "expires_at", "auto_pause_on_expired", "upstream_billing_probe_enabled", "confirm_mixed_channel_risk"} {
		fields[key] = account[key]
	}
	return digest(fields)
}
func sameAccountIdentity(account map[string]any, credentials map[string]any) bool {
	workspace, user := credentialIdentity(credentials)
	actualWorkspace, actualUser := credentialIdentity(object(account["credentials"]))
	return text(account["platform"]) == "openai" && text(account["type"]) == "oauth" && workspace != "" && user != "" && workspace == actualWorkspace && user == actualUser
}
func (s *Service) promoteItem(ctx context.Context, value *privateRun, index int) error {
	return s.applyImportedItem(ctx, value, index, true)
}
func (s *Service) configureImportedItem(ctx context.Context, value *privateRun, index int) error {
	return s.applyImportedItem(ctx, value, index, false)
}
func (s *Service) applyImportedItem(ctx context.Context, value *privateRun, index int, enable bool) error {
	row := &value.Public.Items[index]
	id := row.AccountID
	if id == "" || value.AccountVersions[id] == "" {
		return errors.New("账号尚未确认隔离，不能启用")
	}
	ctx, release, err := targetguard.Acquire(targetguard.Expect(ctx, value.Target), s.private, mutationguard.Account(id))
	if err != nil {
		return errors.New("账号正在变更，请稍后重试")
	}
	defer release()
	if err = s.executionAllowed(ctx, value); err != nil {
		return errors.New("目标或模板已变化，未执行启用")
	}
	client, err := s.client(value.Target)
	if err != nil {
		return errors.New("目标配置无效")
	}
	current, err := client.Account(ctx, id)
	if err != nil {
		return errors.New("启用前账号读取失败")
	}
	if accountVersion(current) != value.AccountVersions[id] || current["schedulable"] != false || !sameAccountIdentity(current, value.Items[index].Credentials) {
		return errors.New("账号已被修改或不再隔离，请核对后重新处理")
	}
	body := maps.Clone(value.Exports[index])
	delete(body, "credentials")
	delete(body, "platform")
	delete(body, "type")
	body["status"] = "active"
	body["schedulable"] = false
	ctx = adminclient.WithMutationAuthorization(ctx, func(ctx context.Context) error { return s.executionAllowed(ctx, value) })
	if value.Phases[row.ID] != "configured" {
		value.Phases[row.ID] = "configuring"
		if err = s.persistRun(value); err != nil {
			return errors.New("应用模板前记录保存失败")
		}
		if _, err = client.UpdateAccount(ctx, id, body); err != nil {
			return errors.New("最终账号配置写入未确认")
		}
		current, err = client.Account(ctx, id)
		if err != nil {
			return errors.New("最终账号配置读取失败，未恢复调度")
		}
		if !sameAccountIdentity(current, value.Items[index].Credentials) {
			return errors.New("最终账号身份核对失败，未恢复调度")
		}
		if err = verifyModelMappings(current, object(value.Exports[index]["credentials"])); err != nil {
			return err
		}
		if err = verifyApplied(current, body); err != nil {
			return err
		}
		value.AccountVersions[id] = accountVersion(current)
		value.Phases[row.ID] = "configured"
		if err = s.persistRun(value); err != nil {
			return errors.New("模板配置确认记录保存失败")
		}
	}
	if !enable {
		return nil
	}
	value.Phases[row.ID] = "promoting"
	if err = s.persistRun(value); err != nil {
		return errors.New("启用前记录保存失败")
	}
	if _, err = client.Mutate(ctx, http.MethodPost, "/admin/accounts/"+id+"/clear-error", map[string]any{}); err != nil {
		return errors.New("旧错误状态清理未确认，未恢复调度")
	}
	if _, err = client.SetAccountSchedulable(ctx, id, true); err != nil {
		return errors.New("恢复调度结果未确认")
	}
	current, err = client.Account(ctx, id)
	if err != nil || current["schedulable"] != true || text(current["status"]) != "active" || !sameAccountIdentity(current, value.Items[index].Credentials) {
		return errors.New("启用后的账号状态核对失败")
	}
	body["schedulable"] = true
	if err = verifyModelMappings(current, object(value.Exports[index]["credentials"])); err != nil {
		return err
	}
	if err = verifyApplied(current, body); err != nil {
		return err
	}
	value.AccountVersions[id] = accountVersion(current)
	value.Phases[row.ID] = "completed"
	return s.persistRun(value)
}
