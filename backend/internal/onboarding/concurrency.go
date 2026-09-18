package onboarding

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type ConcurrencyAllocation struct {
	Concurrency        *int64 `json:"concurrency"`
	WaitingForCapacity bool   `json:"waiting_for_capacity"`
}

// PreviewConcurrency reserves explicit values before splitting remaining capacity.
// When the automatic batch cannot fit, stopped accounts can wait without a reservation.
func (s *Service) PreviewConcurrency(ctx context.Context, requests []Request) ([]ConcurrencyAllocation, error) {
	requests = expandBatchRequests(requests)
	if len(requests) == 0 || len(requests) > 50 {
		return nil, errors.New("单次并发分配必须包含 1 到 50 项")
	}
	result := make([]ConcurrencyAllocation, len(requests))
	pools := map[string][]int{}
	snapshots := map[string]business.OnboardingCapacity{}
	for i, request := range requests {
		validated, err := s.validate(ctx, request)
		if err != nil {
			return nil, err
		}
		if len(request.AccountIDs) > 0 {
			continue
		}
		if !strings.EqualFold(validated.auth.UpstreamType, "sub2api") {
			if request.WaitingForCapacity {
				return nil, errors.New("等待并发额度仅适用于 Sub2API 上游")
			}
			value, err := s.defaultCreationConcurrency(ctx, validated)
			if err != nil {
				return nil, err
			}
			result[i].Concurrency = &value
			continue
		}
		id := validated.candidate.UpstreamID
		if _, exists := snapshots[id]; !exists {
			snapshot, err := s.creationCapacity(ctx, id)
			if err != nil {
				return nil, err
			}
			snapshots[id] = snapshot
		}
		if request.WaitingForCapacity {
			value := int64(1)
			result[i] = ConcurrencyAllocation{Concurrency: &value, WaitingForCapacity: true}
			continue
		}
		pools[id] = append(pools[id], i)
	}
	for id, indices := range pools {
		snapshot := snapshots[id]
		if *snapshot.Limit == 0 {
			for _, i := range indices {
				validated, err := s.validate(ctx, requests[i])
				if err != nil {
					return nil, err
				}
				value, err := s.defaultCreationConcurrency(ctx, validated)
				if err != nil {
					return nil, err
				}
				result[i].Concurrency = &value
			}
			continue
		}
		remaining, err := creationRemaining(snapshot, "")
		if err != nil {
			return nil, err
		}
		automatic := []int{}
		for _, i := range indices {
			if requests[i].Concurrency == nil {
				automatic = append(automatic, i)
				continue
			}
			value := *requests[i].Concurrency
			if value > remaining {
				return nil, fmt.Errorf("手动并发合计超过上游剩余额度 %d（用户上限 %d）；请降低并发或先调整已有账号", remaining, *snapshot.Limit)
			}
			remaining -= value
			result[i].Concurrency = &value
		}
		if remaining < int64(len(automatic)) {
			for _, i := range automatic {
				if requests[i].Schedulable {
					return nil, fmt.Errorf("上游剩余并发 %d 不足以启用 %d 个新账号；请先保持新账号停用或调整已有账号", remaining, len(automatic))
				}
				value := int64(1)
				result[i] = ConcurrencyAllocation{Concurrency: &value, WaitingForCapacity: true}
			}
			continue
		}
		for n, i := range automatic {
			value := min(int64(10_000_000), remaining/int64(len(automatic)))
			if int64(n) < remaining%int64(len(automatic)) {
				value = min(int64(10_000_000), value+1)
			}
			result[i].Concurrency = &value
		}
	}
	return result, nil
}

func (s *Service) defaultCreationConcurrency(ctx context.Context, validated validatedRequest) (int64, error) {
	if validated.request.Concurrency != nil {
		return *validated.request.Concurrency, nil
	}
	defaults, err := s.private.AccountDefaults(ctx)
	if err != nil {
		return 0, err
	}
	if provider, ok := s.private.(accountCreationSettingsStore); ok {
		settings, err := provider.AccountCreationSettings(ctx)
		if err != nil {
			return 0, err
		}
		return configstore.ResolveAccountCreationPolicy(settings, validated.locals[0].ID).Concurrency, nil
	}
	return defaults.Concurrency, nil
}

