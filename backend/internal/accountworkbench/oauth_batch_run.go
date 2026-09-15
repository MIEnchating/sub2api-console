package accountworkbench

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) runOAuthBatch(parent context.Context, job *oauthBatch, task taskstore.Task) {
	defer close(job.done)
	defer s.releaseOAuthBatch(task.ID)
	ctx, cancel := context.WithDeadline(targetguard.Expect(parent, job.target), job.expires)
	defer cancel()
	job.mu.Lock()
	job.cancel = cancel
	if job.view.Status == "cancelled" || job.stopRequested {
		cancel()
	} else {
		job.view.Status = "running"
	}
	count := len(job.view.Items)
	job.mu.Unlock()
	completed := 0
	runErr := ctx.Err()
	for index := 0; index < count && runErr == nil; index++ {
		job.mu.Lock()
		pending := job.view.Items[index].Status == "queued" || job.view.Items[index].Status == "running" && job.checkpoints[index].ID != ""
		job.mu.Unlock()
		if !pending {
			completed++
			continue
		}
		if _, _, runErr = s.bindWorkbenchScope(ctx, job.scope, &job.target); runErr != nil {
			break
		}
		if runErr = ctx.Err(); runErr != nil {
			break
		}
		if runErr = s.runOAuthBatchItem(ctx, job, &task, index); runErr != nil {
			break
		}
		completed++
		if runErr = s.saveOAuthBatch(ctx, job, &task, completed); runErr != nil {
			break
		}
	}
	s.finishOAuthBatch(job, &task, completed, runErr)
}

func (s *Service) runOAuthBatchItem(ctx context.Context, job *oauthBatch, task *taskstore.Task, index int) error {
	job.mu.Lock()
	if job.view.Status == "cancelled" || index >= len(job.inputs) {
		job.mu.Unlock()
		return context.Canceled
	}
	login := job.inputs[index]
	email := job.view.Items[index].Email
	job.inputs[index] = OAuthLoginInput{}
	job.view.Items[index].Status, job.view.Items[index].Message = "running", "正在授权"
	job.view.Message = "正在逐项授权账号"
	job.mu.Unlock()
	if err := s.saveOAuthBatch(ctx, job, task, index); err != nil {
		return err
	}
	var profile *profileAuthorization
	if len(job.profiles) > index {
		profile = job.profiles[index]
	}
	view, err := s.startOAuthQueueItem(ctx, job, task.ID, index, &login, profile)
	login = OAuthLoginInput{}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, targetguard.ErrChanged) {
			return err
		}
		job.mu.Lock()
		defer job.mu.Unlock()
		if job.view.Status == "cancelled" {
			return context.Canceled
		}
		job.view.Items[index].Status, job.view.Items[index].Message = "failed", "授权未启动，请核对浏览器和任务容量后重新授权"
		return nil
	}
	child, err := s.oauthSession(job.owner, view.ID)
	if err != nil {
		s.removeOAuth(job.owner, view.ID)
		return err
	}
	job.mu.Lock()
	cancelled := job.view.Status == "cancelled" || ctx.Err() != nil
	if !cancelled {
		job.view.CurrentOAuthID = view.ID
	}
	job.mu.Unlock()
	if cancelled {
		s.removeOAuth(job.owner, view.ID)
		<-child.done
		return context.Canceled
	}
	if err := s.saveOAuthBatch(ctx, job, task, index); err != nil {
		s.removeOAuth(job.owner, view.ID)
		<-child.done
		return err
	}
	select {
	case <-ctx.Done():
		stopOAuthChild(child)
	case <-child.done:
	}
	child.mu.Lock()
	status, message := child.view.Status, child.view.Message
	var credentials map[string]any
	if status == "authorized" && child.credentials != nil {
		credentials = cloneInputMap(child.credentials)
	}
	child.mu.Unlock()
	s.removeOAuth(job.owner, view.ID)
	job.mu.Lock()
	defer job.mu.Unlock()
	job.view.CurrentOAuthID = ""
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if job.view.Status == "cancelled" {
		return context.Canceled
	}
	if job.view.Items[index].Status == "succeeded" {
		return nil
	}
	if credentials != nil {
		job.results = append(job.results, InputItem{Index: index, Email: email, Credentials: credentials})
		job.view.Items[index].Status, job.view.Items[index].Message = "succeeded", "授权成功，等待导入确认"
	} else {
		job.view.Items[index].Status, job.view.Items[index].Message = "failed", message
	}
	job.view.Available = len(job.results)
	return nil
}

