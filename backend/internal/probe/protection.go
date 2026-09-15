package probe

import (
	"context"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type protectionRepository interface {
	AccountMutationProtections(context.Context, []string) (map[string]business.AccountMutationProtection, error)
}

func (s *Service) applyCurrentProtections(ctx context.Context, targets []Target) ([]Target, error) {
	repository, ok := s.repository.(protectionRepository)
	if !ok || len(targets) == 0 {
		return targets, nil
	}
	ids := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if _, duplicate := seen[target.AccountID]; !duplicate {
			seen[target.AccountID] = struct{}{}
			ids = append(ids, target.AccountID)
		}
	}
	protections, err := repository.AccountMutationProtections(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("探测前人工保护状态复核失败：%w", err)
	}
	result := append([]Target{}, targets...)
	for index := range result {
		protection := protections[result[index].AccountID]
		if protection.ManualPriority {
			result[index].SkipReason = textPointer("账号在探测执行前进入人工优先位，已跳过")
		} else if protection.ManualFused {
			result[index].SkipReason = textPointer("账号在探测执行前被人工熔断，已跳过")
		}
	}
	return result, nil
}
