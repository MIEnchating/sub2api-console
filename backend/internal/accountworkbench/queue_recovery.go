package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type QueueRecoveryView struct {
	Scope     ExportScope         `json:"scope"`
	Items     []QueueRecoveryItem `json:"items"`
	Pending   int                 `json:"pending"`
	Succeeded int                 `json:"succeeded"`
	Review    int                 `json:"review"`
	Active    bool                `json:"active"`
	ID        string              `json:"id"`
	Kind      string              `json:"kind"`
	TaskID    string              `json:"task_id"`
	Status    string              `json:"status"`
	Revision  int64               `json:"revision"`
	ExpiresAt string              `json:"expires_at"`
	CanResume bool                `json:"can_resume"`
}

type QueueRecoveryItem struct {
	Index       int    `json:"index"`
	Email       string `json:"email"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Status      string `json:"status"`
}

type QueueRecoveryAction struct {
	Revision  int64       `json:"revision"`
	Confirmed bool        `json:"confirmed"`
	Scope     ExportScope `json:"scope,omitempty"`
}

func (s *Service) QueueRecoveries(ctx context.Context, owner string, scope ExportScope) ([]QueueRecoveryView, error) {
	if owner == "" {
		return nil, configstore.ErrWorkbenchQueue
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return nil, err
	}
	store, err := s.queueStore()
	if err != nil {
		return nil, err
	}
	if err := store.PurgeExpiredWorkbenchQueues(ctx, time.Now()); err != nil {
		return nil, err
	}
	records, err := store.WorkbenchQueues(ctx, exportHash(owner), workbenchScopeFingerprint(scope, target))
	if err != nil {
		return nil, err
	}
	views := make([]QueueRecoveryView, 0, len(records))
	for _, record := range records {
		current, err := store.WorkbenchQueue(ctx, record.Owner, record.Target, record.ID)
		if errors.Is(err, configstore.ErrWorkbenchQueue) {
			continue
		}
		if err != nil {
			return nil, err
		}
		view, err := queueRecoverySummary(current, scope, s.queueRecoveryActive(current))
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) DeleteQueueRecovery(ctx context.Context, owner, id string, input QueueRecoveryAction) error {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if owner == "" || !input.Confirmed || input.Revision < 1 {
		return configstore.ErrWorkbenchQueue
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, input.Scope, nil)
	if err != nil {
		return err
	}
	store, err := s.queueStore()
	if err != nil {
		return err
	}
	record, err := store.WorkbenchQueue(ctx, exportHash(owner), workbenchScopeFingerprint(input.Scope, target), id)
	if err != nil || record.Revision != input.Revision {
		return configstore.ErrWorkbenchQueue
	}
	if record.Status == "running" || s.queueRecoveryActive(record) {
		return errors.New("请先结束活动批次再删除恢复资料")
	}
	return store.DeleteWorkbenchQueue(ctx, record.Owner, record.Target, record.ID, record.Revision)
}

func (s *Service) ResumeOAuthQueue(ctx context.Context, owner, id string, input QueueRecoveryAction) (OAuthBatchView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if owner == "" || !input.Confirmed || input.Revision < 1 {
		return OAuthBatchView{}, configstore.ErrWorkbenchQueue
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, input.Scope, nil)
	if err != nil {
		return OAuthBatchView{}, err
	}
	store, err := s.queueStore()
	if err != nil {
		return OAuthBatchView{}, err
	}
	record, err := store.WorkbenchQueue(ctx, exportHash(owner), workbenchScopeFingerprint(input.Scope, target), id)
	if err != nil || record.Revision != input.Revision || record.Kind != "oauth-batch" {
		return OAuthBatchView{}, configstore.ErrWorkbenchQueue
	}
	if view, active, err := s.reconnectOAuthQueue(ctx, owner, input.Scope, target, record); active || err != nil {
		return view, err
	}
	if record.Status == "running" {
		return OAuthBatchView{}, configstore.ErrWorkbenchQueue
	}
	var payload oauthQueuePayload
	if json.Unmarshal(record.Payload, &payload) != nil || payload.Version != 1 || len(payload.View.Items) == 0 || len(payload.View.Items) > maxInputItems || len(payload.Inputs) != len(payload.View.Items) {
		return OAuthBatchView{}, configstore.ErrWorkbenchQueue
	}
	storedScope, scopeErr := normalizeWorkbenchScope(payload.View.Scope)
	requestedScope, _ := normalizeWorkbenchScope(input.Scope)
	if scopeErr != nil || storedScope != requestedScope {
		return OAuthBatchView{}, configstore.ErrWorkbenchQueue
	}
	results, err := s.validateOAuthQueuePayload(ctx, owner, storedScope, target, &payload)
	if err != nil {
		return OAuthBatchView{}, err
	}
	s.batches.mu.Lock()
	active := s.batches.active[record.TaskID] != nil || len(s.batches.active) >= 10
	s.batches.mu.Unlock()
	if active {
		return OAuthBatchView{}, errors.New("原批次仍在当前进程，不能重复恢复")
	}
	taskID, err := randomID()
	if err != nil {
		return OAuthBatchView{}, err
	}
	s.oauthMu.Lock()
	if s.oauthBusy || s.oauthBatchID != "" {
		s.oauthMu.Unlock()
		return OAuthBatchView{}, errors.New("授权浏览器仍在使用，请结束后恢复")
	}
	s.oauthBatchID = taskID
	s.oauthMu.Unlock()
	launched := false
	defer func() {
		if !launched {
			s.releaseOAuthBatch(taskID)
		}
	}()
	record.Status, record.TaskID = "running", taskID
	saved, err := saveQueueRecord(ctx, store, record)
	if err != nil {
		return OAuthBatchView{}, err
	}
	defer func() {
		if launched {
			return
		}
		persist, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		record.Revision, record.Status = saved.Revision, "interrupted"
		_, _ = saveQueueRecord(persist, store, record)
	}()
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil {
		return OAuthBatchView{}, configstore.ErrWorkbenchQueue
	}
	view := payload.View
	view.ID, view.TaskID, view.CurrentOAuthID = taskID, taskID, ""
	view.Status, view.Message = "queued", "等待恢复未执行的授权账号"
	view.ExpiresAt, view.RecoveryEnabled, view.RecoveryID = record.ExpiresAt, true, record.ID
	view.Available = len(results)
	job := &oauthBatch{checkpoints: payload.Checkpoints, scope: storedScope, owner: owner, target: target, expires: expires, inputs: payload.Inputs, results: results, view: view, done: make(chan struct{}), queue: &saved}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: taskID, Skill: Skill, Operation: "account-workbench-oauth-batch", Status: "queued", Message: view.Message, CreatedAt: now, UpdatedAt: now, Result: batchTaskResult(view)}
	if err := s.tasks.Save(ctx, task); err != nil {
		return OAuthBatchView{}, err
	}
	s.batches.mu.Lock()
	s.batches.active[taskID] = job
	s.batches.mu.Unlock()
	job.mu.Lock()
	job.timer = time.AfterFunc(time.Until(expires), func() { _ = s.CancelOAuthBatch(owner, taskID) })
	view.Items = append([]OAuthBatchRow(nil), view.Items...)
	job.mu.Unlock()
	if err := taskrunner.GoTask(s.runner, taskID, func(parent context.Context) { s.runOAuthBatch(parent, job, task) }); err != nil {
		s.batches.mu.Lock()
		delete(s.batches.active, taskID)
		s.batches.mu.Unlock()
		job.mu.Lock()
		job.timer.Stop()
		job.mu.Unlock()
		close(job.done)
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return OAuthBatchView{}, err
	}
	launched = true
	return view, nil
}
