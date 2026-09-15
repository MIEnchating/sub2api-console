package accountworkbench

// Stop a runner interrupted by its parent while preserving its private receipt
// and any safe browser checkpoint. Explicit user cancellation uses CancelOAuthBatch.
func (s *Service) stopOAuthBatchForRecovery(job *oauthBatch) {
	job.mu.Lock()
	job.stopRequested = true
	if job.cancel != nil {
		job.cancel()
	}
	current := job.view.CurrentOAuthID
	job.mu.Unlock()
	if current != "" {
		s.removeOAuth(job.owner, current)
	}
	<-job.done
	job.mu.Lock()
	job.queueFrozen = true
	if job.timer != nil {
		job.timer.Stop()
	}
	id := job.view.ID
	job.mu.Unlock()
	s.batches.mu.Lock()
	if s.batches.active[id] == job {
		delete(s.batches.active, id)
	}
	s.batches.mu.Unlock()
}

func stopOAuthChild(child *oauthSession) {
	child.mu.Lock()
	if child.cancel == nil {
		child.view.Status = "cancelled"
	}
	if child.cancel != nil {
		child.cancel()
	}
	child.mu.Unlock()
	<-child.done
}