func (s *Service) creationCapacity(ctx context.Context, id string) (business.OnboardingCapacity, error) {
	snapshot, err := s.repository.OnboardingCapacity(ctx, id)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Limit == nil || *snapshot.Limit < 0 || (snapshot.Status != business.UpstreamConcurrencyKnown && snapshot.Status != business.UpstreamConcurrencyUnlimited) {
		return snapshot, errors.New("上游并发上限尚未确认，请先同步上游用户信息后再添加账号")
	}
	return snapshot, nil
}

func creationRemaining(snapshot business.OnboardingCapacity, exclude string) (int64, error) {
	remaining := *snapshot.Limit
	for _, account := range snapshot.Accounts {
		if account.ID == exclude {
			continue
		}
		if account.Schedulable == nil || account.Concurrency == nil || *account.Concurrency <= 0 {
			return 0, fmt.Errorf("账号 #%s 的并发或调度状态尚未确认，请同步账号后再分配", account.ID)
		}
		// Paused accounts retain their reservation, including newly created ones.
		// Only a confirmed capacity pause has deliberately released its slots.
		if !*account.Schedulable && account.EffectiveState == business.AccountStateConcurrencyLimited {
			continue
		}
		remaining = max(int64(0), remaining-*account.Concurrency)
	}
	return remaining, nil
}

func (s *Service) checkCreationConcurrency(ctx context.Context, validated *validatedRequest, concurrency int64, pending *business.PendingOnboarding) (int64, error) {
	if !strings.EqualFold(validated.auth.UpstreamType, "sub2api") {
		if validated.request.WaitingForCapacity {
			return 0, errors.New("等待并发额度仅适用于 Sub2API 上游")
		}
		return concurrency, nil
	}
	snapshot, err := s.creationCapacity(ctx, validated.candidate.UpstreamID)
	if err != nil {
		return 0, err
	}
	// Waiting accounts never reserve capacity or become active during creation.
	// Their remote stopped state must be confirmed before the projection commits.
	if validated.request.WaitingForCapacity {
		return 1, nil
	}
	if *snapshot.Limit == 0 {
		return concurrency, nil
	}
	exclude := ""
	if pending != nil {
		exclude = pending.UpstreamAccountID
	}
	// Re-read existing accounts under the global capacity lease. A manual
	// upstream edit must not let a stale local snapshot finance a new account.
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return 0, err
	}
	client, err := adminclient.New(adminclient.Config{BaseURL: target.BaseURL, AdminKey: target.AdminKey, Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 1}, nil)
	if err != nil {
		return 0, err
	}
	for i := range snapshot.Accounts {
		account := &snapshot.Accounts[i]
		if account.ID == exclude {
			continue
		}
		remote, err := client.Account(ctx, account.ID)
		if err != nil {
			return 0, fmt.Errorf("账号 #%s 并发复核失败，请同步账号后重试：%w", account.ID, err)
		}
		if textValue(remote["id"]) != account.ID {
			return 0, errors.New("账号并发复核返回了不匹配的稳定 ID")
		}
		concurrencyValue, ok := remote["concurrency"]
		if !ok {
			return 0, errors.New("远端账号未返回并发，请同步账号后重试")
		}
		value, parseErr := strconv.ParseInt(textValue(concurrencyValue), 10, 64)
		if parseErr != nil {
			return 0, errors.New("远端账号并发无效，请同步账号后重试")
		}
		schedulable, ok := remote["schedulable"].(bool)
		if !ok {
			return 0, errors.New("远端账号未返回调度状态，请同步账号后重试")
		}
		account.Concurrency, account.Schedulable = &value, &schedulable
	}
	remaining, err := creationRemaining(snapshot, exclude)
	if err != nil {
		return 0, err
	}
	if validated.request.Concurrency == nil {
		if remaining < 1 && !validated.request.Schedulable {
			validated.request.WaitingForCapacity = true
			return 1, nil
		}
		concurrency = min(remaining, int64(10_000_000))
	}
	if concurrency < 1 || concurrency > remaining {
		return 0, fmt.Errorf("新增账号并发 %d 超过上游剩余额度 %d（用户上限 %d）；请降低并发或先调整已有账号", concurrency, remaining, *snapshot.Limit)
	}
	return concurrency, nil
}
