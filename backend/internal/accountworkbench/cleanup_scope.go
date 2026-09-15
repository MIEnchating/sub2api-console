package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type cleanupPrivateStore interface {
	WorkbenchExecutionSummaries(context.Context, string) ([]configstore.WorkbenchExecutionSummary, error)
	DeleteWorkbenchExecution(context.Context, string, string, int64) error
}
type cleanupTaskStore interface {
	CleanupHistory(context.Context, string) ([]taskstore.Task, error)
	DeleteTerminalBySkill(context.Context, string, []taskstore.HistorySelection) error
}

func (s *Service) cleanupScope(ctx context.Context, owner string, target configstore.TargetSettings, input CleanupPreviewInput, skipTaskID string) (cleanupSnapshot, error) {
	result := cleanupSnapshot{view: CleanupPreview{Target: target.BaseURL, Profiles: []LoginProfileView{}, Items: []CleanupItem{}, Blockers: []CleanupBlocker{}}, exports: map[string]string{}, security: map[string]string{}}
	if len(input.Items) < 1 || len(input.Items) > 500 {
		return result, errors.New("请选择 1～500 份登录资料")
	}
	store, err := s.profileStore()
	if err != nil {
		return result, err
	}
	profiles, err := store.WorkbenchLoginProfiles(ctx, executionTargetFingerprint(target))
	if err != nil {
		return result, err
	}
	byID := map[string]configstore.WorkbenchLoginProfile{}
	for _, profile := range profiles {
		byID[profile.ID] = profile
	}
	seen := map[string]bool{}
	for _, item := range input.Items {
		profile, ok := byID[item.ID]
		if !ok || item.Revision < 1 || profile.Revision != item.Revision || seen[item.ID] {
			return result, configstore.ErrWorkbenchLoginProfile
		}
		seen[item.ID] = true
		result.profiles = append(result.profiles, profile)
		result.view.Profiles = append(result.view.Profiles, loginProfileView(profile))
		result.view.Items = append(result.view.Items, cleanupFileItem(profile.ID, "login_profile", 1, []string{profile.AccountID}, true))
	}
	s.mu.Lock()
	exports := s.exportState
	s.mu.Unlock()
	if exports != nil {
		items, snapshots, err := exports.cleanupExports(exportHash(owner), exportTargetHash(target), result.profiles)
		if err != nil {
			return result, err
		}
		result.view.Items = append(result.view.Items, items...)
		result.exports = snapshots
	}
	s.securityMu.Lock()
	security := s.securityStorage
	s.securityMu.Unlock()
	if security != nil {
		items, snapshots, err := security.cleanupSecurity(executionTargetFingerprint(target), result.profiles)
		if err != nil {
			return result, err
		}
		result.view.Items = append(result.view.Items, items...)
		result.security = snapshots
	}
	private, ok := s.private.(cleanupPrivateStore)
	if !ok {
		return result, errors.New("账号私有记录清理服务尚未就绪")
	}
	executions, err := private.WorkbenchExecutionSummaries(ctx, executionTargetFingerprint(target))
	if err != nil {
		return result, err
	}
	executionItems := map[string]CleanupItem{}
	for _, execution := range executions {
		matches := []string{}
		all := len(execution.Items) > 0
		for _, item := range execution.Items {
			match := ""
			for _, profile := range result.profiles {
				identity, _ := json.Marshal([]string{profile.WorkspaceID, profile.UserID})
				if item.Identity == "identity:"+string(identity) && (item.AccountID == "" || item.AccountID == profile.AccountID) && (item.OriginalAccountID == "" || item.OriginalAccountID == profile.AccountID) {
					match = profile.AccountID
					break
				}
			}
			if match == "" {
				all = false
			} else {
				matches = append(matches, match)
			}
		}
		if len(matches) == 0 {
			continue
		}
		item := cleanupFileItem(execution.ID, "execution", len(execution.Items), matches, all)
		executionItems[execution.ID] = item
		result.view.Items = append(result.view.Items, item)
		if all {
			result.executions = append(result.executions, execution)
		}
	}
	tasks, ok := s.tasks.(cleanupTaskStore)
	if !ok {
		return result, errors.New("账号历史清理服务尚未就绪")
	}
	history, err := tasks.CleanupHistory(ctx, Skill)
	if err != nil {
		return result, err
	}
	artifacts := map[string]CleanupItem{}
	for _, item := range result.view.Items {
		if item.Kind == "account_export" || item.Kind == "profile_export" || item.Kind == "security_result" {
			artifacts[item.ID] = item
		}
	}
	for _, task := range history {
		if task.ID == skipTaskID {
			continue
		}
		if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_input" {
			result.view.Blockers = append(result.view.Blockers, CleanupBlocker{TaskID: task.ID, Operation: task.Operation, Status: task.Status, Message: "工作台任务仍在执行，请结束后重新预览清理范围"})
			continue
		}
		item, ok := executionItems[task.ID]
		if !ok {
			item, ok = cleanupHistoryItem(task, result.profiles, artifacts)
		}
		if !ok {
			continue
		}
		item.ID, item.Kind = task.ID, "history"
		result.view.Items = append(result.view.Items, item)
		if item.Action == "delete" {
			result.history = append(result.history, taskstore.HistorySelection{ID: task.ID, UpdatedAt: task.UpdatedAt})
		}
	}
	result.view.Blocked = len(result.view.Blockers) > 0
	s.oauthMu.Lock()
	busy := s.oauthBusy || s.oauthBatchID != ""
	s.oauthMu.Unlock()
	if busy && !result.view.Blocked {
		result.view.Blockers = append(result.view.Blockers, CleanupBlocker{TaskID: "browser-cleanup", Operation: "account-workbench-oauth", Status: "running", Message: "授权浏览器或短信收尾仍在执行，请结束后重新预览"})
		result.view.Blocked = true
	}
	sort.Slice(result.view.Items, func(i, j int) bool {
		a, b := result.view.Items[i], result.view.Items[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
	return result, nil
}

type cleanupHistoryRow struct {
	AccountID   string `json:"account_id"`
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id"`
	ProfileID   string `json:"profile_id"`
	Email       string `json:"email"`
	ArtifactID  string `json:"artifact_id"`
	Report      struct {
		ArtifactID string `json:"artifact_id"`
	} `json:"report"`
}

func cleanupHistoryItem(task taskstore.Task, profiles []configstore.WorkbenchLoginProfile, artifacts map[string]CleanupItem) (CleanupItem, bool) {
	raw, err := json.Marshal(task.Result["items"])
	if err != nil {
		return CleanupItem{}, false
	}
	var rows []cleanupHistoryRow
	if json.Unmarshal(raw, &rows) != nil {
		return CleanupItem{}, false
	}
	if len(rows) == 0 {
		raw, _ = json.Marshal(task.Result)
		var row cleanupHistoryRow
		if json.Unmarshal(raw, &row) != nil {
			return CleanupItem{}, false
		}
		rows = []cleanupHistoryRow{row}
	}
	matches := []string{}
	all := true
	for _, row := range rows {
		artifactID := row.ArtifactID
		if artifactID == "" {
			artifactID = row.Report.ArtifactID
		}
		if artifact, ok := artifacts[artifactID]; ok {
			matches = append(matches, artifact.AccountIDs...)
			if artifact.Action != "delete" {
				all = false
			}
			continue
		}
		match := ""
		for _, profile := range profiles {
			if row.AccountID != profile.AccountID {
				continue
			}
			matches = append(matches, profile.AccountID)
			if row.UserID == profile.UserID && row.WorkspaceID == profile.WorkspaceID && row.ProfileID == profile.ID && (row.Email == "" || strings.EqualFold(row.Email, profile.Email)) {
				match = profile.AccountID
			}
			break
		}
		if match == "" {
			all = false
		}
	}
	if len(matches) == 0 {
		return CleanupItem{}, false
	}
	return cleanupFileItem(task.ID, "history", len(rows), matches, all), true
}
