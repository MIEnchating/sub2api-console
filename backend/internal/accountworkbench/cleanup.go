package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) PreviewAccountCleanup(ctx context.Context, owner string, input CleanupPreviewInput) (CleanupPreview, error) {
	if owner == "" {
		return CleanupPreview{}, ErrPreview
	}
	ctx, err := targetguard.Pin(ctx, s.private)
	if err != nil {
		return CleanupPreview{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return CleanupPreview{}, err
	}
	snapshot, err := s.cleanupScope(ctx, owner, target, input, "")
	if err != nil {
		return CleanupPreview{}, err
	}
	if _, err := targetguard.Pin(ctx, s.private); err != nil {
		return CleanupPreview{}, err
	}
	id, err := randomID()
	if err != nil {
		return CleanupPreview{}, err
	}
	expires := time.Now().Add(10 * time.Minute)
	snapshot.view.ID, snapshot.view.ExpiresAt = id, expires.UTC().Format(time.RFC3339Nano)
	prepared := &preparedCleanup{owner: owner, target: target, expires: expires, input: CleanupPreviewInput{Items: append([]ProfileExportSelection(nil), input.Items...)}, snapshot: snapshot}
	s.mu.Lock()
	if s.cleanupPreviews == nil {
		s.cleanupPreviews = map[string]*preparedCleanup{}
	}
	for key, previous := range s.cleanupPreviews {
		if previous.owner == owner || time.Now().After(previous.expires) {
			delete(s.cleanupPreviews, key)
		}
	}
	if len(s.cleanupPreviews) >= 20 {
		s.mu.Unlock()
		return CleanupPreview{}, errors.New("清理预览已满，请稍后重试")
	}
	s.cleanupPreviews[id] = prepared
	s.mu.Unlock()
	time.AfterFunc(time.Until(expires), func() { s.DeleteCleanupPreview(owner, id) })
	return cloneCleanupView(snapshot.view), nil
}

func cloneCleanupView(view CleanupPreview) CleanupPreview {
	view.Profiles = append([]LoginProfileView{}, view.Profiles...)
	view.Blockers = append([]CleanupBlocker{}, view.Blockers...)
	view.Items = append([]CleanupItem{}, view.Items...)
	for i := range view.Items {
		view.Items[i].AccountIDs = append([]string{}, view.Items[i].AccountIDs...)
	}
	return view
}

func (s *Service) DeleteCleanupPreview(owner, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value := s.cleanupPreviews[id]; value != nil && value.owner == owner {
		delete(s.cleanupPreviews, id)
	}
}

func (s *Service) CleanupAccounts(ctx context.Context, owner, id string, confirmed bool) (taskstore.Task, error) {
	if !confirmed {
		return taskstore.Task{}, errors.New("请先确认账号关联资料和处理记录的清理范围")
	}
	s.mu.Lock()
	prepared := s.cleanupPreviews[id]
	if prepared == nil || prepared.owner != owner || time.Now().After(prepared.expires) {
		s.mu.Unlock()
		return taskstore.Task{}, ErrPreview
	}
	if prepared.snapshot.view.Blocked {
		s.mu.Unlock()
		return taskstore.Task{}, errors.New("工作台仍有活动任务，请结束后重新预览清理范围")
	}
	delete(s.cleanupPreviews, id)
	s.mu.Unlock()
	if _, err := targetguard.Pin(targetguard.Expect(ctx, prepared.target), s.private); err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "account-workbench-cleanup", "等待清理账号关联资料", func(run context.Context, update func([]ResultItem) error) ([]ResultItem, error) {
		s.cleanupMu.Lock()
		defer s.cleanupMu.Unlock()
		tasks, ok := s.tasks.(cleanupTaskStore)
		if !ok {
			return nil, errors.New("账号历史清理服务尚未就绪")
		}
		active, err := tasks.CleanupHistory(run, Skill)
		if err != nil {
			return nil, err
		}
		for _, task := range active {
			if task.ID != taskcontext.ID(run) && (task.Status == "queued" || task.Status == "running" || task.Status == "waiting_input") {
				return nil, errors.New("清理前发现新的活动任务，未删除任何资料，请重新预览")
			}
		}
		guarded, release, err := targetguard.Acquire(targetguard.Expect(run, prepared.target), s.repository)
		if err != nil {
			return nil, err
		}
		defer func() { _ = release() }()
		guarded, err = targetguard.Bind(guarded, s.private)
		if err != nil {
			return nil, err
		}
		current, err := s.cleanupScope(guarded, owner, prepared.target, prepared.input, taskcontext.ID(run))
		if err != nil {
			return nil, err
		}
		if current.view.Blocked {
			return nil, errors.New("清理前发现新的活动任务，未删除任何资料，请重新预览")
		}
		if !sameCleanupScope(prepared.snapshot, current) {
			return nil, errors.New("账号关联资料或处理记录已变化，未执行清理，请重新预览")
		}
		return s.executeCleanup(guarded, prepared, current, update)
	})
}

