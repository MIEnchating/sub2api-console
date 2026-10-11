package accountautomation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type TaskGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	ScheduleIDs []string `json:"schedule_ids"`
}

type TaskGroupResultItem struct {
	ScheduleID      string `json:"schedule_id"`
	ScheduleName    string `json:"schedule_name"`
	PreviewTaskID   string `json:"preview_task_id,omitempty"`
	ExecutionTaskID string `json:"execution_task_id,omitempty"`
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
}

func (s *Service) TaskGroups(ctx context.Context) ([]TaskGroup, error) {
	if s.scheduleRepo == nil {
		return []TaskGroup{}, nil
	}
	raw, err := s.scheduleRepo.JSONSetting(ctx, taskGroupSettingKey)
	if err != nil {
		return nil, err
	}
	return decodeTaskGroups(raw)
}

func decodeTaskGroups(raw string) ([]TaskGroup, error) {
	if strings.TrimSpace(raw) == "" {
		return []TaskGroup{}, nil
	}
	var groups []TaskGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil, errors.New("账号模型任务组配置损坏")
	}
	for _, group := range groups {
		if err := validateTaskGroup(group, nil); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func (s *Service) SaveTaskGroup(ctx context.Context, group TaskGroup) (TaskGroup, error) {
	group.Name = strings.TrimSpace(group.Name)
	if group.ID == "" {
		id, err := taskGroupID()
		if err != nil {
			return TaskGroup{}, err
		}
		group.ID = id
	}
	err := s.updateScheduleSettings(ctx, []string{scheduleSettingKey, taskGroupSettingKey}, func(settings *scheduleSettings) error {
		available := make(map[string]bool, len(settings.schedules))
		for _, schedule := range settings.schedules {
			available[schedule.ID] = true
		}
		if err := validateTaskGroup(group, available); err != nil {
			return err
		}
		for index := range settings.groups {
			if settings.groups[index].ID == group.ID {
				settings.groups[index] = group
				return nil
			}
		}
		settings.groups = append(settings.groups, group)
		return nil
	})
	if err != nil {
		return TaskGroup{}, err
	}
	return group, nil
}

func (s *Service) DeleteTaskGroup(ctx context.Context, id string) error {
	return s.updateScheduleSettings(ctx, []string{taskGroupSettingKey}, func(settings *scheduleSettings) error {
		filtered := settings.groups[:0]
		for _, group := range settings.groups {
			if group.ID != id {
				filtered = append(filtered, group)
			}
		}
		settings.groups = filtered
		return nil
	})
}

func (s *Service) RunTaskGroup(ctx context.Context, id string) (taskstore.Task, error) {
	if s.store == nil || s.groupRunner == nil {
		return taskstore.Task{}, errors.New("账号模型任务组服务尚未就绪")
	}
	s.scheduleMu.Lock()
	if s.runningTaskGroup {
		s.scheduleMu.Unlock()
		return taskstore.Task{}, errors.New("账号模型任务组正在执行")
	}
	s.runningTaskGroup = true
	s.scheduleMu.Unlock()
	release := func() {
		s.scheduleMu.Lock()
		s.runningTaskGroup = false
		s.scheduleMu.Unlock()
	}
	groups, err := s.TaskGroups(ctx)
	if err != nil {
		release()
		return taskstore.Task{}, err
	}
	var group TaskGroup
	found := false
	for _, candidate := range groups {
		if candidate.ID == id {
			group, found = candidate, true
			break
		}
	}
	if !found {
		release()
		return taskstore.Task{}, errors.New("任务组不存在")
	}
	if err := validateTaskGroup(group, nil); err != nil {
		release()
		return taskstore.Task{}, err
	}
	taskID, err := taskGroupID()
	if err != nil {
		release()
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: taskID, Skill: Skill, Operation: GroupOperation, Status: "queued",
		Message: "账号模型任务组已排队", CreatedAt: now, UpdatedAt: now,
		Result: map[string]any{"task_group_id": group.ID, "task_group_name": group.Name, "items": []TaskGroupResultItem{}},
	}
	if err := s.store.Save(ctx, task); err != nil {
		release()
		return taskstore.Task{}, err
	}
	queued := task
	queued.Result = maps.Clone(task.Result)
	if err := taskrunner.GoTask(s.groupRunner, task.ID, func(run context.Context) {
		defer release()
		s.runTaskGroup(run, task, group)
	}); err != nil {
		release()
		task.Status = "failed"
		task.Message = "任务组启动失败：" + err.Error()
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = s.store.Save(context.WithoutCancel(ctx), task)
		return taskstore.Task{}, err
	}
	return queued, nil
}

func (s *Service) runTaskGroup(ctx context.Context, task taskstore.Task, group TaskGroup) {
	ctx = context.WithValue(ctx, taskGroupOwnerKey{}, task.ID)
	task.Status, task.Message = "running", "正在依次执行运营任务"
	if !s.persist(ctx, task) {
		return
	}
	schedules, err := s.Schedules(ctx)
	if err != nil {
		s.finishTaskGroup(ctx, &task, "failed", "任务组读取失败："+err.Error())
		return
	}
	byID := make(map[string]Schedule, len(schedules))
	for _, schedule := range schedules {
		byID[schedule.ID] = schedule
	}
	items := make([]TaskGroupResultItem, 0, len(group.ScheduleIDs))
	succeeded := 0
	for index, scheduleID := range group.ScheduleIDs {
		item := TaskGroupResultItem{ScheduleID: scheduleID, Status: "failed"}
		schedule, exists := byID[scheduleID]
		if !exists {
			item.Message = "运营任务已不存在"
		} else if ctx.Err() != nil {
			item.ScheduleName, item.Status, item.Message = schedule.Name, "cancelled", "任务组已取消"
		} else {
			item.ScheduleName = schedule.Name
			preview, run, runErr := s.runSchedule(ctx, scheduleID, true)
			if runErr != nil {
				item.Message = runErr.Error()
			} else {
				item.PreviewTaskID = preview.ID
				item.Status, item.Message, item.ExecutionTaskID = s.waitScheduleRun(ctx, run)
			}
		}
		if item.Status == "succeeded" {
			succeeded++
		}
		items = append(items, item)
		task.Result["items"] = items
		task.Progress = (index + 1) * 100 / len(group.ScheduleIDs)
		task.Message = fmt.Sprintf("已完成 %d/%d 个运营任务", index+1, len(group.ScheduleIDs))
		if !s.persist(ctx, task) {
			return
		}
		if ctx.Err() != nil {
			break
		}
	}
	if ctx.Err() != nil {
		s.finishTaskGroup(ctx, &task, "cancelled", "任务组已取消")
		return
	}
	status := "succeeded"
	message := "任务组执行完成"
	if succeeded == 0 {
		status, message = "failed", "任务组执行失败，请查看成员结果"
	} else if succeeded < len(group.ScheduleIDs) {
		status, message = "partial", "任务组部分完成，请查看成员结果"
	}
	s.finishTaskGroup(ctx, &task, status, message)
}

func (s *Service) waitScheduleRun(ctx context.Context, run *scheduledRun) (string, string, string) {
	select {
	case <-run.done:
		return run.outcome.status, run.outcome.message, run.outcome.executionID
	case <-ctx.Done():
		return "cancelled", "任务组已取消", ""
	}
}

func (s *Service) cancelAutomationTask(taskID string) {
	if runner, ok := s.runner.(taskrunner.TaskRunner); ok {
		runner.CancelTask(taskID)
	}
}

func (s *Service) finishTaskGroup(ctx context.Context, task *taskstore.Task, status, message string) {
	if taskstore.Interrupted(ctx) {
		return
	}
	task.Status, task.Message, task.Progress = status, message, 100
	_ = taskstore.SaveFinal(context.WithoutCancel(ctx), s.store, *task)
}

func removeScheduleFromTaskGroups(groups []TaskGroup, scheduleID string) []TaskGroup {
	updated := groups[:0]
	for _, group := range groups {
		filtered := make([]string, 0, len(group.ScheduleIDs))
		for _, id := range group.ScheduleIDs {
			if id != scheduleID {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) >= 2 {
			group.ScheduleIDs = filtered
			updated = append(updated, group)
		}
	}
	return updated
}

func validateTaskGroup(group TaskGroup, available map[string]bool) error {
	if group.ID != "" && !validTaskGroupID(group.ID) {
		return errors.New("任务组 ID 无效")
	}
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" || utf8.RuneCountInString(group.Name) > 128 {
		return errors.New("任务组名称不能为空且不能超过 128 个字符")
	}
	if len(group.ScheduleIDs) < 2 || len(group.ScheduleIDs) > 50 {
		return errors.New("任务组需包含 2 到 50 个运营任务")
	}
	seen := make(map[string]bool, len(group.ScheduleIDs))
	for _, id := range group.ScheduleIDs {
		if !validTaskGroupID(id) || seen[id] {
			return errors.New("任务组成员必须使用不重复的运营任务 ID")
		}
		if available != nil && !available[id] {
			return errors.New("任务组包含不存在的运营任务")
		}
		seen[id] = true
	}
	return nil
}

func taskGroupID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func validTaskGroupID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
