package onboarding

import (
	"context"
	"fmt"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func (s *Service) creationGlobalLimit(ctx context.Context, validated validatedRequest) (*int64, error) {
	document, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return routing.NewAccountGlobalCapacityLimit(document, validated.candidate.UpstreamID, validated.locals[0].ID, validated.request.AllocationOverride, strings.EqualFold(validated.auth.UpstreamType, "sub2api"))
}

func (s *Service) creationGlobalSnapshot(ctx context.Context, limit int64) (creationCapacitySnapshot, error) {
	inventory, err := s.repository.RoutingCapacityAccounts(ctx)
	snapshot := creationCapacitySnapshot{OnboardingCapacity: business.OnboardingCapacity{Limit: &limit, Accounts: inventory}, released: map[string]bool{}}
	if err != nil {
		return snapshot, err
	}
	document, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return snapshot, err
	}
	accounts, err := s.repository.RoutingAccounts(ctx, nil, nil)
	if err != nil {
		return snapshot, err
	}
	scope, err := routing.UpstreamCapacityScope(document, accounts)
	if err != nil {
		return snapshot, err
	}
	stops, err := routing.UpstreamManagedStopScope(document, accounts)
	if err != nil {
		return snapshot, err
	}
	for _, account := range inventory {
		snapshot.released[account.ID] = scope[account.ID] && (account.EffectiveState == business.AccountStateConcurrencyLimited || stops[account.ID])
	}
	return snapshot, nil
}

func (s *Service) applyCreationGlobalPreview(ctx context.Context, requests []Request, result []ConcurrencyAllocation) ([]ConcurrencyAllocation, error) {
	var limit *int64
	constrained := map[int]bool{}
	for i, request := range requests {
		if len(request.AccountIDs) > 0 {
			continue
		}
		validated, err := s.validate(ctx, request)
		if err != nil {
			return nil, err
		}
		value, err := s.creationGlobalLimit(ctx, validated)
		if err != nil {
			return nil, err
		}
		if value != nil {
			constrained[i] = true
			if limit == nil || *value < *limit {
				limit = value
			}
		}
	}
	if limit == nil {
		return result, nil
	}
	snapshot, err := s.creationGlobalSnapshot(ctx, *limit)
	if err != nil {
		return nil, err
	}
	remaining, err := creationRemaining(snapshot, "")
	if err != nil {
		return nil, err
	}
	for i, item := range result {
		if !constrained[i] && item.Concurrency != nil && !item.WaitingForCapacity {
			remaining = max(int64(0), remaining-*item.Concurrency)
		}
	}
	automatic := []int{}
	for i, item := range result {
		if item.Concurrency == nil || item.WaitingForCapacity || !constrained[i] {
			continue
		}
		if requests[i].Concurrency == nil {
			automatic = append(automatic, i)
			continue
		}
		if *item.Concurrency > remaining {
			return nil, fmt.Errorf("新增账号手动并发超过全局剩余额度 %d（全局上限 %d），请降低并发或调整已有账号", remaining, *limit)
		}
		remaining -= *item.Concurrency
	}
	if remaining < int64(len(automatic)) {
		for _, i := range automatic {
			if requests[i].Schedulable || requests[i].UpstreamType != "sub2api" {
				return nil, fmt.Errorf("全局剩余并发 %d 不足以启用 %d 个新账号，请调整全局上限或已有账号", remaining, len(automatic))
			}
			value := int64(1)
			result[i] = ConcurrencyAllocation{Concurrency: &value, WaitingForCapacity: true}
		}
		return result, nil
	}
	allocated := map[int]int64{}
	for _, i := range automatic {
		allocated[i] = 1
		remaining--
	}
	for remaining > 0 {
		active := []int{}
		for _, i := range automatic {
			if allocated[i] < *result[i].Concurrency {
				active = append(active, i)
			}
		}
		if len(active) == 0 {
			break
		}
		share := max(int64(1), remaining/int64(len(active)))
		for _, i := range active {
			increase := min(share, *result[i].Concurrency-allocated[i], remaining)
			allocated[i] += increase
			remaining -= increase
		}
	}
	for _, i := range automatic {
		value := allocated[i]
		result[i].Concurrency = &value
	}
	return result, nil
}

func (s *Service) checkCreationConcurrency(ctx context.Context, validated *validatedRequest, concurrency int64, pending *business.PendingOnboarding) (int64, error) {
	value, err := s.checkCreationUpstreamConcurrency(ctx, validated, concurrency, pending)
	if err != nil {
		return 0, err
	}
	limit, err := s.creationGlobalLimit(ctx, *validated)
	if err != nil {
		return 0, err
	}
	if limit == nil || validated.request.WaitingForCapacity {
		return value, nil
	}
	snapshot, err := s.creationGlobalSnapshot(ctx, *limit)
	if err != nil {
		return 0, err
	}
	exclude := ""
	if pending != nil {
		exclude = pending.UpstreamAccountID
	}
	if err := s.refreshCreationCapacity(ctx, &snapshot, exclude); err != nil {
		return 0, err
	}
	remaining, err := creationRemaining(snapshot, exclude)
	if err != nil {
		return 0, err
	}
	if validated.request.Concurrency == nil {
		if remaining < 1 && !validated.request.Schedulable && strings.EqualFold(validated.auth.UpstreamType, "sub2api") {
			validated.request.WaitingForCapacity = true
			return 1, nil
		}
		value = min(value, remaining)
	}
	if value < 1 || value > remaining {
		return 0, fmt.Errorf("新增账号并发 %d 超过全局剩余额度 %d（全局上限 %d），请降低并发或调整已有账号", value, remaining, *limit)
	}
	return value, nil
}
