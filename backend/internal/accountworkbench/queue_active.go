package accountworkbench

import (
	"context"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) queueRecoveryActive(record configstore.WorkbenchQueue) bool {
	if record.Kind == "oauth-batch" {
		s.batches.mu.Lock()
		job := s.batches.active[record.TaskID]
		s.batches.mu.Unlock()
		if job == nil {
			return false
		}
		job.mu.Lock()
		defer job.mu.Unlock()
		return job.queue != nil && job.queue.ID == record.ID && job.view.Status != "cancelled" && time.Now().Before(job.expires)
	}
	s.mixed.mu.Lock()
	job := s.mixed.active[record.TaskID]
	s.mixed.mu.Unlock()
	if job == nil {
		return false
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	return job.queue != nil && job.queue.ID == record.ID && job.view.Status != "cancelled" && time.Now().Before(job.expires)
}

func (s *Service) reconnectOAuthQueue(ctx context.Context, owner string, scope ExportScope, target configstore.TargetSettings, record configstore.WorkbenchQueue) (OAuthBatchView, bool, error) {
	s.batches.mu.Lock()
	job := s.batches.active[record.TaskID]
	s.batches.mu.Unlock()
	if job == nil {
		return OAuthBatchView{}, false, nil
	}
	if _, _, err := s.bindWorkbenchScope(ctx, scope, &job.target); err != nil {
		return OAuthBatchView{}, true, err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.owner != owner || job.queue == nil || job.queue.ID != record.ID || job.queue.Revision != record.Revision || job.view.Status == "cancelled" || !time.Now().Before(job.expires) || workbenchScopeFingerprint(job.scope, target) != record.Target {
		return OAuthBatchView{}, true, configstore.ErrWorkbenchQueue
	}
	view := job.view
	view.Items = append([]OAuthBatchRow(nil), view.Items...)
	return view, true, nil
}
