package accountworkbench

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) RestoreOAuthCheckpoint(ctx context.Context, owner, id string, input OAuthCheckpointAction) (OAuthView, error) {
	return s.restoreOAuthCheckpoint(ctx, owner, id, input, "", "")
}

func (s *Service) restoreOAuthCheckpoint(ctx context.Context, owner, id string, input OAuthCheckpointAction, originalParentID, activeBatchID string, callbacks ...oauthCallbacks) (OAuthView, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	if owner == "" || input.Revision < 1 || !input.Confirmed {
		return OAuthView{}, errors.New("请确认检查点版本并明确恢复授权")
	}
	if _, supported := s.oauthFactory.(browserlogin.OAuthRecoveryFactory); !supported {
		return OAuthView{}, errors.New("授权浏览器尚未支持检查点恢复，请更新 browser 服务")
	}
	store, err := s.checkpointStore()
	if err != nil {
		return OAuthView{}, err
	}
	input.Scope, err = normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return OAuthView{}, err
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, input.Scope, nil)
	if err != nil {
		return OAuthView{}, err
	}
	record, err := store.WorkbenchOAuthCheckpoint(ctx, exportHash(owner), workbenchScopeFingerprint(input.Scope, target), id)
	if err != nil || record.Revision != input.Revision || record.Status != "ready" && !(record.Automatic && record.Status == "watching") || record.Scope != string(input.Scope) || record.ParentID != originalParentID || (originalParentID == "") != (activeBatchID == "") {
		return OAuthView{}, configstore.ErrWorkbenchOAuthCheckpoint
	}
	if record.Automatic {
		if active := s.activeCheckpointSession(record); active != nil {
			if err := s.validateOAuthScope(ctx, active); err != nil {
				return OAuthView{}, err
			}
			return s.ReadOAuth(ctx, owner, record.SourceTaskID)
		}
		current, err := s.readCheckpointView(ctx, record)
		if err != nil {
			return OAuthView{}, err
		}
		if !current.CanRestore || input.CheckpointRevision < 1 || input.CheckpointRevision != current.CheckpointRevision {
			return OAuthView{}, errors.New("自动登录检查点已变化或不可恢复，请刷新后重新确认")
		}
		record.WorkerRevision, record.Stage = current.CheckpointRevision, current.Stage
	}
	payload, err := validateCheckpointPayload(record)
	if err != nil {
		return OAuthView{}, err
	}
	lease, err := randomID()
	if err != nil {
		return OAuthView{}, err
	}
	taskID, err := randomID()
	if err != nil {
		return OAuthView{}, err
	}
	expires, _ := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	binding := *payload.Options.Recovery
	binding.Lease = lease
	payload.Options.Recovery = &binding
	restore := browserlogin.OAuthRestoreOptions{Checkpoint: browserlogin.OAuthCheckpointRef{ID: record.WorkerID, Owner: record.OwnerHash, Lease: record.WorkerLease}, Lease: lease, Options: payload.Options, Revision: record.WorkerRevision}
	view := OAuthView{Scope: input.Scope, View: browserlogin.View{ID: taskID, TaskID: taskID, Host: "auth.openai.com", Status: "starting", Message: "正在恢复授权检查点，等待人工接管", ExpiresAt: record.ExpiresAt, Width: browserlogin.Width, Height: browserlogin.Height}}
	value := &oauthSession{owner: owner, scope: input.Scope, batchID: activeBatchID, view: view, target: target, expires: expires, finish: make(chan struct{}, 1), done: make(chan struct{}), assistPaused: true, options: payload.Options, verifier: payload.Verifier, expectedEmail: payload.ExpectedEmail, expectedWorkspace: payload.ExpectedWorkspace, restore: &restore}
	value.smsOriginTaskID = payload.SMSOriginTaskID
	if value.smsOriginTaskID == "" {
		value.smsOriginTaskID = record.SourceTaskID
	}
	if len(callbacks) > 0 {
		value.callbacks = callbacks[0]
	}
	s.oauthMu.Lock()
	if s.oauthBusy || s.oauthBatchID != "" && s.oauthBatchID != activeBatchID || len(s.oauthSessions) >= 20 {
		s.oauthMu.Unlock()
		return OAuthView{}, errors.New("授权浏览器正在使用或关闭中，请稍后恢复")
	}
	s.oauthBusy = true
	s.oauthSessions[taskID] = value
	s.oauthMu.Unlock()
	launched := false
	defer func() {
		if !launched {
			_ = s.discardAutomaticCheckpoint(value)
			s.removeOAuth(owner, taskID)
			s.releaseOAuthBrowser()
		}
	}()
	if err := s.validateOAuthScope(ctx, value); err != nil {
		return OAuthView{}, err
	}
	record.Status, record.TaskID = "restoring", taskID
	saved, err := store.SaveWorkbenchOAuthCheckpoint(ctx, record)
	if err != nil {
		return OAuthView{}, err
	}
	value.restoreRecord = &saved
	if record.Automatic {
		if err := s.prepareAutomaticCheckpoint(ctx, value); err != nil {
			s.completeCheckpointRestore(context.WithoutCancel(ctx), value, false)
			return OAuthView{}, err
		}
		view = value.view
	}
	if value.callbacks.beforeLaunch != nil {
		if err := value.callbacks.beforeLaunch(ctx, view); err != nil {
			_ = s.completeCheckpointRestore(context.WithoutCancel(ctx), value, false)
			return OAuthView{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: taskID, Skill: Skill, Operation: "account-workbench-oauth-recovery", Status: "queued", Message: view.Message, CreatedAt: now, UpdatedAt: now, Result: map[string]any{"phase": "starting", "checkpoint_id": record.ID, "source_task_id": record.SourceTaskID}}
	if err := s.tasks.Save(ctx, task); err != nil {
		s.completeCheckpointRestore(context.WithoutCancel(ctx), value, false)
		return OAuthView{}, err
	}
	value.mu.Lock()
	value.timer = time.AfterFunc(time.Until(expires), func() { s.removeOAuth(owner, taskID) })
	value.mu.Unlock()
	if err := taskrunner.GoTask(s.runner, taskID, func(parent context.Context) { s.runOAuth(parent, value, payload.Options, payload.Verifier, task, nil) }); err != nil {
		s.completeCheckpointRestore(context.WithoutCancel(ctx), value, false)
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return OAuthView{}, err
	}
	launched = true
	return view, nil
}

func validateCheckpointPayload(record configstore.WorkbenchOAuthCheckpoint) (oauthCheckpointPayload, error) {
	var payload oauthCheckpointPayload
	if json.Unmarshal(record.Payload, &payload) != nil || string(payload.Scope) != record.Scope || payload.ParentID != record.ParentID || payload.Options.Validate() != nil || payload.Options.Recovery == nil || payload.Options.Recovery.Owner != record.OwnerHash || payload.Options.Recovery.Lease != record.WorkerLease || len(payload.Verifier) < 43 || len(payload.Verifier) > 128 {
		return payload, configstore.ErrWorkbenchOAuthCheckpoint
	}
	if payload.Options.Recovery.AutoCheckpoint != record.Automatic || record.Automatic && payload.Options.Recovery.CheckpointID != record.WorkerID {
		return payload, configstore.ErrWorkbenchOAuthCheckpoint
	}
	if payload.SMSOriginTaskID != "" && !validExportID(payload.SMSOriginTaskID) {
		return payload, configstore.ErrWorkbenchOAuthCheckpoint
	}
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil || !expires.Equal(payload.Options.Recovery.ExpiresAt) {
		return payload, configstore.ErrWorkbenchOAuthCheckpoint
	}
	authorization, err := url.Parse(payload.Options.AuthorizationURL)
	digest := sha256.Sum256([]byte(payload.Verifier))
	if err != nil || authorization.Query().Get("code_challenge") != base64.RawURLEncoding.EncodeToString(digest[:]) {
		return payload, configstore.ErrWorkbenchOAuthCheckpoint
	}
	return payload, nil
}

func (s *Service) openOAuthBrowser(ctx context.Context, value *oauthSession, options browserlogin.OAuthOptions) (browserlogin.OAuthBrowser, error) {
	if value.restore == nil {
		return s.oauthFactory.OpenOAuth(ctx, options)
	}
	factory, ok := s.oauthFactory.(browserlogin.OAuthRecoveryFactory)
	if !ok {
		return nil, browserlogin.ErrOAuthCheckpoint
	}
	browser, err := factory.RestoreOAuth(ctx, *value.restore)
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if saveErr := s.completeCheckpointRestore(persist, value, err == nil && browser != nil); saveErr != nil && err == nil {
		err = saveErr
	}
	return browser, err
}

func (s *Service) completeCheckpointRestore(ctx context.Context, value *oauthSession, succeeded bool) error {
	if value.restoreRecord == nil {
		return nil
	}
	store, err := s.checkpointStore()
	if err != nil {
		return err
	}
	record := *value.restoreRecord
	record.Status, record.Payload = "failed", nil
	if succeeded {
		record.Status = "restored"
	}
	_, err = store.SaveWorkbenchOAuthCheckpoint(ctx, record)
	if err == nil {
		value.restoreRecord = nil
	}
	return err
}
