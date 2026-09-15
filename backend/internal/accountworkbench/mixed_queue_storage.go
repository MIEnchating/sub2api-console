package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type mixedQueuePayload struct {
	Version   int                `json:"version"`
	View      WorkbenchRunView   `json:"view"`
	Options   PreviewInput       `json:"options"`
	Templates map[string]int64   `json:"templates"`
	Inputs    []privateQueueItem `json:"inputs"`
	Results   []privateQueueItem `json:"results"`
	OAuth     *oauthQueuePayload `json:"oauth,omitempty"`
}

func (s *Service) prepareMixedQueue(ctx context.Context, job *workbenchRun) error {
	if !job.prepared.view.RecoveryEnabled {
		return nil
	}
	job.view.RecoveryEnabled, job.view.RecoveryID = true, job.view.ID
	job.queue = &configstore.WorkbenchQueue{ID: job.view.ID, Owner: exportHash(job.prepared.owner), Target: workbenchScopeFingerprint(job.prepared.options.Scope, job.prepared.target), Kind: "mixed", TaskID: job.view.ID, Status: "running", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: job.expires.UTC().Format(time.RFC3339Nano)}
	if prepared := job.prepared.oauth; prepared != nil {
		job.oauthQueue = &oauthQueuePayload{Version: 1, Inputs: prepared.inputs, View: OAuthBatchView{Scope: prepared.scope, ExpiresAt: job.view.ExpiresAt, Items: prepared.view.Items}}
	}
	return s.persistMixedQueue(ctx, job, "running")
}

func (s *Service) persistMixedQueue(ctx context.Context, job *workbenchRun, status string) error {
	job.mu.Lock()
	defer job.mu.Unlock()
	return s.persistMixedQueueLocked(ctx, job, status)
}

// Child callbacks take child.mu before the parent lock.
func (s *Service) persistMixedQueueLocked(ctx context.Context, job *workbenchRun, status string) error {
	if job.queue == nil || job.queueFrozen {
		return nil
	}
	store, err := s.queueStore()
	if err != nil {
		return err
	}
	inputs := []InputItem{}
	for _, entry := range job.prepared.entries {
		if entry.item != nil {
			item := *entry.item
			item.Index = entry.index
			inputs = append(inputs, item)
		}
	}
	payload, err := json.Marshal(mixedQueuePayload{Version: 1, View: job.view, Options: job.prepared.options, Templates: job.prepared.loadedTemplates, Inputs: queueItems(inputs), Results: queueItems(job.items), OAuth: job.oauthQueue})
	if err != nil {
		return errors.New("混合批次恢复资料无法编码")
	}
	record := *job.queue
	record.Status, record.Payload = status, payload
	saved, err := saveQueueRecord(ctx, store, record)
	if err != nil {
		return errors.New("混合批次恢复资料未可靠保存，已停止后续操作")
	}
	job.queue = &saved
	return nil
}

func (s *Service) persistMixedOAuth(ctx context.Context, job *workbenchRun, child *oauthBatch, status string) error {
	raw, err := json.Marshal(oauthQueuePayload{Version: 1, View: child.view, Inputs: child.inputs, Results: queueItems(child.results), Checkpoints: child.checkpoints})
	if err != nil {
		return errors.New("混合授权恢复资料无法编码")
	}
	var snapshot oauthQueuePayload
	if json.Unmarshal(raw, &snapshot) != nil {
		return errors.New("混合授权恢复资料无法编码")
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.queueFrozen {
		return nil
	}
	job.oauthQueue = &snapshot
	job.view.OAuthBatchID = child.view.ID
	queueStatus := "running"
	if status == "interrupted" {
		queueStatus = status
	}
	return s.persistMixedQueueLocked(ctx, job, queueStatus)
}

func (s *Service) deleteMixedQueue(job *workbenchRun) error {
	if job.queue == nil {
		return nil
	}
	store, err := s.queueStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	record := job.queue
	if err := store.DeleteWorkbenchQueue(ctx, record.Owner, record.Target, record.ID, record.Revision); err != nil {
		return err
	}
	job.queueFrozen = true
	job.queue = nil
	return nil
}

func (s *Service) stopMixedOAuth(job *workbenchRun, child *oauthBatch) {
	job.mu.Lock()
	preserve := job.queue != nil && !job.queueFrozen && job.view.Status != "cancelled" && time.Now().Before(job.expires)
	job.mu.Unlock()
	if preserve {
		s.stopOAuthBatchForRecovery(child)
		return
	}
	_ = s.CancelOAuthBatch(job.prepared.owner, child.view.ID)
}
