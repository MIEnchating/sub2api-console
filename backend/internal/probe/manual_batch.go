package probe

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func manualBatchIDs(request Request) ([]string, error) {
	if request.SelectedAccountIDs == nil {
		return nil, nil
	}
	if len(request.SelectedAccountIDs) == 0 || len(request.SelectedAccountIDs) > 100 {
		return nil, errors.New("批量探活每次请选择 1 至 100 个账号")
	}
	if request.AccountID != nil || request.GroupName != nil || request.Platform != nil ||
		request.Automatic || len(request.AccountIDs) > 0 || request.ProbeModel != "" || len(request.ProbeModels) > 0 {
		return nil, errors.New("批量探活不能与其他探测范围或模型参数混用")
	}
	ids := make([]string, 0, len(request.SelectedAccountIDs))
	seen := map[string]bool{}
	for _, raw := range request.SelectedAccountIDs {
		id := strings.TrimSpace(raw)
		if !stablePositiveID(id) || len(id) > 32 || seen[id] {
			return nil, errors.New("批量探活账号必须使用不重复的稳定数字 ID")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func selectManualCandidates(candidates []business.ProbeCandidate, ids []string) ([]business.ProbeCandidate, error) {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = false
	}
	result := make([]business.ProbeCandidate, 0)
	for _, candidate := range candidates {
		if _, found := selected[candidate.AccountID]; found {
			selected[candidate.AccountID] = true
			result = append(result, candidate)
		}
	}
	for _, id := range ids {
		if !selected[id] {
			return nil, fmt.Errorf("账号 %s 不存在、没有分组或处于人工优先位，请刷新账号后重新选择", id)
		}
	}
	return result, nil
}
