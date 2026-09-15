package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) runWorkbenchInput(parent context.Context, job *workbenchRun, task taskstore.Task) {
	defer close(job.done)
	ctx, cancel := context.WithDeadline(targetguard.Expect(parent, job.prepared.target), job.expires)
	defer cancel()
	job.mu.Lock()
	job.cancel = cancel
	if job.view.Status == "cancelled" {
		cancel()
	}
	job.mu.Unlock()
	err := s.executeWorkbenchInput(ctx, job, &task)
	job.mu.Lock()
	job.ready = false
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		if job.queue != nil && !job.queueFrozen && job.view.Status != "cancelled" && time.Now().Before(job.expires) {
			persist, done := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.persistMixedQueueLocked(persist, job, "interrupted")
			done()
			job.queueFrozen = true
		}
		job.view.Status, job.view.Message = "failed", publicError(err).Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			job.view.Status, job.view.Message = "cancelled", "混合输入批次已取消或到期"
		}
		job.items = nil
		job.view.Available = 0
		for i := range job.view.Items {
			if job.view.Items[i].Status == "queued" || job.view.Items[i].Status == "running" || job.view.Items[i].Status == "waiting_input" {
				job.view.Items[i].Status, job.view.Items[i].Message = "failed", job.view.Message
			}
		}
	} else if len(job.items) == 0 {
		job.view.Status, job.view.Message = "failed", "本批没有取得可用账号，请核对失败明细后重新输入"
	} else {
		job.view.Status, job.view.Message = "ready", "账号处理完成，请预览并确认最终操作"
		job.view.Available = len(job.items)
	}
	job.prepared.entries = nil
	job.prepared.oauth = nil
	task.Status, task.Message, task.Progress = job.view.Status, job.view.Message, 100
	if task.Status == "ready" {
		task.Status = "succeeded"
		for _, row := range job.view.Items {
			if row.Status != "succeeded" {
				task.Status = "partial"
				break
			}
		}
	}
	task.Result = mixedTaskResult(job.view)
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	job.mu.Unlock()
	persist, done := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer done()
	if saveErr := s.persistMixedQueue(persist, job, "completed"); saveErr != nil {
		job.mu.Lock()
		job.items = nil
		job.view.Available = 0
		job.view.Status, job.view.Message = "failed", "混合批次恢复资料保存失败，请重新输入后处理"
		job.mu.Unlock()
		return
	}
	if saveErr := s.tasks.Save(persist, task); saveErr != nil {
		job.mu.Lock()
		job.items = nil
		job.view.Available = 0
		job.view.Status, job.view.Message = "failed", "混合批次结果保存失败，请重新输入后处理"
		job.mu.Unlock()
		return
	}
	job.mu.Lock()
	job.ready = job.view.Status == "ready"
	job.mu.Unlock()
}

