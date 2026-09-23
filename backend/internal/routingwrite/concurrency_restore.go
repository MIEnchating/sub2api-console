package routingwrite

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func hasConcurrencyRestoration(targets map[string]business.AccountRoutingTarget) bool {
	for _, target := range targets {
		if target.RestoreConcurrency {
			return true
		}
	}
	return false
}

// Restoring a reduction is independent of the disabled scaling write switch,
// but is authorized only by the latest scope and the confirmed owned baseline.
// The enclosing writer holds the account lease and revalidates the policy
// before every remote mutation, including retries.
func (s *Service) checkConcurrencyRestoration(ctx context.Context, target business.AccountRoutingTarget, current values, fingerprint string) (string, error) {
	if target.ReleaseControl || target.AbandonControl || target.CleanupAction != nil || target.UpstreamReductionID != "" || target.UpstreamAllocation {
		return "", errors.New("并发恢复不能合并容量分配、清理或交还控制权操作")
	}
	baseline, found, err := s.baseline(ctx, target.AccountID, fingerprint)
	if err != nil {
		return "", err
	}
	if !found || baseline.OwnershipVersion == 2 || baseline.Concurrency == nil || baseline.ManagedConcurrency == nil ||
		target.Concurrency == nil || current.concurrency == nil || *current.concurrency <= 0 ||
		*target.Concurrency != *baseline.Concurrency || *baseline.Concurrency <= *current.concurrency ||
		*current.concurrency != *baseline.ManagedConcurrency || current.schedulable == nil || !*current.schedulable ||
		current.status == nil || !strings.EqualFold(*current.status, "active") {
		return "账号并发或托管基线已变化，保留当前值并等待重新计算", nil
	}
	reader, ok := s.repository.(interface {
		RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error)
	})
	if !ok {
		return "", errors.New("缺少账号分组与容量策略，无法确认并发恢复范围")
	}
	accounts, err := reader.RoutingAccounts(ctx, &target.AccountID, nil)
	if err != nil {
		return "", err
	}
	document, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return "", err
	}
	scope, err := routing.UpstreamCapacityScope(document, accounts)
	if err != nil {
		return "", err
	}
	if enabled, found := scope[target.AccountID]; !found || enabled {
		return "账号并发控制范围已变化，等待重新计算后再分配", nil
	}
	return "", nil
}
