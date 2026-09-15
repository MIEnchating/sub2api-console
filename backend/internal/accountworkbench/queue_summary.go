package accountworkbench

import (
	"encoding/json"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type queueSummaryRow struct {
	Index       int    `json:"index"`
	Email       string `json:"email"`
	WorkspaceID string `json:"workspace_id"`
	Status      string `json:"status"`
	Kind        string `json:"kind"`
}

type queueSummaryPayload struct {
	Version int `json:"version"`
	View    struct {
		Scope ExportScope       `json:"scope"`
		Items []queueSummaryRow `json:"items"`
	} `json:"view"`
	OAuth *queueSummaryPayload `json:"oauth"`
}

// Decode a separate whitelist so credential inputs, results, proxy settings and
// browser checkpoints never become fields of the recovery-list response.
func queueRecoverySummary(record configstore.WorkbenchQueue, scope ExportScope, active bool) (QueueRecoveryView, error) {
	var payload queueSummaryPayload
	if json.Unmarshal(record.Payload, &payload) != nil || payload.Version != 1 || len(payload.View.Items) == 0 || len(payload.View.Items) > maxInputItems {
		return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
	}
	stored, err := normalizeWorkbenchScope(payload.View.Scope)
	requested, requestErr := normalizeWorkbenchScope(scope)
	if err != nil || requestErr != nil || stored != requested {
		return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
	}
	if payload.OAuth != nil {
		if record.Kind != "mixed" || payload.OAuth.Version != 1 {
			return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
		}
		childScope, err := normalizeWorkbenchScope(payload.OAuth.View.Scope)
		if err != nil || childScope != stored {
			return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
		}
		child := 0
		for i := range payload.View.Items {
			row := &payload.View.Items[i]
			if row.Kind != "oauth_login" {
				continue
			}
			if child >= len(payload.OAuth.View.Items) {
				return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
			}
			peer := payload.OAuth.View.Items[child]
			if peer.Index != child || !strings.EqualFold(peer.Email, row.Email) || peer.WorkspaceID != row.WorkspaceID {
				return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
			}
			row.Status = peer.Status
			child++
		}
		if child != len(payload.OAuth.View.Items) {
			return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
		}
	}
	view := QueueRecoveryView{Scope: stored, Items: []QueueRecoveryItem{}, ID: record.ID, Kind: record.Kind, TaskID: record.TaskID, Status: record.Status, Revision: record.Revision, ExpiresAt: record.ExpiresAt, Active: active, CanResume: active || record.Status != "running"}
	for i, row := range payload.View.Items {
		if row.Index != i || len(row.Email) > 320 || len(row.WorkspaceID) > 500 || strings.ContainsAny(row.Email+row.WorkspaceID, "\r\n\x00") {
			return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
		}
		switch row.Status {
		case "queued":
			view.Pending++
		case "succeeded":
			view.Succeeded++
		case "running", "waiting_input":
			if active {
				view.Pending++
			} else {
				view.Review++
			}
		case "failed", "cancelled":
			view.Review++
		default:
			return QueueRecoveryView{}, configstore.ErrWorkbenchQueue
		}
		view.Items = append(view.Items, QueueRecoveryItem{Index: i, Email: row.Email, WorkspaceID: row.WorkspaceID, Status: row.Status})
	}
	return view, nil
}
