package routingwrite

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func (s *Service) acquireManualCostWallSnapshot(
	ctx context.Context,
	repository manualCostRepository,
	accounts []business.RoutingAccount,
	selected map[string]bool,
) (context.Context, []business.RoutingAccount, func() error, error) {
	for attempt := 0; attempt < 3; attempt++ {
		lockedIDs := manualCostWallAccountIDs(accounts, selected)
		if len(lockedIDs) == 0 {
			return ctx, accounts, nil, nil
		}
		resources := []string{mutationguard.AccountCatalog()}
		for id := range lockedIDs {
			resources = append(resources, mutationguard.Account(id))
		}
		var guarded context.Context
		var release func() error
		var err error
		if s.admin == nil {
			ctx, err = targetguard.Capture(ctx, s.targets)
			if err != nil {
				return nil, nil, nil, err
			}
			guarded, release, err = targetguard.Acquire(ctx, s.repository, resources...)
		} else {
			guarded, release, err = mutationguard.Acquire(ctx, s.repository, resources...)
		}
		if err != nil {
			return nil, nil, nil, err
		}
		latest, err := repository.RoutingAccounts(guarded, nil, nil)
		if err != nil {
			return nil, nil, nil, errors.Join(err, release())
		}
		covered := true
		for id := range manualCostWallAccountIDs(latest, selected) {
			if !lockedIDs[id] {
				covered = false
				break
			}
		}
		// Recalculate from the locked snapshot. New manual accounts require a
		// fresh atomic lease set before any remote write can begin.
		if !covered {
			if err := release(); err != nil {
				return nil, nil, nil, err
			}
			accounts = latest
			continue
		}
		if s.admin == nil {
			guarded, err = targetguard.Bind(guarded, s.targets)
			if err != nil {
				return nil, nil, nil, errors.Join(err, release())
			}
		}
		return guarded, latest, release, nil
	}
	return nil, nil, nil, errors.New("成本墙检查范围持续变化，自动重算未完成，请稍后重试")
}

func manualCostWallAccountIDs(accounts []business.RoutingAccount, selected map[string]bool) map[string]bool {
	ids := map[string]bool{}
	for _, account := range accounts {
		if account.ManualPriority != nil && (len(selected) == 0 || selected[account.ID]) {
			ids[account.ID] = true
		}
	}
	return ids
}
