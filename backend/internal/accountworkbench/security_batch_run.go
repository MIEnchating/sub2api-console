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

func securityBatchTaskResult(view SecurityBatchView) map[string]any {
	return map[string]any{"request_id": view.ID, "phase": view.Status, "scope": view.Scope, "operation": view.Operation, "completed": view.Completed, "succeeded": view.Succeeded, "items": cloneSecurityRows(view.Items)}
}

func (s *Service) runSecurityBatch(parent context.Context, job *securityBatch, task taskstore.Task) {
	defer s.forgetClosingSecurityBatch(task.ID)
	defer close(job.done)
	defer s.releaseOAuthBatch(task.ID)
	ctx, cancel := context.WithDeadline(parent, job.expires)
	defer cancel()
	job.mu.Lock()
	job.cancel = cancel
	if job.view.Status == "cancelled" {
		cancel()
	} else {
		job.view.Status = "running"
	}
	count := len(job.items)
	job.mu.Unlock()
	runErr := ctx.Err()
	for index := 0; index < count && runErr == nil; index++ {
		if _, _, runErr = s.bindWorkbenchScope(ctx, job.view.Scope, &job.target); runErr != nil {
			break
		}
		if runErr = ctx.Err(); runErr != nil {
			break
		}
		if runErr = s.runSecurityBatchItem(ctx, job, &task, index); runErr != nil {
			break
		}
		job.mu.Lock()
		job.view.Completed = index + 1
		job.mu.Unlock()
		if runErr = s.saveSecurityBatch(ctx, job, &task); runErr != nil {
			break
		}
	}
	s.finishSecurityBatch(job, &task, runErr)
}

func (s *Service) runSecurityBatchItem(ctx context.Context, job *securityBatch, task *taskstore.Task, index int) error {
	job.mu.Lock()
	if job.view.Status == "cancelled" {
		job.mu.Unlock()
		return context.Canceled
	}
	item := job.items[index]
	input := SecurityStartInput{Scope: job.view.Scope, AccountID: item.accountID, Operation: job.view.Operation, Password: job.password, ProxyURL: job.proxyURL, Confirmed: true}
	job.view.Items[index].Status, job.view.Items[index].Message = "running", "等待完成该账号的官方安全验证"
	job.view.Message = "正在逐项执行账号安全操作"
	job.mu.Unlock()
	if err := s.saveSecurityBatch(ctx, job, task); err != nil {
		return err
	}
	view, err := s.startSecurityBatchChild(ctx, job, input, item)
	input.Password = ""
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, targetguard.ErrChanged) || errors.Is(err, browserlogin.ErrSecurityIdentity) || errors.Is(err, ErrSecurityStorage) {
			return err
		}
		job.mu.Lock()
		defer job.mu.Unlock()
		if job.view.Status == "cancelled" {
			return context.Canceled
		}
		job.view.Items[index].Status, job.view.Items[index].Message = "failed", "安全操作未启动，请核对账号身份、浏览器与任务容量后重新确认"
		return nil
	}
	child, err := s.securitySession(job.owner, view.ID)
	if err != nil {
		s.removeSecurity(job.owner, view.ID)
		return err
	}
	job.mu.Lock()
	cancelled := job.view.Status == "cancelled" || ctx.Err() != nil
	if !cancelled {
		job.view.CurrentSecurityID = view.ID
		job.view.Items[index].SecurityID = view.ID
	}
	job.mu.Unlock()
	if cancelled {
		s.removeSecurity(job.owner, view.ID)
		<-child.done
		s.captureSecurityBatchChild(job, index, child)
		return context.Canceled
	}
	if err := s.saveSecurityBatch(ctx, job, task); err != nil {
		s.removeSecurity(job.owner, view.ID)
		<-child.done
		s.captureSecurityBatchChild(job, index, child)
		return err
	}
	select {
	case <-ctx.Done():
		s.removeSecurity(job.owner, view.ID)
		<-child.done
	case <-child.done:
	}
	s.captureSecurityBatchChild(job, index, child)
	if item.source == nil {
		s.removeSecurity(job.owner, view.ID)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.view.Status == "cancelled" {
		return context.Canceled
	}
	return nil
}

func (s *Service) captureSecurityBatchChild(job *securityBatch, index int, child *securitySession) {
	child.mu.Lock()
	view := child.view
	child.mu.Unlock()
	job.mu.Lock()
	defer job.mu.Unlock()
	job.view.CurrentSecurityID = ""
	row := &job.view.Items[index]
	row.Status, row.Message, row.ArtifactID = view.Status, view.Message, view.ArtifactID
	row.UserID, row.Email = view.UserID, view.Email
	row.WorkspaceID = view.WorkspaceID
	if row.UserID == "" {
		row.UserID = child.expected.UserID
	}
	if view.Status == "expired" {
		row.Status = "cancelled"
	}
	if view.Status == "succeeded" {
		job.view.Succeeded++
	}
}

func (s *Service) finishSecurityBatch(job *securityBatch, task *taskstore.Task, runErr error) {
	job.mu.Lock()
	if job.view.Status == "cancelled" {
		runErr = context.Canceled
	} else if !time.Now().Before(job.expires) {
		runErr = context.DeadlineExceeded
	}
	job.password = ""
	job.proxyURL = ""
	job.view.CurrentSecurityID = ""
	job.view.Status, job.view.Message = "failed", "本批安全操作未完成，请逐项核对官方状态和私有结果"
	if job.view.Succeeded == len(job.items) {
		job.view.Status, job.view.Message = "succeeded", "本批账号安全操作已完成，请查看各项状态及私有结果"
	} else if job.view.Succeeded > 0 {
		job.view.Status, job.view.Message = "partial", "本批部分安全操作已完成，请核对未完成项目和私有结果"
	}
	if runErr != nil {
		job.view.Status, job.view.Message = "failed", "批量安全操作已停止，请核对管理目标、账号身份和私有结果"
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			job.view.Status, job.view.Message = "cancelled", "批量安全操作已取消或到期，已执行项目请核对官方状态与私有结果"
		}
	}
	for i := range job.view.Items {
		if job.view.Items[i].Status == "queued" || job.view.Items[i].Status == "running" {
			job.view.Items[i].Status, job.view.Items[i].Message = "cancelled", "本批已停止，未继续安全操作"
		}
	}
	job.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := s.saveSecurityBatch(ctx, job, task)
	cancel()
	if err == nil {
		return
	}
	slog.Error("批量安全任务结果保存失败", "task_id", task.ID)
	job.mu.Lock()
	if job.view.Status != "cancelled" {
		job.view.Status, job.view.Message = "failed", "批量安全任务记录保存失败，请按各项私有结果核对官方状态"
	}
	job.mu.Unlock()
	recovery, cancelRecovery := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRecovery()
	if err := s.saveSecurityBatch(recovery, job, task); err != nil {
		slog.Error("批量安全失败状态保存失败", "task_id", task.ID)
	}
}

func (s *Service) saveSecurityBatch(ctx context.Context, job *securityBatch, task *taskstore.Task) error {
	job.mu.Lock()
	defer job.mu.Unlock()
	task.Status = job.view.Status
	if task.Status == "queued" {
		task.Status = "running"
	}
	task.Message = job.view.Message
	if len(job.items) > 0 {
		task.Progress = job.view.Completed * 100 / len(job.items)
	}
	task.Result = securityBatchTaskResult(job.view)
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return s.tasks.Save(ctx, *task)
}