func sameCleanupScope(a, b cleanupSnapshot) bool {
	x, y := a.view, b.view
	x.ID, x.ExpiresAt, y.ID, y.ExpiresAt = "", "", "", ""
	left, _ := json.Marshal(struct {
		View       CleanupPreview
		Exports    map[string]string
		Security   map[string]string
		Executions []configstore.WorkbenchExecutionSummary
		History    []taskstore.HistorySelection
	}{x, a.exports, a.security, a.executions, a.history})
	right, _ := json.Marshal(struct {
		View       CleanupPreview
		Exports    map[string]string
		Security   map[string]string
		Executions []configstore.WorkbenchExecutionSummary
		History    []taskstore.HistorySelection
	}{y, b.exports, b.security, b.executions, b.history})
	return string(left) == string(right)
}

func (s *Service) executeCleanup(ctx context.Context, prepared *preparedCleanup, snapshot cleanupSnapshot, update func([]ResultItem) error) (rows []ResultItem, resultErr error) {
	rows = []ResultItem{}
	defer func() {
		if resultErr == nil {
			return
		}
		for i := range rows {
			if rows[i].Status == "queued" {
				rows[i].Status, rows[i].Message = "skipped", "清理已停止，未删除此项"
			}
			if rows[i].Status == "running" {
				rows[i].Status, rows[i].Message = "failed", "清理中断，请核对本项是否已删除"
			}
		}
	}()
	for _, item := range snapshot.view.Items {
		if item.Action == "delete" && item.Kind != "login_profile" {
			rows = append(rows, ResultItem{Index: len(rows), Name: item.ID, Status: "queued", Message: item.Kind})
		}
	}
	for _, profile := range snapshot.profiles {
		rows = append(rows, ResultItem{Index: len(rows), Name: profile.ID, AccountID: profile.AccountID, Status: "queued", Message: "login_profile"})
	}
	for i := range rows {
		if err := ctx.Err(); err != nil {
			return rows, err
		}
		kind := rows[i].Message
		rows[i].Status, rows[i].Message = "running", "正在清理账号关联资料"
		if err := update(rows); err != nil {
			return rows, err
		}
		err := s.deleteCleanupItem(ctx, prepared, snapshot, kind, rows[i].Name)
		if err != nil {
			rows[i].Status, rows[i].Message = "failed", "资料已变化或清理失败，请核对已完成项后重新预览"
			return rows, err
		}
		rows[i].Status, rows[i].Message = "succeeded", "已清理"
		if err := update(rows); err != nil {
			return rows, err
		}
	}
	return rows, nil
}

func (s *Service) deleteCleanupItem(ctx context.Context, prepared *preparedCleanup, snapshot cleanupSnapshot, kind, id string) error {
	switch kind {
	case "login_profile":
		store, err := s.profileStore()
		if err != nil {
			return err
		}
		for _, profile := range snapshot.profiles {
			if profile.ID == id {
				return store.DeleteWorkbenchLoginProfile(ctx, executionTargetFingerprint(prepared.target), id, profile.Revision)
			}
		}
	case "execution":
		store := s.private.(cleanupPrivateStore)
		for _, execution := range snapshot.executions {
			if execution.ID == id {
				return store.DeleteWorkbenchExecution(ctx, executionTargetFingerprint(prepared.target), id, execution.Revision)
			}
		}
	case "history":
		store := s.tasks.(cleanupTaskStore)
		for _, item := range snapshot.history {
			if item.ID == id {
				return store.DeleteTerminalBySkill(ctx, Skill, []taskstore.HistorySelection{item})
			}
		}
	case "account_export", "profile_export":
		state, err := s.exportStorage()
		if err != nil {
			return err
		}
		return state.cleanupRemoveExport(exportHash(prepared.owner), exportTargetHash(prepared.target), id, snapshot.exports[id])
	case "security_result":
		s.securityMu.Lock()
		storage := s.securityStorage
		s.securityMu.Unlock()
		if storage == nil {
			return ErrSecurityStorage
		}
		return storage.cleanupRemoveSecurity(id, snapshot.security[id])
	}
	return errors.New("清理项目不在确认范围内")
}
