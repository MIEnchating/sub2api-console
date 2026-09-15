package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const oauthBatchLifetime = 2 * time.Hour

type OAuthBatchPreviewInput struct {
	Scope           ExportScope    `json:"scope,omitempty"`
	RecoveryEnabled bool           `json:"recovery_enabled,omitempty"`
	ProxyURL        string         `json:"proxy_url,omitempty"`
	Content         string         `json:"content"`
	SMS             *OAuthSMSInput `json:"sms,omitempty"`
}

type OAuthBatchRow struct {
	AccountID       string `json:"account_id,omitempty"`
	UserID          string `json:"user_id,omitempty"`
	ProfileID       string `json:"profile_id,omitempty"`
	ProfileRevision int64  `json:"profile_revision,omitempty"`
	Index           int    `json:"index"`
	Email           string `json:"email"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	HasPassword     bool   `json:"has_password"`
	HasTOTP         bool   `json:"has_totp"`
	MailKind        string `json:"mail_kind,omitempty"`
	SMSProvider     string `json:"sms_provider,omitempty"`
	Status          string `json:"status"`
	Message         string `json:"message"`
}

type OAuthBatchPreview struct {
	Scope           ExportScope     `json:"scope,omitempty"`
	RecoveryEnabled bool            `json:"recovery_enabled,omitempty"`
	FreshLogin      bool            `json:"fresh_login,omitempty"`
	ID              string          `json:"id"`
	ExpiresAt       string          `json:"expires_at"`
	Target          string          `json:"target"`
	Items           []OAuthBatchRow `json:"items"`
	Errors          []InputError    `json:"errors"`
}

type OAuthBatchView struct {
	Scope           ExportScope     `json:"scope,omitempty"`
	RecoveryEnabled bool            `json:"recovery_enabled,omitempty"`
	RecoveryID      string          `json:"recovery_id,omitempty"`
	FreshLogin      bool            `json:"fresh_login,omitempty"`
	ID              string          `json:"id"`
	TaskID          string          `json:"task_id"`
	Status          string          `json:"status"`
	Message         string          `json:"message"`
	ExpiresAt       string          `json:"expires_at"`
	CurrentOAuthID  string          `json:"current_oauth_id,omitempty"`
	Available       int             `json:"available"`
	Items           []OAuthBatchRow `json:"items"`
}

type preparedOAuthBatch struct {
	sourceProfiles     []sourceProfileAuthorization
	parentQueuePersist func(context.Context, *oauthBatch, string) error
	resumePayload      *oauthQueuePayload
	scope              ExportScope
	profiles           []*profileAuthorization
	owner              string
	target             configstore.TargetSettings
	expires            time.Time
	inputs             []OAuthLoginInput
	view               OAuthBatchPreview
}

type oauthBatch struct {
	sourceProfiles     []sourceProfileAuthorization
	stopRequested      bool
	checkpoints        map[int]oauthQueueCheckpoint
	parentQueuePersist func(context.Context, *oauthBatch, string) error
	scope              ExportScope
	queue              *configstore.WorkbenchQueue
	queueFrozen        bool
	done               chan struct{}
	profiles           []*profileAuthorization
	mu                 sync.Mutex
	owner              string
	target             configstore.TargetSettings
	expires            time.Time
	inputs             []OAuthLoginInput
	results            []InputItem
	ready              bool
	view               OAuthBatchView
	previews           []string
	cancel             context.CancelFunc
	timer              *time.Timer
}

type oauthBatches struct {
	mu       sync.Mutex
	previews map[string]*preparedOAuthBatch
	active   map[string]*oauthBatch
}

func newOAuthBatches() *oauthBatches {
	return &oauthBatches{previews: make(map[string]*preparedOAuthBatch), active: make(map[string]*oauthBatch)}
}

func (s *Service) PreviewOAuthBatch(ctx context.Context, owner string, input OAuthBatchPreviewInput) (OAuthBatchPreview, error) {
	view := OAuthBatchPreview{Items: []OAuthBatchRow{}, Errors: []InputError{}, RecoveryEnabled: input.RecoveryEnabled}
	if owner == "" {
		return view, browserlogin.ErrSession
	}
	inputs, failures := ParseOAuthLogins(input.Content)
	if len(failures) > 0 {
		view.Errors = failures
		return view, nil
	}
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return view, err
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return view, err
	}
	view.Scope = scope
	for i := range inputs {
		if inputs[i].ProxyURL == "" {
			inputs[i].ProxyURL = input.ProxyURL
		}
		if err := browserlogin.ValidateProxyURL(inputs[i].ProxyURL); err != nil {
			return view, err
		}
		if inputs[i].SMS == nil && input.SMS != nil {
			copy := *input.SMS
			inputs[i].SMS = &copy
		}
		validation := inputs[i]
		if validation.SMS != nil {
			copy := *validation.SMS
			copy.Confirmed = true
			validation.SMS = &copy
		}
		assist, err := s.prepareOAuthAssist(&validation)
		if err != nil {
			view.Errors = append(view.Errors, InputError{Index: i, Message: err.Error()})
			continue
		}
		assist.close()
		row := OAuthBatchRow{Index: i, Email: inputs[i].Email, WorkspaceID: inputs[i].WorkspaceID, HasPassword: inputs[i].Password != "", HasTOTP: inputs[i].TOTPSecret != "", Status: "queued", Message: "等待授权"}
		if inputs[i].Mailbox != nil {
			row.MailKind = inputs[i].Mailbox.Kind
		}
		if inputs[i].SMS != nil {
			row.SMSProvider = inputs[i].SMS.Provider
		}
		view.Items = append(view.Items, row)
	}
	if len(view.Errors) > 0 {
		view.Items = []OAuthBatchRow{}
		return view, nil
	}
	if _, _, err := s.bindWorkbenchScope(ctx, scope, &target); err != nil {
		return OAuthBatchPreview{}, err
	}
	view.ID, err = randomID()
	if err != nil {
		return view, err
	}
	expires := time.Now().Add(10 * time.Minute)
	view.ExpiresAt, view.Target = expires.UTC().Format(time.RFC3339Nano), target.BaseURL
	prepared := &preparedOAuthBatch{scope: scope, owner: owner, target: target, expires: expires, inputs: inputs, view: view}
	s.batches.mu.Lock()
	for id, existing := range s.batches.previews {
		if existing.owner == owner || time.Now().After(existing.expires) {
			delete(s.batches.previews, id)
		}
	}
	if len(s.batches.previews) >= 20 {
		s.batches.mu.Unlock()
		return OAuthBatchPreview{}, errors.New("批量授权预览已满，请稍后重试")
	}
	s.batches.previews[view.ID] = prepared
	s.batches.mu.Unlock()
	time.AfterFunc(time.Until(expires), func() { s.DeleteOAuthBatchPreview(owner, view.ID) })
	view.Items = append([]OAuthBatchRow(nil), view.Items...)
	return view, nil
}

func (s *Service) DeleteOAuthBatchPreview(owner, id string) {
	s.batches.mu.Lock()
	defer s.batches.mu.Unlock()
	if value := s.batches.previews[id]; value != nil && value.owner == owner {
		delete(s.batches.previews, id)
	}
}

func (s *Service) StartOAuthBatch(ctx context.Context, owner, previewID string, confirmed bool) (OAuthBatchView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if !confirmed {
		return OAuthBatchView{}, errors.New("请先确认批量授权账号范围及短信费用")
	}
	s.batches.mu.Lock()
	prepared := s.batches.previews[previewID]
	if prepared == nil || prepared.owner != owner || time.Now().After(prepared.expires) {
		s.batches.mu.Unlock()
		return OAuthBatchView{}, errors.New("批量授权预览已失效，请重新解析")
	}
	if len(s.batches.active) >= 10 {
		s.batches.mu.Unlock()
		return OAuthBatchView{}, errors.New("批量授权结果已满，请关闭已有批次")
	}
	s.batches.mu.Unlock()
	if _, _, err := s.bindWorkbenchScope(ctx, prepared.scope, &prepared.target); err != nil {
		return OAuthBatchView{}, err
	}
	if err := s.validateReauthorizationProfiles(ctx, prepared.target, prepared.profiles); err != nil {
		return OAuthBatchView{}, err
	}
	if err := s.validateSourceProfileAuthorizations(ctx, owner, prepared.sourceProfiles); err != nil {
		return OAuthBatchView{}, err
	}
	id, err := randomID()
	if err != nil {
		return OAuthBatchView{}, err
	}
	s.oauthMu.Lock()
	if s.oauthBusy || s.oauthBatchID != "" {
		s.oauthMu.Unlock()
		return OAuthBatchView{}, errors.New("授权浏览器正在使用，请结束当前授权后重试")
	}
	s.oauthBatchID = id
	s.oauthMu.Unlock()
	s.batches.mu.Lock()
	if s.batches.previews[previewID] != prepared || time.Now().After(prepared.expires) {
		s.batches.mu.Unlock()
		s.releaseOAuthBatch(id)
		return OAuthBatchView{}, errors.New("批量授权预览已失效，请重新解析")
	}
	delete(s.batches.previews, previewID)
	s.batches.mu.Unlock()
	now := time.Now().UTC()
	view := OAuthBatchView{ID: id, TaskID: id, Status: "queued", Message: "等待批量授权", ExpiresAt: now.Add(oauthBatchLifetime).Format(time.RFC3339Nano), Items: append([]OAuthBatchRow(nil), prepared.view.Items...), FreshLogin: prepared.view.FreshLogin}
	view.Scope = prepared.scope
	job := &oauthBatch{parentQueuePersist: prepared.parentQueuePersist, scope: prepared.scope, owner: owner, target: prepared.target, expires: now.Add(oauthBatchLifetime), inputs: prepared.inputs, view: view, profiles: prepared.profiles, done: make(chan struct{})}
	job.sourceProfiles = prepared.sourceProfiles
	if prepared.resumePayload != nil {
		payload := prepared.resumePayload
		results, restoreErr := s.validateOAuthQueuePayload(ctx, owner, prepared.scope, prepared.target, payload)
		if restoreErr != nil {
			s.releaseOAuthBatch(id)
			return OAuthBatchView{}, restoreErr
		}
		expires, expiryErr := time.Parse(time.RFC3339Nano, payload.View.ExpiresAt)
		if expiryErr != nil || !time.Now().Before(expires) {
			s.releaseOAuthBatch(id)
			return OAuthBatchView{}, configstore.ErrWorkbenchQueue
		}
		job.expires, job.inputs, job.results, job.checkpoints = expires, payload.Inputs, results, payload.Checkpoints
		job.view.Items, job.view.Available, job.view.ExpiresAt = payload.View.Items, len(results), payload.View.ExpiresAt
		view = job.view
	}
	if prepared.view.RecoveryEnabled {
		if len(prepared.profiles) > 0 {
			s.releaseOAuthBatch(id)
			return OAuthBatchView{}, errors.New("资料绑定批次请从已保存登录资料重新授权")
		}
		job.view.RecoveryEnabled, job.view.RecoveryID = true, id
		view.RecoveryEnabled, view.RecoveryID = true, id
		for i := range job.inputs {
			if job.inputs[i].SMS != nil {
				job.inputs[i].SMS.Confirmed = true
			}
		}
		if job.parentQueuePersist == nil {
			job.queue = &configstore.WorkbenchQueue{ID: id, Owner: exportHash(owner), Target: workbenchScopeFingerprint(prepared.scope, prepared.target), Kind: "oauth-batch", TaskID: id, Status: "running", CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: job.expires.UTC().Format(time.RFC3339Nano)}
		}
		if err := s.persistOAuthQueue(ctx, job, "running"); err != nil {
			s.releaseOAuthBatch(id)
			return OAuthBatchView{}, err
		}
	}
	view.Items = append([]OAuthBatchRow(nil), view.Items...)
	for i := range job.inputs {
		if job.inputs[i].SMS != nil {
			job.inputs[i].SMS.Confirmed = true
		}
	}
	task := taskstore.Task{ID: id, Skill: Skill, Operation: "account-workbench-oauth-batch", Status: "queued", Message: view.Message, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), Result: batchTaskResult(view)}
	if err := s.tasks.Save(ctx, task); err != nil {
		persist, done := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.persistOAuthQueue(persist, job, "interrupted")
		done()
		s.releaseOAuthBatch(id)
		return OAuthBatchView{}, err
	}
	s.batches.mu.Lock()
	s.batches.active[id] = job
	s.batches.mu.Unlock()
	job.mu.Lock()
	job.timer = time.AfterFunc(time.Until(job.expires), func() { _ = s.CancelOAuthBatch(owner, id) })
	job.mu.Unlock()
	if err := taskrunner.GoTask(s.runner, id, func(parent context.Context) { s.runOAuthBatch(parent, job, task) }); err != nil {
		persist, done := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.persistOAuthQueue(persist, job, "interrupted")
		done()
		s.batches.mu.Lock()
		delete(s.batches.active, id)
		s.batches.mu.Unlock()
		job.mu.Lock()
		job.timer.Stop()
		job.mu.Unlock()
		close(job.done)
		s.releaseOAuthBatch(id)
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return OAuthBatchView{}, err
	}
	return view, nil
}

func batchTaskResult(view OAuthBatchView) map[string]any {
	return map[string]any{"request_id": view.ID, "phase": view.Status, "items": append([]OAuthBatchRow(nil), view.Items...), "available": view.Available, "recovery_id": view.RecoveryID}
}

func (s *Service) releaseOAuthBatch(id string) {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	if s.oauthBatchID == id {
		s.oauthBatchID = ""
	}
}

func (s *Service) oauthBatch(owner, id string) (*oauthBatch, error) {
	s.batches.mu.Lock()
	job := s.batches.active[id]
	s.batches.mu.Unlock()
	if job == nil || job.owner != owner || time.Now().After(job.expires) {
		return nil, browserlogin.ErrSession
	}
	return job, nil
}

func (s *Service) ReadOAuthBatch(owner, id string) (OAuthBatchView, error) {
	job, err := s.oauthBatch(owner, id)
	if err != nil {
		return OAuthBatchView{}, err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	view := job.view
	view.Items = append([]OAuthBatchRow(nil), view.Items...)
	return view, nil
}

func (s *Service) CancelOAuthBatch(owner, id string) error {
	s.batches.mu.Lock()
	job := s.batches.active[id]
	if job == nil || job.owner != owner {
		s.batches.mu.Unlock()
		return browserlogin.ErrSession
	}
	delete(s.batches.active, id)
	s.batches.mu.Unlock()
	job.mu.Lock()
	deleteErr := s.deleteOAuthQueue(job)
	job.view.Status, job.view.Message = "cancelled", "批量授权已关闭"
	job.ready = false
	job.view.Available = 0
	job.inputs = nil
	job.results = nil
	current := job.view.CurrentOAuthID
	job.view.CurrentOAuthID = ""
	for i := range job.view.Items {
		if job.view.Items[i].Status == "queued" || job.view.Items[i].Status == "running" {
			job.view.Items[i].Status, job.view.Items[i].Message = "cancelled", "本批已结束，未继续授权"
		}
	}
	if job.cancel != nil {
		job.cancel()
	}
	if job.timer != nil {
		job.timer.Stop()
	}
	previews := append([]string(nil), job.previews...)
	job.mu.Unlock()
	if current != "" {
		_ = s.CancelOAuth(owner, current)
	}
	for _, preview := range previews {
		s.DeletePreview(owner, preview)
	}
	return deleteErr
}

func (s *Service) PreviewOAuthBatchImport(ctx context.Context, owner, id string, input OAuthPreviewInput) (Preview, error) {
	job, err := s.oauthBatch(owner, id)
	if err != nil {
		return Preview{}, err
	}
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return Preview{}, err
	}
	jobScope, _ := normalizeWorkbenchScope(job.scope)
	if scope != jobScope || scope == ScopeLocalExport && !input.ExportOnly {
		return Preview{}, errors.New("本地授权批次只能在本地私有导出范围预览")
	}
	job.mu.Lock()
	if job.view.Status != "authorized" || !job.ready || len(job.results) == 0 {
		job.mu.Unlock()
		return Preview{}, errors.New("请等待批量授权结束并取得有效账号")
	}
	credentials := make([]map[string]any, 0, len(job.results))
	indexes := make([]int, 0, len(job.results))
	bindings := make([]*profileAuthorization, 0, len(job.results))
	for _, result := range job.results {
		credentials = append(credentials, map[string]any{"email": result.Email, "credentials": result.Credentials})
		indexes = append(indexes, result.Index)
		if len(job.profiles) > result.Index {
			bindings = append(bindings, job.profiles[result.Index])
		}
	}
	raw, err := json.Marshal(credentials)
	job.mu.Unlock()
	if err != nil {
		return Preview{}, errors.New("批量授权结果无法读取，请重新授权")
	}
	if err := s.validateReauthorizationProfiles(ctx, job.target, bindings); err != nil {
		return Preview{}, err
	}
	previewCtx, _, err := s.bindWorkbenchScope(ctx, job.scope, &job.target)
	if err != nil {
		return Preview{}, err
	}
	preview, err := s.Preview(previewCtx, owner, PreviewInput{Scope: scope, ExportOnly: input.ExportOnly, Content: string(raw), TemplateID: input.TemplateID, CheckAfterImport: input.CheckAfterImport, Model: input.Model})
	if err != nil {
		return Preview{}, err
	}
	if len(bindings) > 0 {
		if err := s.validateReauthorizationProfiles(ctx, job.target, bindings); err != nil {
			s.DeletePreview(owner, preview.ID)
			return Preview{}, err
		}
		if len(preview.Items) != len(bindings) {
			s.DeletePreview(owner, preview.ID)
			return Preview{}, errors.New("重新授权结果与原账号范围不一致，请重新核对")
		}
		for i, binding := range bindings {
			if !preview.Items[i].Duplicate || preview.Items[i].AccountID != binding.identity.accountID {
				s.DeletePreview(owner, preview.ID)
				return Preview{}, errors.New("重新授权只能更新原稳定账号，目标账号已变化，请重新核对")
			}
		}
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.view.Status != "authorized" || !job.ready || time.Now().After(job.expires) {
		s.DeletePreview(owner, preview.ID)
		return Preview{}, browserlogin.ErrSession
	}
	for i := range preview.Errors {
		if preview.Errors[i].Index < len(indexes) {
			preview.Errors[i].Index = indexes[preview.Errors[i].Index]
		}
	}
	if preview.ID == "" {
		return preview, nil
	}
	s.mu.Lock()
	prepared := s.previews[preview.ID]
	if prepared == nil || prepared.owner != owner || len(prepared.items) != len(indexes) {
		s.mu.Unlock()
		return Preview{}, ErrPreview
	}
	for i := range prepared.items {
		prepared.items[i].Index = indexes[i]
		preview.Items[i].Index, preview.Items[i].ID = indexes[i], strconv.Itoa(indexes[i])
	}
	prepared.view = preview
	prepared.profiles = bindings
	s.mu.Unlock()
	job.previews = append(job.previews, preview.ID)
	return preview, nil
}
