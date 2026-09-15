package accountworkbench

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) validateOAuthQueueCheckpoint(ctx context.Context, owner string, scope ExportScope, target configstore.TargetSettings, row OAuthBatchRow, ref *oauthQueueCheckpoint) (bool, error) {
	store, err := s.checkpointStore()
	if err != nil {
		return false, err
	}
	record, err := store.WorkbenchOAuthCheckpoint(ctx, exportHash(owner), workbenchScopeFingerprint(scope, target), ref.ID)
	if errors.Is(err, configstore.ErrWorkbenchOAuthCheckpoint) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !record.Automatic || record.Status != "watching" && record.Status != "ready" {
		return false, nil
	}
	if record.SourceTaskID != ref.SourceTaskID || record.ParentID != ref.ParentID || ref.ParentID == "" || record.Scope != string(scope) {
		return false, configstore.ErrWorkbenchQueue
	}
	payload, err := validateCheckpointPayload(record)
	if err != nil || !strings.EqualFold(payload.ExpectedEmail, row.Email) || payload.ExpectedWorkspace != row.WorkspaceID {
		return false, configstore.ErrWorkbenchQueue
	}
	view, err := s.readCheckpointView(ctx, record)
	if err != nil {
		return false, err
	}
	ref.Revision, ref.WorkerRevision = record.Revision, view.CheckpointRevision
	return view.CanRestore, nil
}

func (s *Service) oauthQueueCallbacks(job *oauthBatch, index int) oauthCallbacks {
	if job.queue == nil && job.parentQueuePersist == nil {
		return oauthCallbacks{}
	}
	return oauthCallbacks{
		beforeLaunch: func(ctx context.Context, view OAuthView) error {
			job.mu.Lock()
			defer job.mu.Unlock()
			if job.queueFrozen || job.view.Status == "cancelled" {
				return context.Canceled
			}
			if job.checkpoints == nil {
				job.checkpoints = map[int]oauthQueueCheckpoint{}
			}
			if view.CheckpointID != "" {
				job.checkpoints[index] = oauthQueueCheckpoint{ID: view.CheckpointID, SourceTaskID: view.ID, ParentID: job.view.ID}
			}
			job.view.CurrentOAuthID = view.ID
			return s.persistOAuthQueue(ctx, job, "running")
		},
		onAuthorized: func(ctx context.Context, credentials map[string]any) error {
			job.mu.Lock()
			defer job.mu.Unlock()
			if job.queueFrozen || job.view.Status == "cancelled" {
				return context.Canceled
			}
			if job.view.Items[index].Status != "succeeded" {
				job.results = append(job.results, InputItem{Index: index, Email: job.view.Items[index].Email, Credentials: cloneInputMap(credentials)})
				job.view.Items[index].Status, job.view.Items[index].Message = "succeeded", "授权成功，等待确认"
				job.view.Available = len(job.results)
				delete(job.checkpoints, index)
			}
			return s.persistOAuthQueue(ctx, job, "running")
		},
	}
}

func (s *Service) startOAuthQueueItem(ctx context.Context, job *oauthBatch, taskID string, index int, login *OAuthLoginInput, profile *profileAuthorization) (OAuthView, error) {
	job.mu.Lock()
	checkpoint := job.checkpoints[index]
	job.mu.Unlock()
	callbacks := s.oauthQueueCallbacks(job, index)
	callbacks = s.sourceProfileCallbacks(job, index, callbacks)
	if profile != nil && profile.maintenanceRevision != 0 {
		childID := ""
		email := login.Email
		callbacks = oauthCallbacks{
			beforeLaunch: func(_ context.Context, view OAuthView) error { childID = view.ID; return nil },
			onAuthorized: func(persist context.Context, credentials map[string]any) error {
				return s.persistMaintenanceUploadResult(persist, taskID, childID, job.target, profile, InputItem{Index: index, Email: email, Credentials: credentials})
			},
		}
	}
	if checkpoint.ID != "" {
		return s.restoreOAuthCheckpoint(ctx, job.owner, checkpoint.ID, OAuthCheckpointAction{Scope: job.scope, Revision: checkpoint.Revision, CheckpointRevision: checkpoint.WorkerRevision, Confirmed: true}, checkpoint.ParentID, taskID, callbacks)
	}
	_, supportsRecovery := s.oauthFactory.(browserlogin.OAuthCheckpointReader)
	return s.startOAuthWithInput(ctx, job.owner, OAuthStartInput{Scope: job.scope, RecoveryEnabled: job.view.RecoveryEnabled && supportsRecovery, Login: login, callbacks: callbacks}, taskID, profile)
}
