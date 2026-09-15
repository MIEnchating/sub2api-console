package accountworkbench

import (
	"context"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) validateOAuthQueuePayload(ctx context.Context, owner string, scope ExportScope, target configstore.TargetSettings, payload *oauthQueuePayload) ([]InputItem, error) {
	if payload.Version != 1 || len(payload.View.Items) == 0 || len(payload.View.Items) > maxInputItems || len(payload.Inputs) != len(payload.View.Items) {
		return nil, configstore.ErrWorkbenchQueue
	}
	stored, err := normalizeWorkbenchScope(payload.View.Scope)
	if err != nil || stored != scope {
		return nil, configstore.ErrWorkbenchQueue
	}
	results, err := restoreQueueItems(payload.Results, len(payload.View.Items))
	if err != nil {
		return nil, err
	}
	byIndex := map[int]InputItem{}
	for _, result := range results {
		byIndex[result.Index] = result
	}
	for i := range payload.View.Items {
		row := &payload.View.Items[i]
		if row.Index != i {
			return nil, configstore.ErrWorkbenchQueue
		}
		result, available := byIndex[i]
		if (row.Status == "succeeded") != available {
			return nil, configstore.ErrWorkbenchQueue
		}
		if available && (!strings.EqualFold(result.Email, row.Email) || (row.WorkspaceID != "" && stringValue(result.Credentials["chatgpt_account_id"]) != row.WorkspaceID)) {
			return nil, configstore.ErrWorkbenchQueue
		}
		if row.Status == "running" {
			if checkpoint, ok := payload.Checkpoints[i]; ok && checkpoint.ID != "" {
				resumable, err := s.validateOAuthQueueCheckpoint(ctx, owner, scope, target, *row, &checkpoint)
				if err != nil {
					return nil, err
				}
				if resumable {
					payload.Checkpoints[i] = checkpoint
					payload.Inputs[i] = OAuthLoginInput{}
					continue
				}
			}
			row.Status, row.Message = "failed", "上次授权提交结果未确认，请核对原检查点或单独重新授权"
			delete(payload.Checkpoints, i)
			payload.Inputs[i] = OAuthLoginInput{}
		}
		if row.Status == "queued" {
			if payload.Inputs[i].Email != row.Email || payload.Inputs[i].WorkspaceID != row.WorkspaceID {
				return nil, configstore.ErrWorkbenchQueue
			}
			assist, err := s.prepareOAuthAssist(&payload.Inputs[i])
			if err != nil {
				return nil, err
			}
			assist.close()
		} else if row.Status != "succeeded" && row.Status != "failed" && row.Status != "cancelled" {
			return nil, configstore.ErrWorkbenchQueue
		}
	}
	return results, nil
}
