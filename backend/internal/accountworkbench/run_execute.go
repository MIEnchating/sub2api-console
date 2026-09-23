package accountworkbench

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) executionAllowed(ctx context.Context, value *privateRun) error {
	if (value.Public.Action == "export" && value.Scope != "local-export") || (value.Public.Action != "export" && value.Scope != "managed") {
		return errors.New("账号处理范围无效，请重新预览")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.checkOwner(ctx, value.Owner); err != nil {
		return err
	}
	if !value.Public.ExpiresAt.After(time.Now().UTC()) {
		return errors.New("本批资料已到期，请重新输入")
	}
	if value.Public.Action != "export" {
		if _, err := targetguard.Pin(targetguard.Expect(ctx, value.Target), s.private); err != nil {
			return err
		}
		library, err := s.readTemplates(ctx, value.Target)
		if err != nil {
			return err
		}
		if library.Revision != value.TemplateRevision {
			return errors.New("模板已变化，请重新预览")
		}
	}
	return nil
}
func (s *Service) execute(parent context.Context, value *privateRun, task taskstore.Task, active *activeRun) {
	ctx, cancel := context.WithDeadline(parent, value.Public.ExpiresAt)
	defer cancel()
	task.Status = "running"
	task.Message = "正在处理账号"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		value.Public.Status = "interrupted"
		_ = s.persistRun(value)
		return
	}
	value.Public.Status = "running"
	for index := range value.Items {
		row := &value.Public.Items[index]
		if len(value.ManualIDs) > 0 && !slices.Contains(value.ManualIDs, row.ID) {
			continue
		}
		if row.Status == "completed" || row.Status == "exported" {
			continue
		}
		if err := s.executionAllowed(ctx, value); err != nil {
			row.Status = "interrupted"
			row.Message = "任务已停止，请核对登录会话、目标与模板后继续"
			break
		}
		var itemErr error
		if len(value.ManualIDs) > 0 {
			itemErr = s.promoteItem(ctx, value, index)
			if itemErr == nil {
				row.Status = "completed"
				row.Message = "已启用（保留原检测结论）"
				row.ManualEnabled = true
			}
		} else {
			itemErr = s.processItem(ctx, value, index, active)
		}
		if itemErr != nil {
			row.Status = "failed"
			row.Message = itemErr.Error()
			phase := value.Phases[row.ID]
			if phase == "refreshing" || phase == "exchanging" {
				row.Status = "interrupted"
				row.Message = "上次提交结果需要核对，请勿重复提交该项"
			}
			if phase == "isolating" || phase == "creating" || phase == "updating" || phase == "configuring" || phase == "promoting" {
				row.Status = "interrupted"
				row.Message = itemErr.Error() + "；未重复提交，请先核对站点状态"
			}
		}
		row.LoginPrompt = nil
		if err := s.persistRun(value); err != nil {
			value.Public.Status = "interrupted"
			break
		}
		task.Progress = (index + 1) * 100 / len(value.Items)
		task.Message = fmt.Sprintf("已处理 %d / %d 项", index+1, len(value.Items))
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.tasks.Save(ctx, task); err != nil {
			value.Public.Status = "interrupted"
			break
		}
	}
	terminal := "completed"
	taskStatus := "succeeded"
	for i := range value.Public.Items {
		row := &value.Public.Items[i]
		if row.Status == "queued" || row.Status == "preparing" || row.Status == "authorizing" || row.Status == "checking" || row.Status == "waiting_input" {
			row.Status = "interrupted"
			row.Message = "任务已中断，可在有效期内继续未提交项"
		}
		if row.Status != "completed" && row.Status != "exported" {
			terminal = "needs_attention"
			taskStatus = "partial"
		}
		row.LoginPrompt = nil
	}
	if ctx.Err() != nil {
		terminal = "interrupted"
		taskStatus = "cancelled"
	}
	if value.Public.Status == "interrupted" {
		terminal = "interrupted"
		taskStatus = "failed"
	}
	value.Public.Status = terminal
	value.ManualIDs = nil
	if err := s.persistRun(value); err != nil {
		taskStatus = "failed"
	}
	task.Status = taskStatus
	task.Message = "账号处理已结束，请查看工作台处理记录"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}
