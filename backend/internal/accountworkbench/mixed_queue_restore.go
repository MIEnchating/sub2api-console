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

func (s *Service) ResumeWorkbenchQueue(ctx context.Context, owner, id string, input QueueRecoveryAction) (WorkbenchRunView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if owner == "" || !input.Confirmed || input.Revision < 1 {
		return WorkbenchRunView{}, configstore.ErrWorkbenchQueue
	}
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return WorkbenchRunView{}, err
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, scope, nil)
	if err != nil {
		return WorkbenchRunView{}, err
	}
	store, err := s.queueStore()
	if err != nil {
		return WorkbenchRunView{}, err
	}
	record, err := store.WorkbenchQueue(ctx, exportHash(owner), workbenchScopeFingerprint(scope, target), id)
	if err != nil || record.Revision != input.Revision || record.Kind != "mixed" {
		return WorkbenchRunView{}, configstore.ErrWorkbenchQueue
	}
	s.mixed.mu.Lock()
	live := s.mixed.active[record.TaskID]
	s.mixed.mu.Unlock()
	if live != nil {
		if _, _, err := s.bindWorkbenchScope(ctx, scope, &live.prepared.target); err != nil {
			return WorkbenchRunView{}, err
		}
		live.mu.Lock()
		defer live.mu.Unlock()
		if live.prepared.owner != owner || live.queue == nil || live.queue.ID != record.ID || live.queue.Revision != record.Revision || live.view.Status == "cancelled" {
			return WorkbenchRunView{}, configstore.ErrWorkbenchQueue
		}
		view := live.view
		view.Items = append([]WorkbenchRunRow(nil), view.Items...)
		view.Errors = append([]InputError{}, view.Errors...)
		return view, nil
	}
	if record.Status == "running" {
		return WorkbenchRunView{}, configstore.ErrWorkbenchQueue
	}
	job, err := s.restoreMixedQueue(ctx, owner, scope, target, record)
	if err != nil {
		return WorkbenchRunView{}, err
	}
	if err := s.validateMixedTemplates(ctx, job.prepared); err != nil {
		return WorkbenchRunView{}, err
	}
	s.mixed.mu.Lock()
	active := s.mixed.active[record.TaskID] != nil || len(s.mixed.active) >= 10
	s.mixed.mu.Unlock()
	if active {
		return WorkbenchRunView{}, errors.New("原混合批次仍在当前进程，不能重复恢复")
	}
	id, err = randomID()
	if err != nil {
		return WorkbenchRunView{}, err
	}
	job.view.ID, job.view.TaskID = id, id
	job.view.Status, job.view.Message = "queued", "等待恢复混合批次未执行项目"
	job.view.OAuthBatchID, job.view.CurrentOAuthID = "", ""
	job.view.Available = len(job.items)
	job.queue.TaskID = id
	if err := s.persistMixedQueue(ctx, job, "running"); err != nil {
		return WorkbenchRunView{}, err
	}
	launched := false
	defer func() {
		if !launched {
			persist, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.persistMixedQueue(persist, job, "interrupted")
		}
	}()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: id, Skill: Skill, Operation: "account-workbench-mixed", Status: "queued", Message: job.view.Message, CreatedAt: now, UpdatedAt: now, Result: mixedTaskResult(job.view)}
	if err := s.tasks.Save(ctx, task); err != nil {
		return WorkbenchRunView{}, err
	}
	s.mixed.mu.Lock()
	s.mixed.active[id] = job
	s.mixed.mu.Unlock()
	job.mu.Lock()
	job.timer = time.AfterFunc(time.Until(job.expires), func() { _ = s.CancelWorkbenchRun(owner, id) })
	view := job.view
	view.Items = append([]WorkbenchRunRow(nil), view.Items...)
	view.Errors = append([]InputError{}, view.Errors...)
	job.mu.Unlock()
	if err := taskrunner.GoTask(s.runner, id, func(parent context.Context) { s.runWorkbenchInput(parent, job, task) }); err != nil {
		s.mixed.mu.Lock()
		delete(s.mixed.active, id)
		s.mixed.mu.Unlock()
		job.timer.Stop()
		close(job.done)
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return WorkbenchRunView{}, err
	}
	launched = true
	return view, nil
}

func (s *Service) restoreMixedQueue(ctx context.Context, owner string, scope ExportScope, target configstore.TargetSettings, record configstore.WorkbenchQueue) (*workbenchRun, error) {
	var payload mixedQueuePayload
	if json.Unmarshal(record.Payload, &payload) != nil || payload.Version != 1 || len(payload.View.Items) == 0 || len(payload.View.Items) > maxInputItems {
		return nil, configstore.ErrWorkbenchQueue
	}
	storedScope, err := normalizeWorkbenchScope(payload.View.Scope)
	optionScope, optionErr := normalizeWorkbenchScope(payload.Options.Scope)
	if err != nil || optionErr != nil || storedScope != scope || optionScope != scope || payload.View.ExportOnly != payload.Options.ExportOnly {
		return nil, configstore.ErrWorkbenchQueue
	}
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil || !expires.After(time.Now()) {
		return nil, configstore.ErrWorkbenchQueue
	}
	results, err := restoreQueueItems(payload.Results, len(payload.View.Items))
	if err != nil {
		return nil, err
	}
	entries, positions, err := restoreMixedEntries(&payload, results)
	if err != nil {
		return nil, err
	}
	prepared := &preparedWorkbenchRun{owner: owner, target: target, expires: expires, entries: entries, options: payload.Options, loadedTemplates: payload.Templates, view: WorkbenchRunPreview{RecoveryEnabled: true}}
	if payload.OAuth != nil {
		if len(positions) != len(payload.OAuth.View.Items) {
			return nil, configstore.ErrWorkbenchQueue
		}
		payload.OAuth.View.ExpiresAt = record.ExpiresAt
		if _, err := s.validateOAuthQueuePayload(ctx, owner, scope, target, payload.OAuth); err != nil {
			return nil, err
		}
		for i, row := range payload.OAuth.View.Items {
			parent := &payload.View.Items[positions[i]]
			if parent.Email != row.Email || parent.WorkspaceID != row.WorkspaceID {
				return nil, configstore.ErrWorkbenchQueue
			}
			parent.Status, parent.Message = row.Status, row.Message
		}
		prepared.oauth = &preparedOAuthBatch{scope: scope, owner: owner, target: target, expires: expires, inputs: payload.OAuth.Inputs, view: OAuthBatchPreview{Scope: scope, RecoveryEnabled: true, Items: payload.OAuth.View.Items}, resumePayload: payload.OAuth}
	}
	view := payload.View
	view.RecoveryEnabled, view.RecoveryID, view.ExpiresAt = true, record.ID, record.ExpiresAt
	return &workbenchRun{prepared: prepared, view: view, items: results, expires: expires, queue: &record, oauthQueue: payload.OAuth, done: make(chan struct{})}, nil
}
