package accountworkbench

import (
	"encoding/json"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func restoreMixedEntries(payload *mixedQueuePayload, results []InputItem) ([]mixedEntry, []int, error) {
	count := len(payload.View.Items)
	entries := make([]mixedEntry, count)
	ready := map[int]InputItem{}
	for _, item := range results {
		ready[item.Index] = item
	}
	for _, saved := range payload.Inputs {
		index := saved.Item.Index
		if index < 0 || index >= count || entries[index].item != nil || payload.View.Items[index].Kind == "oauth_login" {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		raw, err := json.Marshal(map[string]any{"name": saved.Item.Name, "email": saved.Item.Email, "credentials": saved.Credentials})
		if err != nil {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		parsed, failures := Parse(string(raw))
		if len(failures) != 0 || len(parsed) != 1 {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		parsed[0].Index, parsed[0].Kind = index, saved.Item.Kind
		if parsed[0].Kind != payload.View.Items[index].Kind || !strings.EqualFold(parsed[0].Email, payload.View.Items[index].Email) {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		entries[index].item = &parsed[0]
	}
	positions := []int{}
	for i := range payload.View.Items {
		row := &payload.View.Items[i]
		entries[i].index = i
		if row.Index != i {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		result, available := ready[i]
		if available && (row.Status != "succeeded" || row.Email != "" && !strings.EqualFold(result.Email, row.Email) || row.WorkspaceID != "" && stringValue(result.Credentials["chatgpt_account_id"]) != row.WorkspaceID) {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		if row.Kind == "oauth_login" {
			positions = append(positions, i)
			if payload.OAuth == nil && (row.Status == "queued" || row.Status == "running" || row.Status == "waiting_input" || row.Status == "succeeded" && !available) {
				return nil, nil, configstore.ErrWorkbenchQueue
			}
			continue
		}
		if row.Kind != "sub2api_json" && row.Kind != "codex_json" && row.Kind != "refresh_token" {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		if (row.Status == "succeeded") != available {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		if row.Status == "running" {
			row.Status, row.Message = "failed", "上次凭据刷新结果未确认，请核对后单独重新处理"
			entries[i].item = nil
		}
		if row.Status == "queued" && entries[i].item == nil {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
		if row.Status != "queued" && row.Status != "succeeded" && row.Status != "failed" && row.Status != "cancelled" {
			return nil, nil, configstore.ErrWorkbenchQueue
		}
	}
	return entries, positions, nil
}