func (s *Service) processItem(ctx context.Context, value *privateRun, index int, active *activeRun) error {
	row := &value.Public.Items[index]
	stored := &value.Items[index]
	phase := value.Phases[row.ID]
	switch phase {
	case "refreshing", "exchanging":
		return errors.New("上次提交结果尚未核对")
	case "isolating", "creating", "updating", "configuring", "promoting":
		if err := s.reconcileItem(ctx, value, index); err != nil {
			return err
		}
		if row.Status == "completed" {
			return nil
		}
		phase = value.Phases[row.ID]
	}
	proxyURL := stored.ProxyURL
	if proxyURL == "" && value.Settings.ProxyURL != "" {
		expanded, err := loginproxy.SessionURL(value.Settings.ProxyURL)
		if err != nil {
			return errors.New("登录代理配置无效")
		}
		proxyURL = expanded
		stored.ProxyURL = expanded
		if err = s.persistRun(value); err != nil {
			return errors.New("登录代理会话保存失败")
		}
	}
	row.Status = "preparing"
	row.Message = "正在准备账号"
	if stored.Item.Kind == "login" {
		credentials, err := s.authorizeItem(ctx, value, index, active, proxyURL)
		if len(credentials) > 0 {
			stored.Credentials = credentials
			stored.Item.Kind = "codex_json"
			value.Phases[row.ID] = "materialized"
			if saveErr := s.persistRun(value); saveErr != nil {
				return errors.New("授权凭据保存失败，已停止处理")
			}
		}
		if err != nil {
			return err
		}
	} else if stored.Item.Kind == "refresh_token" || text(stored.Credentials["access_token"]) == "" {
		value.Phases[row.ID] = "refreshing"
		if err := s.persistRun(value); err != nil {
			return errors.New("刷新前记录保存失败")
		}
		// Do not interrupt a one-use request midway; preserve a received rotation first.
		exchange, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
		credentials, err := s.RefreshCredential(exchange, text(stored.Credentials["refresh_token"]), proxyURL)
		cancel()
		if len(credentials) > 0 {
			stored.Credentials = credentials
			stored.Item.Kind = "codex_json"
			value.Phases[row.ID] = "materialized"
			if saveErr := s.persistRun(value); saveErr != nil {
				return errors.New("旋转凭据保存失败，已停止后续处理")
			}
		}
		if err != nil {
			return err
		}
	}
	if err := s.executionAllowed(ctx, value); err != nil {
		return errors.New("会话、目标或模板已变化，已停止处理")
	}
	// Apply and validate the selected template before checking account identity.
	item := stored.Item
	item.Credentials = stored.Credentials
	if _, err := AccountPayload(item, value.Template); err != nil {
		return err
	}
	identity, err := s.VerifyCredential(ctx, stored.Credentials, proxyURL)
	if err != nil {
		return err
	}
	if stored.Item.Email != "" && !equalEmail(stored.Item.Email, identity.Email) {
		return ErrIdentity
	}
	stored.Credentials["chatgpt_account_id"] = identity.WorkspaceID
	stored.Credentials["chatgpt_user_id"] = identity.UserID
	stored.Credentials["email"] = identity.Email
	stored.Item.Email = identity.Email
	row.Email = identity.Email
	row.IdentitySource = "official_signature"
	item = stored.Item
	item.Credentials = stored.Credentials
	payload, err := AccountPayload(item, value.Template)
	if err != nil {
		return err
	}
	value.Exports[index] = payload
	if err = s.persistRun(value); err != nil {
		return errors.New("已验证凭据保存失败")
	}
	if value.Public.Action == "export" {
		row.Status = "exported"
		row.Message = "私有 JSON 已就绪"
		value.Phases[row.ID] = "exported"
		return nil
	}
	if row.AccountID == "" {
		if err = s.importItem(ctx, value, index, payload); err != nil {
			return err
		}
	}
	if err = s.configureImportedItem(ctx, value, index); err != nil {
		return err
	}
	if value.Settings.Check {
		if err = s.checkImportItem(ctx, value, index); err != nil {
			return err
		}
		if row.Status == "review" {
			return nil
		}
	}
	if value.Settings.Promote {
		if err = s.promoteItem(ctx, value, index); err != nil {
			return err
		}
	}
	row.Status = "completed"
	row.Message = "账号已导入并应用设置"
	if row.ImportAction == "updated" {
		row.Message = "账号已存在，已更新设置"
	}
	if !value.Settings.Promote {
		row.Message += "，未启用"
	}
	return nil
}
