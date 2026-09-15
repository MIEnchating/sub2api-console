package accountworkbench

import (
	"context"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type ReauthorizationCandidate struct {
	AccountID            string `json:"account_id"`
	ProfileID            string `json:"profile_id"`
	ProfileRevision      int64  `json:"profile_revision"`
	Email                string `json:"email"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

// Maintenance can annotate accounts needing fresh authentication using this
// read-only boundary. Starting a browser and purchasing SMS still requires an
// owner-bound preview and explicit batch confirmation.
func (s *Service) ReauthorizationCandidates(ctx context.Context, accountIDs []string) ([]ReauthorizationCandidate, error) {
	ctx, err := targetguard.Pin(ctx, s.private)
	if err != nil {
		return nil, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return nil, err
	}
	store, err := s.profileStore()
	if err != nil {
		return nil, err
	}
	profiles, err := store.WorkbenchLoginProfiles(ctx, executionTargetFingerprint(target))
	if err != nil {
		return nil, err
	}
	selected := make(map[string]bool, len(accountIDs))
	for _, id := range accountIDs {
		selected[id] = true
	}
	client, err := s.clientFor(target)
	if err != nil {
		return nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, publicError(err)
	}
	byID := make(map[string]securityBatchItem, len(accounts))
	for _, account := range accounts {
		if item, err := securityBatchSnapshot(account, target); err == nil {
			byID[item.accountID] = item
		}
	}
	values := []ReauthorizationCandidate{}
	for _, profile := range profiles {
		if !selected[profile.AccountID] {
			continue
		}
		current, ok := byID[profile.AccountID]
		if !ok || current.workspaceID != profile.WorkspaceID || !sameSecurityIdentity(current.identity, profileIdentity(profile)) {
			continue
		}
		values = append(values, ReauthorizationCandidate{AccountID: profile.AccountID, ProfileID: profile.ID, ProfileRevision: profile.Revision, Email: profile.Email, RequiresConfirmation: true})
	}
	return values, nil
}
