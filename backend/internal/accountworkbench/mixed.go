package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) PreviewWorkbenchRun(ctx context.Context, owner string, input WorkbenchRunInput) (WorkbenchRunPreview, error) {
	view := WorkbenchRunPreview{Items: []WorkbenchRunRow{}, Errors: []InputError{}, ExportOnly: input.ExportOnly}
	if owner == "" {
		return view, browserlogin.ErrSession
	}
	entries, failures := parseWorkbenchRun(input.Content)
	if len(failures) > 0 {
		view.Errors = failures
		return view, nil
	}
	if err := browserlogin.ValidateProxyURL(input.ProxyURL); err != nil {
		return view, err
	}
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return view, err
	}
	if scope == ScopeLocalExport {
		if !input.ExportOnly || input.TemplateID != "" || input.CheckAfterImport {
			return view, errors.New("本地混合运行仅支持私有导出，不使用管理模板或检测")
		}
	}
	view.Scope, view.RecoveryEnabled = scope, input.RecoveryEnabled
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "gpt-5.6-sol"
	}
	if len(model) > 200 || strings.ContainsAny(model, "\r\n") {
		return view, errors.New("检测模型无效")
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return view, err
	}
	var templates []configstore.WorkbenchTemplate
	if scope != ScopeLocalExport {
		templates, err = s.private.WorkbenchTemplates(ctx, target.BaseURL)
		if err != nil {
			return view, err
		}
	}
	versions := map[string]int64{}
	for _, template := range templates {
		versions[template.ID] = template.Revision
	}
	if input.TemplateID != "" && versions[input.TemplateID] == 0 {
		return view, errors.New("所选模板不存在，请刷新后重试")
	}
	logins := []OAuthLoginInput{}
	positions := []int{}
	for _, entry := range entries {
		row := WorkbenchRunRow{Index: entry.index, Status: "queued", Message: "等待处理"}
		if entry.item != nil {
			row.Kind, row.Name, row.Email = entry.item.Kind, entry.item.Name, entry.item.Email
			row.WorkspaceID = stringValue(entry.item.Credentials["chatgpt_account_id"])
		} else {
			row.Kind, row.Name, row.Email, row.WorkspaceID = "oauth_login", entry.login.Email, entry.login.Email, entry.login.WorkspaceID
			logins = append(logins, *entry.login)
			positions = append(positions, entry.index)
		}
		view.Items = append(view.Items, row)
	}
	oauthPreviewID := ""
	if len(logins) > 0 {
		raw, err := json.Marshal(logins)
		if err != nil {
			return view, errors.New("授权资料无法解析，请重新输入")
		}
		oauth, err := s.PreviewOAuthBatch(ctx, owner, OAuthBatchPreviewInput{Scope: scope, RecoveryEnabled: input.RecoveryEnabled, Content: string(raw), ProxyURL: input.ProxyURL, SMS: input.SMS})
		if err != nil {
			return view, err
		}
		if len(oauth.Errors) > 0 {
			for _, problem := range oauth.Errors {
				if problem.Index < len(positions) {
					problem.Index = positions[problem.Index]
				}
				view.Errors = append(view.Errors, problem)
			}
			view.Items = []WorkbenchRunRow{}
			return view, nil
		}
		oauthPreviewID = oauth.ID
		for i, login := range oauth.Items {
			row := &view.Items[positions[i]]
			row.HasPassword, row.HasTOTP, row.MailKind, row.SMSProvider = login.HasPassword, login.HasTOTP, login.MailKind, login.SMSProvider
			row.HasProxy = logins[i].ProxyURL != "" || input.ProxyURL != ""
		}
	}
	if _, _, err := s.bindWorkbenchScope(ctx, scope, &target); err != nil {
		s.DeleteOAuthBatchPreview(owner, oauthPreviewID)
		return view, err
	}
	view.ID, err = randomID()
	if err != nil {
		s.DeleteOAuthBatchPreview(owner, oauthPreviewID)
		return view, err
	}
	expires := time.Now().Add(10 * time.Minute)
	view.ExpiresAt, view.Target = expires.UTC().Format(time.RFC3339Nano), target.BaseURL
	prepared := &preparedWorkbenchRun{owner: owner, target: target, expires: expires, entries: entries, options: PreviewInput{Scope: scope, TemplateID: input.TemplateID, Model: model, ProxyURL: input.ProxyURL, ExportOnly: input.ExportOnly, CheckAfterImport: input.CheckAfterImport && !input.ExportOnly}, oauthPreviewID: oauthPreviewID, view: view, loadedTemplates: versions}
	if oauthPreviewID != "" {
		s.batches.mu.Lock()
		prepared.oauth = s.batches.previews[oauthPreviewID]
		delete(s.batches.previews, oauthPreviewID)
		s.batches.mu.Unlock()
		if prepared.oauth == nil || prepared.oauth.owner != owner {
			return WorkbenchRunPreview{}, ErrPreview
		}
	}
	// This mixed batch owns the login preparation until its runner consumes it.
	for i := range prepared.entries {
		prepared.entries[i].login = nil
	}
	s.mixed.mu.Lock()
	if len(s.mixed.previews) >= 20 {
		s.mixed.mu.Unlock()
		s.DeleteOAuthBatchPreview(owner, oauthPreviewID)
		return WorkbenchRunPreview{}, errors.New("混合输入预览已满，请关闭已有预览")
	}
	s.mixed.previews[view.ID] = prepared
	s.mixed.mu.Unlock()
	time.AfterFunc(time.Until(expires), func() { s.DeleteWorkbenchRunPreview(owner, view.ID) })
	view.Items = append([]WorkbenchRunRow(nil), view.Items...)
	return view, nil
}