func (s *Service) executeWorkbenchInput(ctx context.Context, job *workbenchRun, task *taskstore.Task) error {
	if _, _, err := s.bindWorkbenchScope(ctx, job.prepared.options.Scope, &job.prepared.target); err != nil {
		return err
	}
	if err := s.validateMixedTemplates(ctx, job.prepared); err != nil {
		return err
	}
	var client *adminclient.Client
	if job.prepared.options.Scope != ScopeLocalExport {
		var err error
		client, err = s.clientFor(job.prepared.target)
		if err != nil {
			return err
		}
	}
	job.mu.Lock()
	entries := append([]mixedEntry(nil), job.prepared.entries...)
	job.view.Status, job.view.Message = "running", "正在处理已提供的账号凭据"
	job.mu.Unlock()
	if err := s.saveWorkbenchRun(ctx, job, task); err != nil {
		return err
	}
	job.mu.Lock()
	items := append([]InputItem(nil), job.items...)
	job.mu.Unlock()
	positions := []int{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, _, err := s.bindWorkbenchScope(ctx, job.prepared.options.Scope, &job.prepared.target); err != nil {
			return err
		}
		job.mu.Lock()
		row := job.view.Items[entry.index]
		job.mu.Unlock()
		if row.Kind == "oauth_login" {
			positions = append(positions, entry.index)
			continue
		}
		if row.Status != "queued" {
			continue
		}
		if entry.item == nil {
			return ErrPreview
		}
		job.mu.Lock()
		job.view.Items[entry.index].Status, job.view.Items[entry.index].Message = "running", "正在校验账号凭据"
		job.prepared.entries[entry.index].item = nil
		job.mu.Unlock()
		if err := s.saveWorkbenchRun(ctx, job, task); err != nil {
			return err
		}
		item := *entry.item
		var materializeErr error
		if job.prepared.options.Scope != ScopeLocalExport {
			item, materializeErr = materialize(ctx, client, item)
		}
		job.mu.Lock()
		if materializeErr != nil {
			job.view.Items[entry.index].Status, job.view.Items[entry.index].Message = "failed", publicError(materializeErr).Error()
		} else {
			item.Index = entry.index
			items = append(items, item)
			job.items = append(job.items, item)
			job.view.Items[entry.index].Status, job.view.Items[entry.index].Message = "succeeded", "凭据已就绪，等待最终预览"
			if job.prepared.options.Scope == ScopeLocalExport && inputText(item.Credentials["access_token"]) == "" {
				job.view.Items[entry.index].Message = "刷新令牌已解析，等待最终确认后刷新身份"
			}
		}
		job.mu.Unlock()
		if err := s.saveWorkbenchRun(ctx, job, task); err != nil {
			return err
		}
	}
	job.mu.Lock()
	hasOAuth := job.prepared.oauth != nil
	job.mu.Unlock()
	if len(positions) > 0 && hasOAuth {
		authorized, err := s.runWorkbenchOAuth(ctx, job, task, positions)
		if err != nil {
			return err
		}
		items = append(items, authorized...)
		job.mu.Lock()
		job.items = append([]InputItem(nil), items...)
		job.oauthQueue = nil
		job.mu.Unlock()
		if err := s.saveWorkbenchRun(ctx, job, task); err != nil {
			return err
		}
	}
	if _, _, err := s.bindWorkbenchScope(ctx, job.prepared.options.Scope, &job.prepared.target); err != nil {
		return err
	}
	if err := s.validateMixedTemplates(ctx, job.prepared); err != nil {
		return err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Index < items[j].Index })
	seen := map[string]bool{}
	duplicates := []InputError{}
	for _, item := range items {
		key := IdentityKey(item)
		if key != "" && seen[key] {
			duplicates = append(duplicates, InputError{Index: item.Index, Message: "此账号与本批其他来源的稳定身份重复，请保留一项后重新输入"})
		}
		seen[key] = true
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if len(duplicates) > 0 {
		job.view.Errors = duplicates
		return errors.New("混合输入存在重复稳定身份，未生成最终预览，请删除重复项")
	}
	job.items = items
	return nil
}

