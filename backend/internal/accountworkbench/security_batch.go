package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) StartSecurityBatch(ctx context.Context, owner, previewID string, confirmed bool) (SecurityBatchView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if !confirmed {
		return SecurityBatchView{}, errors.New("请确认批量安全操作的账号范围及同批新密码")
	}
	if owner == "" {
		return SecurityBatchView{}, browserlogin.ErrSession
	}
	s.securityMu.Lock()
	storage := s.securityStorage
	s.securityMu.Unlock()
	if s.securityFactory == nil {
		return SecurityBatchView{}, errors.New("安全浏览器尚未配置，请检查 browser 服务")
	}
	if storage == nil || !storage.available() {
		return SecurityBatchView{}, ErrSecurityStorage
	}
	s.securityBatches.mu.Lock()
	prepared := s.securityBatches.previews[previewID]
	if prepared == nil || prepared.owner != owner || !time.Now().Before(prepared.expires) {
		s.securityBatches.mu.Unlock()
		return SecurityBatchView{}, errors.New("批量安全操作预览已失效，请重新确认")
	}
	if len(s.securityBatches.active) >= 10 {
		s.securityBatches.mu.Unlock()
		return SecurityBatchView{}, errors.New("批量安全操作结果已满，请关闭已有批次")
	}
	s.securityBatches.mu.Unlock()
	if err := s.validatePreparedSecurityBatch(ctx, prepared); err != nil {
		return SecurityBatchView{}, err
	}
	id, err := randomID()
	if err != nil {
		return SecurityBatchView{}, err
	}
	s.oauthMu.Lock()
	if s.oauthBusy || s.oauthBatchID != "" {
		s.oauthMu.Unlock()
		return SecurityBatchView{}, errors.New("授权浏览器正在使用，请结束当前操作后重试")
	}
	s.oauthBatchID = id
	s.oauthMu.Unlock()
	s.securityBatches.mu.Lock()
	if s.securityBatches.previews[previewID] != prepared || !time.Now().Before(prepared.expires) {
		s.securityBatches.mu.Unlock()
		s.releaseOAuthBatch(id)
		return SecurityBatchView{}, errors.New("批量安全操作预览已失效，请重新确认")
	}
	delete(s.securityBatches.previews, previewID)
	password := prepared.password
	proxyURL := prepared.proxyURL
	prepared.proxyURL = ""
	prepared.password = ""
	s.securityBatches.mu.Unlock()
	now := time.Now().UTC()
	expires := now.Add(oauthBatchLifetime)
	if len(prepared.items) > 0 && prepared.items[0].source != nil && prepared.expires.Before(expires) {
		expires = prepared.expires
	}
	view := SecurityBatchView{Scope: prepared.view.Scope, ID: id, TaskID: id, Operation: prepared.view.Operation, Status: "queued", Message: securityBatchMessage(prepared.view.Operation), ExpiresAt: expires.Format(time.RFC3339Nano), Items: cloneSecurityRows(prepared.view.Items)}
	job := &securityBatch{owner: owner, target: prepared.target, expires: expires, password: password, items: prepared.items, view: view, done: make(chan struct{})}
	job.users = map[string]bool{}
	job.proxyURL = proxyURL
	view.Items = cloneSecurityRows(view.Items)
	task := taskstore.Task{ID: id, Skill: Skill, Operation: "account-workbench-security-batch", Status: "queued", Message: view.Message, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), Result: securityBatchTaskResult(view)}
	if err := s.tasks.Save(ctx, task); err != nil {
		job.password = ""
		job.proxyURL = ""
		s.releaseOAuthBatch(id)
		return SecurityBatchView{}, err
	}
	s.securityBatches.mu.Lock()
	s.securityBatches.active[id] = job
	s.securityBatches.mu.Unlock()
	job.mu.Lock()
	job.timer = time.AfterFunc(time.Until(expires), func() { _ = s.CancelSecurityBatch(owner, id) })
	job.mu.Unlock()
	if err := taskrunner.GoTask(s.runner, id, func(parent context.Context) { s.runSecurityBatch(parent, job, task) }); err != nil {
		_ = s.CancelSecurityBatch(owner, id)
		close(job.done)
		s.forgetClosingSecurityBatch(id)
		s.releaseOAuthBatch(id)
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return SecurityBatchView{}, err
	}
	return view, nil
}

func (s *Service) ReadSecurityBatch(owner, id string) (SecurityBatchView, error) {
	s.securityBatches.mu.Lock()
	job := s.securityBatches.active[id]
	s.securityBatches.mu.Unlock()
	if job == nil || job.owner != owner || !time.Now().Before(job.expires) {
		return SecurityBatchView{}, browserlogin.ErrSession
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	view := job.view
	view.Items = cloneSecurityRows(view.Items)
	return view, nil
}

func (s *Service) validatePreparedSecurityBatch(ctx context.Context, prepared *preparedSecurityBatch) error {
	if len(prepared.items) > 0 && prepared.items[0].source != nil {
		return s.validateSecuritySources(ctx, prepared.owner, prepared.view.Scope, prepared.items)
	}
	return s.validateSecurityBatch(ctx, prepared.target, prepared.items)
}

func (s *Service) CancelSecurityBatch(owner, id string) error {
	s.securityBatches.mu.Lock()
	job := s.securityBatches.active[id]
	if job == nil || job.owner != owner {
		s.securityBatches.mu.Unlock()
		return browserlogin.ErrSession
	}
	delete(s.securityBatches.active, id)
	select {
	case <-job.done:
	default:
		s.securityBatches.closing[id] = job
	}
	s.securityBatches.mu.Unlock()
	job.mu.Lock()
	job.view.Status, job.view.Message = "cancelled", "批量安全操作已关闭，已执行项目请核对官方状态与私有结果"
	job.password = ""
	job.proxyURL = ""
	current := job.view.CurrentSecurityID
	job.view.CurrentSecurityID = ""
	if job.cancel != nil {
		job.cancel()
	}
	if job.timer != nil {
		job.timer.Stop()
	}
	job.mu.Unlock()
	if current != "" {
		s.removeSecurity(owner, current)
	}
	return nil
}

func (s *Service) closeSecurityBatches() error {
	s.securityBatches.mu.Lock()
	values := make([]*securityBatch, 0, len(s.securityBatches.active))
	for _, job := range s.securityBatches.active {
		values = append(values, job)
	}
	for _, job := range s.securityBatches.closing {
		values = append(values, job)
	}
	for id, preview := range s.securityBatches.previews {
		preview.password = ""
		preview.proxyURL = ""
		delete(s.securityBatches.previews, id)
	}
	s.securityBatches.mu.Unlock()
	for _, job := range values {
		_ = s.CancelSecurityBatch(job.owner, job.view.ID)
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for _, job := range values {
		select {
		case <-job.done:
		case <-deadline.C:
			return errors.New("批量安全任务尚未清理，请等待当前官方操作结束")
		}
	}
	return nil
}

func (s *Service) forgetClosingSecurityBatch(id string) {
	s.securityBatches.mu.Lock()
	delete(s.securityBatches.closing, id)
	s.securityBatches.mu.Unlock()
}