func (s *Service) DeleteWorkbenchRunPreview(owner, id string) {
	s.mixed.mu.Lock()
	prepared := s.mixed.previews[id]
	if prepared == nil || prepared.owner != owner {
		s.mixed.mu.Unlock()
		return
	}
	delete(s.mixed.previews, id)
	s.mixed.mu.Unlock()
	s.DeleteOAuthBatchPreview(owner, prepared.oauthPreviewID)
}

func (s *Service) StartWorkbenchRun(ctx context.Context, owner, previewID string, confirmed bool) (WorkbenchRunView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if !confirmed {
		return WorkbenchRunView{}, errors.New("请先确认混合批次范围及短信费用")
	}
	s.mixed.mu.Lock()
	prepared := s.mixed.previews[previewID]
	if prepared == nil || prepared.owner != owner || time.Now().After(prepared.expires) {
		s.mixed.mu.Unlock()
		return WorkbenchRunView{}, ErrPreview
	}
	if len(s.mixed.active) >= 10 {
		s.mixed.mu.Unlock()
		return WorkbenchRunView{}, errors.New("混合输入批次已满，请关闭已完成批次")
	}
	s.mixed.mu.Unlock()
	if _, _, err := s.bindWorkbenchScope(ctx, prepared.options.Scope, &prepared.target); err != nil {
		return WorkbenchRunView{}, err
	}
	if err := s.validateMixedTemplates(ctx, prepared); err != nil {
		return WorkbenchRunView{}, err
	}
	id, err := randomID()
	if err != nil {
		return WorkbenchRunView{}, err
	}
	now := time.Now().UTC()
	view := WorkbenchRunView{ID: id, TaskID: id, Status: "queued", Message: "等待处理混合账号输入", ExpiresAt: now.Add(oauthBatchLifetime).Format(time.RFC3339Nano), ExportOnly: prepared.options.ExportOnly, Items: append([]WorkbenchRunRow(nil), prepared.view.Items...), Errors: []InputError{}}
	view.Scope = prepared.options.Scope
	job := &workbenchRun{prepared: prepared, view: view, done: make(chan struct{}), expires: now.Add(oauthBatchLifetime)}
	s.mixed.mu.Lock()
	if s.mixed.previews[previewID] != prepared {
		s.mixed.mu.Unlock()
		return WorkbenchRunView{}, ErrPreview
	}
	if err := s.prepareMixedQueue(ctx, job); err != nil {
		s.mixed.mu.Unlock()
		return WorkbenchRunView{}, err
	}
	view = job.view
	view.Items = append([]WorkbenchRunRow(nil), view.Items...)
	task := taskstore.Task{ID: id, Skill: Skill, Operation: "account-workbench-mixed", Status: "queued", Message: view.Message, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), Result: mixedTaskResult(view)}
	delete(s.mixed.previews, previewID)
	s.mixed.active[id] = job
	s.mixed.mu.Unlock()
	if err := s.tasks.Save(ctx, task); err != nil {
		_ = s.CancelWorkbenchRun(owner, id)
		return WorkbenchRunView{}, err
	}
	job.mu.Lock()
	job.timer = time.AfterFunc(oauthBatchLifetime, func() { _ = s.CancelWorkbenchRun(owner, id) })
	job.mu.Unlock()
	if err := taskrunner.GoTask(s.runner, id, func(parent context.Context) { s.runWorkbenchInput(parent, job, task) }); err != nil {
		_ = s.CancelWorkbenchRun(owner, id)
		close(job.done)
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return WorkbenchRunView{}, err
	}
	return view, nil
}

