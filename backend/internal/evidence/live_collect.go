package evidence

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// CollectLive bypasses the inspection interval only for an actively watched account.
// It preserves policy scope and uses the same validation/deduplication as inspection.
func (s *Service) CollectLive(ctx context.Context, policy map[string]any, targets targetguard.Store, accountID string) error {
	configured, err := parsePolicy(policy)
	if err != nil {
		return err
	}
	if configured.source != "traffic" {
		return nil
	}
	memberships, err := s.repository.EvidenceTargets(ctx, &accountID, nil)
	if err != nil {
		return err
	}
	memberships = filterEvidenceTargets(memberships, configured)
	if len(memberships) == 0 {
		return nil
	}
	target, err := targetguard.Settings(ctx, targets)
	if err != nil {
		return err
	}
	client, err := adminclient.New(adminclient.Config{BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: min(time.Duration(target.TimeoutSeconds)*time.Second, 10*time.Second), Attempts: 1}, nil)
	if err != nil {
		return err
	}
	rows, readErr := client.RequestDetails(ctx, accountID, configured.lookbackMinutes, 100)
	var latencyErr *adminclient.LatencyEnrichmentError
	if readErr != nil && !errors.As(readErr, &latencyErr) {
		return readErr
	}
	// A changed target must never write evidence into the new target's account IDs.
	current, err := targetguard.Settings(ctx, targets)
	if err != nil {
		return err
	}
	if current.BaseURL != target.BaseURL || current.AdminKey != target.AdminKey {
		return targetguard.ErrChanged
	}
	now := time.Now().UTC()
	samples, _ := convertTrafficRows(accountID, memberships, rows, now.Add(-time.Duration(configured.lookbackMinutes)*time.Minute), now, 100)
	if len(samples) > 0 {
		if _, err := s.repository.PersistTrafficSamples(ctx, samples); err != nil {
			return err
		}
	}
	return readErr
}
