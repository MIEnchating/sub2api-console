package evidence

import (
	"sort"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func AutomaticProbeBatchSize(policy map[string]any) (int, error) {
	configured, err := parsePolicy(policy)
	if err != nil {
		return 0, err
	}
	return 8 * configured.trafficConcurrency, nil
}

func (s *Service) selectProbeBatch(accountIDs []string, targets map[string][]business.EvidenceTarget, now time.Time, limit int) []string {
	s.probeBatchMu.Lock()
	defer s.probeBatchMu.Unlock()
	if s.probeSelectedAt == nil {
		s.probeSelectedAt = map[string]time.Time{}
	}
	for accountID := range s.probeSelectedAt {
		if _, present := targets[accountID]; !present {
			delete(s.probeSelectedAt, accountID)
		}
	}
	lastChecked := make(map[string]time.Time, len(accountIDs))
	for _, accountID := range accountIDs {
		primary, _ := primaryEvidenceMembership(targets[accountID])
		last := time.Time{}
		if primary.ProbeAt != nil && !primary.ProbeAt.After(now.Add(time.Minute)) {
			last = *primary.ProbeAt
		}
		selectedAt := s.probeSelectedAt[accountID]
		if !selectedAt.After(now.Add(time.Minute)) && selectedAt.After(last) {
			last = selectedAt
		}
		lastChecked[accountID] = last
	}
	// The incoming IDs are already in stable numeric order; equal evidence times
	// keep that order while missing and oldest observations move to the front.
	sort.SliceStable(accountIDs, func(left, right int) bool {
		return lastChecked[accountIDs[left]].Before(lastChecked[accountIDs[right]])
	})
	selected := accountIDs[:min(limit, len(accountIDs))]
	// Selection history rotates skipped or failed-to-start batches without
	// manufacturing a health sample for an account that was never probed.
	for _, accountID := range selected {
		s.probeSelectedAt[accountID] = now
	}
	return selected
}