func (s *Service) mixedRun(owner, id string) (*workbenchRun, error) {
	s.mixed.mu.Lock()
	job := s.mixed.active[id]
	s.mixed.mu.Unlock()
	if job == nil || job.prepared.owner != owner {
		return nil, browserlogin.ErrSession
	}
	return job, nil
}

func (s *Service) ReadWorkbenchRun(ctx context.Context, owner, id string) (WorkbenchRunView, error) {
	job, err := s.mixedRun(owner, id)
	if err != nil {
		return WorkbenchRunView{}, err
	}
	if _, _, err := s.bindWorkbenchScope(ctx, job.prepared.options.Scope, &job.prepared.target); err != nil {
		return WorkbenchRunView{}, err
	}
	job.mu.Lock()
	view := job.view
	view.Items = append([]WorkbenchRunRow(nil), view.Items...)
	view.Errors = append([]InputError{}, view.Errors...)
	job.mu.Unlock()
	if view.OAuthBatchID != "" && (view.Status == "running" || view.Status == "waiting_input") {
		if child, err := s.ReadOAuthBatch(owner, view.OAuthBatchID); err == nil {
			view.CurrentOAuthID = child.CurrentOAuthID
		}
	}
	return view, nil
}

func (s *Service) CancelWorkbenchRun(owner, id string) error {
	s.mixed.mu.Lock()
	job := s.mixed.active[id]
	if job == nil || job.prepared.owner != owner {
		s.mixed.mu.Unlock()
		return browserlogin.ErrSession
	}
	delete(s.mixed.active, id)
	s.mixed.mu.Unlock()
	job.mu.Lock()
	deleteErr := s.deleteMixedQueue(job)
	job.ready = false
	job.items = nil
	job.view.Status, job.view.Message, job.view.Available = "cancelled", "混合输入批次已关闭", 0
	if job.cancel != nil {
		job.cancel()
	}
	if job.timer != nil {
		job.timer.Stop()
	}
	batchID := job.view.OAuthBatchID
	previews := append([]string(nil), job.previews...)
	job.mu.Unlock()
	s.DeleteOAuthBatchPreview(owner, job.prepared.oauthPreviewID)
	if batchID != "" {
		_ = s.CancelOAuthBatch(owner, batchID)
	}
	for _, preview := range previews {
		s.DeletePreview(owner, preview)
	}
	return deleteErr
}

func (s *Service) validateMixedTemplates(ctx context.Context, prepared *preparedWorkbenchRun) error {
	if prepared.options.Scope == ScopeLocalExport {
		if prepared.options.TemplateID != "" || !prepared.options.ExportOnly {
			return ErrPreview
		}
		return nil
	}
	current, err := s.private.WorkbenchTemplates(ctx, prepared.target.BaseURL)
	if err != nil {
		return err
	}
	if len(current) != len(prepared.loadedTemplates) {
		return errors.New("模板列表已变化，请重新解析混合输入")
	}
	for _, template := range current {
		if prepared.loadedTemplates[template.ID] != template.Revision {
			return errors.New("模板配置已变化，请重新解析混合输入")
		}
	}
	return nil
}

func mixedTaskResult(view WorkbenchRunView) map[string]any {
	return map[string]any{"request_id": view.ID, "phase": view.Status, "items": append([]WorkbenchRunRow(nil), view.Items...), "errors": append([]InputError{}, view.Errors...), "available": view.Available, "export_only": view.ExportOnly, "recovery_id": view.RecoveryID}
}