func (s *Service) finishOAuthBatch(job *oauthBatch, task *taskstore.Task, completed int, runErr error) {
	job.mu.Lock()
	if runErr != nil && (job.queue != nil || job.parentQueuePersist != nil) && job.view.Status != "cancelled" && time.Now().Before(job.expires) {
		persist, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.persistOAuthQueue(persist, job, "interrupted"); err != nil {
			slog.Error("批量恢复状态写入失败", "task_id", task.ID)
		}
		cancel()
		job.queueFrozen = true
	}
	if job.view.Status == "cancelled" {
		runErr = context.Canceled
	} else if time.Now().After(job.expires) {
		runErr = context.DeadlineExceeded
	}
	if job.queue == nil || runErr != nil {
		job.inputs = nil
	}
	job.ready = false
	job.view.CurrentOAuthID = ""
	job.view.Status, job.view.Message = "failed", "批量授权未取得可用账号，请核对结果后重新授权"
	if len(job.results) > 0 {
		job.view.Status, job.view.Message = "authorized", "批量授权结束，等待确认导入成功账号"
	}
	if runErr != nil {
		job.view.Status, job.view.Message = "failed", "批量授权已停止，请核对管理目标与任务记录"
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			job.view.Status, job.view.Message = "cancelled", "批量授权已取消或到期"
		}
		job.results = nil
		job.view.Available = 0
	}
	for i := range job.view.Items {
		if job.view.Items[i].Status == "queued" || job.view.Items[i].Status == "running" {
			job.view.Items[i].Status, job.view.Items[i].Message = "cancelled", "本批已结束，未继续授权"
		}
	}
	job.mu.Unlock()
	final, cancelFinal := context.WithTimeout(context.Background(), 5*time.Second)
	err := s.saveOAuthBatch(final, job, task, completed)
	cancelFinal()
	if err == nil {
		return
	}
	slog.Error("批量授权任务结果保存失败", "task_id", task.ID)
	job.mu.Lock()
	job.results = nil
	job.ready = false
	job.view.Available = 0
	if job.view.Status != "cancelled" {
		job.view.Status, job.view.Message = "failed", "批量授权结果保存失败，请重新授权"
	}
	job.mu.Unlock()
	// A bounded best effort records the failure itself; credentials remain
	// unavailable even when the task repository is still failing.
	recovery, cancelRecovery := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRecovery()
	if err := s.saveOAuthBatch(recovery, job, task, completed); err != nil {
		slog.Error("批量授权失败状态保存失败", "task_id", task.ID)
	}
}

func (s *Service) saveOAuthBatch(ctx context.Context, job *oauthBatch, task *taskstore.Task, completed int) error {
	job.mu.Lock()
	defer job.mu.Unlock()
	queueStatus := "running"
	if job.view.Status == "authorized" || job.view.Status == "failed" || job.view.Status == "cancelled" {
		queueStatus = "completed"
	}
	if err := s.persistOAuthQueue(ctx, job, queueStatus); err != nil {
		return err
	}
	task.Status = "running"
	switch job.view.Status {
	case "authorized":
		task.Status = "succeeded"
		for _, item := range job.view.Items {
			if item.Status != "succeeded" {
				task.Status = "partial"
				break
			}
		}
	case "failed":
		task.Status = "failed"
	case "cancelled":
		task.Status = "cancelled"
	}
	task.Message = job.view.Message
	if len(job.view.Items) > 0 {
		task.Progress = completed * 100 / len(job.view.Items)
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = batchTaskResult(job.view)
	err := s.tasks.Save(ctx, *task)
	if job.view.Status == "authorized" && err == nil {
		job.ready = true
	}
	return err
}
