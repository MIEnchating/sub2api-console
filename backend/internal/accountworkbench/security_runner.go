package accountworkbench

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func securityTaskResult(view SecurityView) map[string]any {
	result := map[string]any{"phase": view.Status, "scope": view.Scope, "operation": view.Operation}
	if view.SourceOAuthBatchID != "" {
		result["source_oauth_batch_id"] = view.SourceOAuthBatchID
		if view.SourceIndex != nil {
			result["source_index"] = *view.SourceIndex
		}
	} else if view.SourceOAuthID != "" {
		result["source_oauth_id"] = view.SourceOAuthID
	} else if view.SourceCheckpointID != "" {
		result["source_checkpoint_id"] = view.SourceCheckpointID
		result["identity_confirmed"] = view.IdentityConfirmed
	} else {
		result["account_id"] = view.AccountID
	}
	if view.ArtifactID != "" {
		result["artifact_id"] = view.ArtifactID
	}
	return result
}

func (s *Service) runSecurity(parent context.Context, value *securitySession, task taskstore.Task) {
	defer close(value.done)
	defer s.releaseOAuthBrowser()
	defer func() { _ = s.completeSecurityCheckpoint(context.Background(), value, false) }()
	ctx, cancel := context.WithDeadline(parent, value.expires)
	defer cancel()
	value.mu.Lock()
	value.cancel = cancel
	proxyURL := value.proxyURL
	value.proxyURL = ""
	if value.view.Status == "cancelled" {
		cancel()
	}
	value.mu.Unlock()
	err := ctx.Err()
	if err == nil {
		err = s.validateSecurityOrigin(ctx, value)
	}
	var browser browserlogin.SecurityBrowser
	if err == nil {
		browser, err = s.openSecurityBrowser(ctx, value, proxyURL)
	}
	if browser != nil {
		defer func() {
			value.op.Lock()
			value.mu.Lock()
			value.browser = nil
			value.password = ""
			value.mu.Unlock()
			browser.Close()
			value.op.Unlock()
		}()
	}
	if err != nil || browser == nil {
		s.finishSecurity(value, &task, "failed", "安全浏览器启动失败，请检查 browser 服务后重试", ctx.Err())
		return
	}
	value.mu.Lock()
	value.browser = browser
	value.mu.Unlock()
	message := "请在官方页面登录所选账号，完成后继续"
	if value.checkpoint != nil {
		value.op.Lock()
		err = s.discoverSecurityIdentity(ctx, value, browser)
		value.op.Unlock()
		if err != nil && !errors.Is(err, browserlogin.ErrSecurityPending) {
			s.finishSecurity(value, &task, "failed", securityErrorMessage(err), ctx.Err())
			return
		}
	}
	if err := s.waitSecurity(ctx, value, &task, message); err != nil {
		s.finishSecurity(value, &task, "failed", "安全任务状态保存失败，请重试", ctx.Err())
		return
	}
	for {
		select {
		case <-ctx.Done():
			s.finishSecurity(value, &task, "cancelled", "账号安全操作已关闭", ctx.Err())
			return
		case command := <-value.commands:
			task.Status, task.Message = "running", "正在核对官方身份并处理安全步骤"
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			value.mu.Lock()
			task.Result = securityTaskResult(value.view)
			value.mu.Unlock()
			task.Result["phase"] = "running"
			if err := s.tasks.Save(ctx, task); err != nil {
				s.finishSecurity(value, &task, "failed", "安全任务状态保存失败，请重试", ctx.Err())
				return
			}
			value.op.Lock()
			guarded, release, stepErr := s.securityGuard(ctx, value)
			complete := false
			message := "请继续完成官方页面验证，完成后继续"
			if stepErr == nil {
				if command.confirmation != nil {
					stepErr = s.confirmSecurityIdentityStep(guarded, value, browser, *command.confirmation)
				} else if command.auth != nil {
					stepErr = browser.ApplyAuth(guarded, *command.auth)
					command.auth.Value = ""
				} else if value.checkpoint != nil && !value.view.IdentityConfirmed {
					stepErr = s.discoverSecurityIdentity(guarded, value, browser)
				} else {
					complete, message, stepErr = s.securityStep(guarded, value, browser)
				}
				_ = release()
			}
			value.op.Unlock()
			if errors.Is(stepErr, browserlogin.ErrSecurityPending) || errors.Is(stepErr, browserlogin.ErrAuthPageChanged) {
				stepErr = nil
			}
			if stepErr != nil {
				s.finishSecurity(value, &task, "failed", securityErrorMessage(stepErr), ctx.Err())
				return
			}
			if complete {
				s.finishSecurity(value, &task, "succeeded", message, ctx.Err())
				return
			}
			if err := s.waitSecurity(ctx, value, &task, message); err != nil {
				s.finishSecurity(value, &task, "failed", "安全任务状态保存失败，请重试", ctx.Err())
				return
			}
		}
	}
}

