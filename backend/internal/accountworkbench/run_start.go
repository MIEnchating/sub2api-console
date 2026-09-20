package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type activeRun struct {
	done   chan struct{}
	owner  string
	taskID string
	mu     sync.Mutex
	prompt *activeLoginPrompt
}
type RunConfirmation struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

func (s *Service) Start(ctx context.Context, owner string, input RunConfirmation) (Run, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.tasks == nil || s.runner == nil {
		return Run{}, errors.New("工作台任务执行器尚未就绪")
	}
	preview, err := s.readPreview(ctx, owner, input.ID, input.Revision)
	if err != nil {
		return Run{}, err
	}
	if len(preview.Items) == 0 || len(preview.Public.Errors) > 0 {
		return Run{}, errors.New("请先修正输入错误并重新预览")
	}
	if preview.Public.Check && s.checker == nil {
		return Run{}, errors.New("账号检测服务尚未就绪")
	}
	preview.Claimed = true
	raw, err := json.Marshal(preview)
	if err != nil {
		return Run{}, err
	}
	claimedRevision, err := s.private.SaveWorkbenchDocument(ctx, "preview:"+owner, input.Revision, raw)
	if err != nil {
		return Run{}, err
	}
	now := time.Now().UTC()
	value := privateRun{Owner: owner, Target: preview.Target, Settings: preview.Settings, Template: preview.Public.Template, TemplateRevision: preview.TemplateRevision, Items: preview.Items, Exports: make([]map[string]any, len(preview.Items)), AccountVersions: map[string]string{}, Phases: map[string]string{}, Public: Run{ID: newID(), Status: "queued", Action: preview.Public.Action, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(2 * time.Hour), DuplicateCount: preview.Public.DuplicateCount, Items: []RunItem{}}}
	for _, item := range preview.Items {
		label := "默认配置"
		if value.Template != nil {
			label = value.Template.Name
		}
		value.Public.Items = append(value.Public.Items, RunItem{InputItem: item.Item, Status: "queued", TemplateName: label})
	}
	value.Scope = preview.Scope
	result, err := s.launch(ctx, &value)
	if err == nil {
		_ = s.private.DeleteWorkbenchDocument(ctx, "preview:"+owner, claimedRevision)
	}
	return result, err
}
func (s *Service) launch(ctx context.Context, value *privateRun) (Run, error) {
	if s.tasks == nil || s.runner == nil || (value.Settings.Check && s.checker == nil) {
		return Run{}, errors.New("工作台任务执行服务尚未就绪")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: newID(), Skill: Skill, Operation: "workbench-run", Status: "queued", Message: "账号处理已排队", Result: map[string]any{"run_id": value.Public.ID, "total": len(value.Items)}, CreatedAt: now, UpdatedAt: now}
	value.Public.TaskID = task.ID
	value.Public.Status = "queued"
	value.TaskIDs = append(value.TaskIDs, task.ID)
	if err := s.saveRun(ctx, value); err != nil {
		return Run{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		value.Public.Status = "interrupted"
		_ = s.persistRun(value)
		return Run{}, errors.New("任务保存失败，请在处理记录核对后继续")
	}
	// Return an immutable public snapshot; the worker owns the mutable document.
	raw, _ := json.Marshal(value.Public)
	var queued Run
	_ = json.Unmarshal(raw, &queued)
	active := &activeRun{done: make(chan struct{}), owner: value.Owner, taskID: task.ID}
	s.activeMu.Lock()
	s.active[value.Public.ID] = active
	s.activeMu.Unlock()
	err := s.runner.GoTask(task.ID, func(parent context.Context) {
		defer func() {
			s.activeMu.Lock()
			delete(s.active, value.Public.ID)
			close(active.done)
			s.activeMu.Unlock()
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.PruneRecords(cleanupCtx, value.Owner)
		}()
		s.execute(parent, value, task, active)
	})
	if err != nil {
		s.activeMu.Lock()
		delete(s.active, value.Public.ID)
		close(active.done)
		s.activeMu.Unlock()
		value.Public.Status = "interrupted"
		_ = s.persistRun(value)
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return Run{}, errors.New("任务未启动，请稍后从处理记录继续")
	}
	return queued, nil
}
func (s *Service) persistRun(value *privateRun) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.saveRun(ctx, value)
}
