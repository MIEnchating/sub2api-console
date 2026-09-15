package accountworkbench

import (
	"context"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func (s *Service) startSecurityBatchChild(ctx context.Context, job *securityBatch, input SecurityStartInput, item securityBatchItem) (SecurityView, error) {
	if item.source == nil {
		return s.startSecurity(ctx, job.owner, input, job.view.ID, &item)
	}
	origin, err := s.prepareSecurityReference(ctx, job.owner, job.view.Scope, *item.source)
	if err != nil {
		return SecurityView{}, err
	}
	if err := s.validateSecuritySources(ctx, job.owner, job.view.Scope, []securityBatchItem{item}); err != nil {
		return SecurityView{}, err
	}
	if origin.identity.UserID != "" {
		job.mu.Lock()
		duplicate := job.users[origin.identity.UserID]
		job.users[origin.identity.UserID] = true
		job.mu.Unlock()
		if duplicate {
			return SecurityView{}, browserlogin.ErrSecurityIdentity
		}
	}
	input.SourceOAuthID = item.source.OAuthID
	s.securityMu.Lock()
	storage := s.securityStorage
	s.securityMu.Unlock()
	if storage == nil || !storage.available() {
		return SecurityView{}, ErrSecurityStorage
	}
	return s.launchSecurity(ctx, job.owner, input, origin, storage, job.view.ID, nil)
}

func (s *Service) reserveSecurityBatchIdentity(value *securitySession, identity browserlogin.SecurityIdentity) error {
	if value.securityBatchID == "" {
		return nil
	}
	s.securityBatches.mu.Lock()
	job := s.securityBatches.active[value.securityBatchID]
	s.securityBatches.mu.Unlock()
	if job == nil || job.owner != value.owner {
		return browserlogin.ErrSession
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.users[identity.UserID] {
		return browserlogin.ErrSecurityIdentity
	}
	job.users[identity.UserID] = true
	return nil
}
