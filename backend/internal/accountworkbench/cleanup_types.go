package accountworkbench

import (
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type CleanupPreviewInput struct {
	Items []ProfileExportSelection `json:"items"`
}
type CleanupItem struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	AccountIDs []string `json:"account_ids"`
	Count      int      `json:"count"`
	Action     string   `json:"action"`
	Reason     string   `json:"reason"`
}
type CleanupBlocker struct {
	TaskID    string `json:"task_id"`
	Operation string `json:"operation"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}
type CleanupPreview struct {
	ID        string             `json:"id"`
	Target    string             `json:"target"`
	ExpiresAt string             `json:"expires_at"`
	Profiles  []LoginProfileView `json:"profiles"`
	Items     []CleanupItem      `json:"items"`
	Blocked   bool               `json:"blocked"`
	Blockers  []CleanupBlocker   `json:"blockers"`
}
type cleanupSnapshot struct {
	view       CleanupPreview
	profiles   []configstore.WorkbenchLoginProfile
	history    []taskstore.HistorySelection
	executions []configstore.WorkbenchExecutionSummary
	exports    map[string]string
	security   map[string]string
}
type preparedCleanup struct {
	owner    string
	target   configstore.TargetSettings
	expires  time.Time
	input    CleanupPreviewInput
	snapshot cleanupSnapshot
}