func securityErrorMessage(err error) string {
	switch {
	case errors.Is(err, browserlogin.ErrSecurityIdentity):
		return "官方登录身份与所选账号不一致，请重新登录指定账号"
	case errors.Is(err, targetguard.ErrChanged):
		return "管理目标已变化，请关闭会话后重新确认账号"
	case errors.Is(err, browserlogin.ErrSession):
		return "授权来源已关闭或过期，请重新授权并确认账号身份"
	case errors.Is(err, browserlogin.ErrSecurityConfirmation):
		return "官方身份尚未确认，请核对邮箱和用户 ID 后重新确认"
	case errors.Is(err, ErrSecurityStorage):
		return "安全结果保存失败，已停止后续写入，请检查服务器私有目录并核对官方状态"
	case errors.Is(err, browserlogin.ErrSecurityUncertain):
		return "安全操作结果未确认，请先核对官方账号状态和私有结果，不能自动重试"
	default:
		return "安全操作未完成，请核对官方账号状态后重新确认"
	}
}

func (s *Service) waitSecurity(ctx context.Context, value *securitySession, task *taskstore.Task, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	value.mu.Lock()
	if value.view.Status == "cancelled" {
		value.mu.Unlock()
		return context.Canceled
	}
	value.view.Status, value.view.Message = "waiting", message
	if value.checkpoint != nil && value.expected.UserID != "" && !value.view.IdentityConfirmed {
		value.view.Status, value.view.Message = "awaiting_confirmation", "请核对官方返回的邮箱和用户 ID，并再次确认账号安全设置"
		message = value.view.Message
	}
	task.Result = securityTaskResult(value.view)
	value.mu.Unlock()
	task.Status, task.Message, task.UpdatedAt = "waiting_input", message, time.Now().UTC().Format(time.RFC3339Nano)
	return s.tasks.Save(ctx, *task)
}

func (s *Service) finishSecurity(value *securitySession, task *taskstore.Task, status, message string, cause error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	if errors.Is(cause, context.DeadlineExceeded) || !time.Now().Before(value.expires) {
		status, message = "expired", "账号安全会话已过期，请核对官方状态和私有结果"
	} else if errors.Is(cause, context.Canceled) || value.view.Status == "cancelled" {
		status, message = "cancelled", "账号安全会话已关闭，请核对官方状态和私有结果"
	}
	if err := value.saveSecurityOutcome(status); err != nil {
		status, message = "failed", "私有结果状态保存失败，请核对服务器文件与官方账号状态"
	}
	value.password = ""
	value.view.Status, value.view.Message, value.view.ArtifactID = status, message, value.artifactID
	task.Status = status
	if status == "expired" {
		task.Status = "failed"
	}
	task.Message, task.Progress, task.UpdatedAt, task.Result = message, 100, time.Now().UTC().Format(time.RFC3339Nano), securityTaskResult(value.view)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.tasks.Save(ctx, *task); err != nil {
		slog.Error("账号安全任务结果保存失败", "task_id", task.ID)
		value.view.Status, value.view.Message = "failed", "安全任务结果保存失败，请核对官方状态和私有结果"
	}
}