func (s *Service) runWorkbenchOAuth(ctx context.Context, job *workbenchRun, task *taskstore.Task, positions []int) ([]InputItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	job.mu.Lock()
	prepared := job.prepared.oauth
	job.prepared.oauth = nil
	if prepared == nil {
		job.mu.Unlock()
		return nil, ErrPreview
	}
	if job.queue != nil {
		prepared.view.RecoveryEnabled = true
		prepared.parentQueuePersist = func(ctx context.Context, child *oauthBatch, status string) error {
			return s.persistMixedOAuth(ctx, job, child, status)
		}
		if prepared.resumePayload == nil {
			prepared.resumePayload = job.oauthQueue
		}
	}
	job.mu.Unlock()
	previewID, err := randomID()
	if err != nil {
		return nil, err
	}
	prepared.expires = time.Now().Add(10 * time.Minute)
	prepared.view.ID, prepared.view.ExpiresAt = previewID, prepared.expires.UTC().Format(time.RFC3339Nano)
	s.batches.mu.Lock()
	s.batches.previews[previewID] = prepared
	s.batches.mu.Unlock()
	defer s.DeleteOAuthBatchPreview(job.prepared.owner, previewID)
	view, err := s.StartOAuthBatch(ctx, job.prepared.owner, previewID, true)
	if err != nil {
		return nil, err
	}
	child, err := s.oauthBatch(job.prepared.owner, view.ID)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { s.stopMixedOAuth(job, child) })
	defer stop()
	defer func() {
		if ctx.Err() != nil {
			s.stopMixedOAuth(job, child)
		} else {
			_ = s.CancelOAuthBatch(job.prepared.owner, view.ID)
		}
	}()
	job.mu.Lock()
	job.view.OAuthBatchID = view.ID
	job.view.Status, job.view.Message = "waiting_input", "正在授权邮箱账号，官方验证可在本批接管"
	for _, index := range positions {
		job.view.Items[index].Status, job.view.Items[index].Message = "waiting_input", "等待官方授权验证"
	}
	job.mu.Unlock()
	if err := s.saveWorkbenchRun(ctx, job, task); err != nil {
		s.stopMixedOAuth(job, child)
		<-child.done
		return nil, err
	}
	<-child.done
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	child.mu.Lock()
	rows := append([]OAuthBatchRow(nil), child.view.Items...)
	results := append([]InputItem(nil), child.results...)
	ready := child.ready
	status := child.view.Status
	child.mu.Unlock()
	job.mu.Lock()
	for i, row := range rows {
		if i < len(positions) {
			job.view.Items[positions[i]].Status, job.view.Items[positions[i]].Message = row.Status, row.Message
		}
	}
	job.view.Status, job.view.Message = "running", "正在合并各来源账号"
	job.mu.Unlock()
	if status == "cancelled" {
		return nil, context.Canceled
	}
	if len(results) > 0 && !ready {
		return nil, errors.New("授权结果尚未可靠保存，请重新授权")
	}
	items := []InputItem{}
	for _, result := range results {
		if result.Index < 0 || result.Index >= len(positions) {
			return nil, errors.New("授权结果序号不一致，请重新输入")
		}
		raw, err := json.Marshal(map[string]any{"credentials": result.Credentials})
		if err != nil {
			return nil, errors.New("授权凭据无法解析，请重新授权")
		}
		parsed, failures := Parse(string(raw))
		if len(failures) > 0 || len(parsed) != 1 {
			return nil, errors.New("授权结果缺少有效身份，请重新授权")
		}
		parsed[0].Index = positions[result.Index]
		items = append(items, parsed[0])
	}
	return items, nil
}

func (s *Service) saveWorkbenchRun(ctx context.Context, job *workbenchRun, task *taskstore.Task) error {
	job.mu.Lock()
	view := job.view
	if err := s.persistMixedQueueLocked(ctx, job, "running"); err != nil {
		job.mu.Unlock()
		return err
	}
	completed := 0
	for _, row := range view.Items {
		if row.Status == "succeeded" || row.Status == "failed" {
			completed++
		}
	}
	task.Result = mixedTaskResult(view)
	job.mu.Unlock()
	task.Status, task.Message = view.Status, view.Message
	if len(view.Items) > 0 {
		task.Progress = completed * 100 / len(view.Items)
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return s.tasks.Save(ctx, *task)
}

func (s *Service) PreviewWorkbenchRunResult(ctx context.Context, owner, id string) (Preview, error) {
	job, err := s.mixedRun(owner, id)
	if err != nil {
		return Preview{}, err
	}
	ctx, _, err = s.bindWorkbenchScope(ctx, job.prepared.options.Scope, &job.prepared.target)
	if err != nil {
		return Preview{}, err
	}
	if err := s.validateMixedTemplates(ctx, job.prepared); err != nil {
		return Preview{}, err
	}
	job.mu.Lock()
	if !job.ready || job.view.Status != "ready" {
		job.mu.Unlock()
		return Preview{}, errors.New("请等待混合批次处理完成并取得有效账号")
	}
	items := make([]InputItem, len(job.items))
	for i, item := range job.items {
		items[i] = item
		items[i].Credentials = cloneInputMap(item.Credentials)
	}
	job.mu.Unlock()
	preview, err := s.previewItems(ctx, owner, job.prepared.options, items)
	if err != nil {
		return preview, err
	}
	if err := s.validateMixedTemplates(ctx, job.prepared); err != nil {
		s.DeletePreview(owner, preview.ID)
		return Preview{}, err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if !job.ready || job.view.Status != "ready" {
		s.DeletePreview(owner, preview.ID)
		return Preview{}, ErrPreview
	}
	if preview.ID != "" {
		job.previews = append(job.previews, preview.ID)
	}
	return preview, nil
}
