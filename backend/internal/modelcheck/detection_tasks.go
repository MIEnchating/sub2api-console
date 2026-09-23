package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type DetectionTask struct {
	ID                string   `json:"id"`
	Version           int      `json:"version"`
	Name              string   `json:"name"`
	GroupIDs          []string `json:"group_ids"`
	Model             string   `json:"model"`
	Precheck          bool     `json:"precheck"`
	PrecheckQuestions []string `json:"precheck_questions,omitempty"`
	Terminal          bool     `json:"terminal"`
	TerminalRounds    int      `json:"terminal_rounds"`
	Automatic         bool     `json:"automatic"`
	ScheduleType      string   `json:"schedule_type"`
	IntervalMinutes   int      `json:"interval_minutes"`
	DailyTimes        []string `json:"daily_times,omitempty"`
	Timezone          string   `json:"timezone,omitempty"`
	TimeoutSeconds    int      `json:"timeout_seconds"`
}
type DetectionTaskView struct {
	DetectionTask
	Running    bool   `json:"running"`
	NextAt     string `json:"next_at,omitempty"`
	LastTaskID string `json:"last_task_id,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}
type detectionTaskRepository interface {
	LoadDetectionTasks(context.Context) ([]byte, error)
	SaveDetectionTasks(context.Context, []byte, string, string) error
	DetectionTaskAccountIDs(context.Context, []string) ([]string, error)
	DetectionTaskScope(context.Context, []string) (business.DetectionTaskScope, error)
}

type detectionTaskState struct {
	mu         sync.Mutex
	values     map[string]DetectionTask
	next       map[string]time.Time
	active     map[string]string
	last       map[string]string
	errors     map[string]string
	repository detectionTaskRepository
}

func detectionTiming(value DetectionTask) AnimationSchedule {
	return AnimationSchedule{ScheduleType: value.ScheduleType, IntervalMinutes: value.IntervalMinutes, DailyTimes: value.DailyTimes, Timezone: value.Timezone}
}
func cloneDetectionTask(value DetectionTask) DetectionTask {
	value.GroupIDs = slices.Clone(value.GroupIDs)
	value.DailyTimes = slices.Clone(value.DailyTimes)
	value.PrecheckQuestions = slices.Clone(value.PrecheckQuestions)
	return value
}
func validateDetectionTask(value DetectionTask) error {
	if strings.TrimSpace(value.Name) == "" || len([]rune(value.Name)) > 80 || strings.ContainsFunc(value.Name, unicode.IsControl) {
		return errors.New("任务名称需为 1～80 个字符，不能包含控制字符")
	}
	if value.Version < 0 || len(value.GroupIDs) < 1 || len(value.GroupIDs) > 100 {
		return errors.New("请选择 1 到 100 个分组并使用有效版本")
	}
	seen := map[string]bool{}
	for _, id := range value.GroupIDs {
		if !stablePositiveID(id) || seen[id] {
			return errors.New("分组 ID 无效或重复")
		}
		seen[id] = true
	}
	if strings.TrimSpace(value.Model) == "" || len(value.Model) > 256 || strings.ContainsFunc(value.Model, unicode.IsControl) {
		return errors.New("请输入有效的检测模型 ID（不超过 256 字符）")
	}
	if value.TimeoutSeconds < 5 || value.TimeoutSeconds > 120 {
		return errors.New("请求超时必须在 5 到 120 秒之间")
	}
	if value.TerminalRounds < 1 || value.TerminalRounds > 20 {
		return errors.New("终端检测轮数必须在 1 到 20 之间")
	}
	if value.Precheck {
		if _, err := normalizePrecheckQuestions(precheckMode, value.PrecheckQuestions); err != nil {
			return err
		}
	}
	if err := validateAnimationTiming(detectionTiming(value)); err != nil {
		return err
	}
	if value.ScheduleType != "daily" && (value.IntervalMinutes < 1 || value.IntervalMinutes > 1440) {
		return errors.New("检测间隔必须在 1 到 1440 分钟之间")
	}
	return nil
}
func (s *Service) loadDetectionTasks(ctx context.Context) error {
	state := &detectionTaskState{values: map[string]DetectionTask{}, next: map[string]time.Time{}, active: map[string]string{}, last: map[string]string{}, errors: map[string]string{}}
	state.repository, _ = s.accounts.(detectionTaskRepository)
	if state.repository != nil {
		raw, err := state.repository.LoadDetectionTasks(ctx)
		if err != nil {
			return err
		}
		if len(raw) > 0 {
			var values []DetectionTask
			if err := json.Unmarshal(raw, &values); err != nil {
				return errors.New("检测任务配置无效")
			}
			for _, value := range values {
				if err := validateDetectionTask(value); err != nil {
					return err
				}
				if value.ID == "" || state.values[value.ID].ID != "" {
					return errors.New("检测任务 ID 无效或重复")
				}
				state.values[value.ID] = value
				state.next[value.ID] = nextAnimationTime(detectionTiming(value), time.Now())
			}
		}
	}
	s.detectionTasks = state
	history, err := s.tasks.ListBySkill(ctx, animationSkill, 200)
	if err != nil {
		return err
	}
	for _, task := range history {
		id, _ := task.Result["detection_task_id"].(string)
		if _, exists := state.values[id]; exists && s.detectionTasks.last[id] == "" {
			s.detectionTasks.last[id] = task.ID
		}
	}
	return nil
}
func (s *Service) DetectionTasks() []DetectionTaskView {
	state := s.detectionTasks
	state.mu.Lock()
	defer state.mu.Unlock()
	values := []DetectionTaskView{}
	for id, value := range state.values {
		item := DetectionTaskView{DetectionTask: cloneDetectionTask(value), Running: state.active[id] != "", LastTaskID: state.last[id], LastError: state.errors[id]}
		if value.Automatic {
			item.NextAt = state.next[id].UTC().Format(time.RFC3339Nano)
		}
		values = append(values, item)
	}
	slices.SortFunc(values, func(a, b DetectionTaskView) int { return strings.Compare(a.ID, b.ID) })
	return values
}
func (s *Service) SaveDetectionTask(ctx context.Context, value DetectionTask, actor string) ([]DetectionTaskView, error) {
	if err := validateDetectionTask(value); err != nil {
		return nil, err
	}
	state := s.detectionTasks
	if state.repository == nil {
		return nil, errors.New("检测任务存储尚未就绪")
	}
	if _, err := state.repository.DetectionTaskAccountIDs(ctx, value.GroupIDs); err != nil {
		return nil, err
	}
	value = cloneDetectionTask(value)
	value.Name = strings.TrimSpace(value.Name)
	value.Model = strings.TrimSpace(value.Model)
	slices.Sort(value.DailyTimes)
	state.mu.Lock()
	if value.ID == "" {
		if value.Version != 0 {
			state.mu.Unlock()
			return nil, errors.New("新任务版本必须为 0")
		}
		id, err := randomTaskID()
		if err != nil {
			state.mu.Unlock()
			return nil, err
		}
		value.ID = id
	} else if old, ok := state.values[value.ID]; !ok || old.Version != value.Version {
		state.mu.Unlock()
		return nil, errors.New("检测任务已变化，请刷新后重新编辑")
	}
	value.Version++
	values := make([]DetectionTask, 0, len(state.values)+1)
	for id, item := range state.values {
		if id != value.ID {
			values = append(values, item)
		}
	}
	values = append(values, value)
	raw, err := json.Marshal(values)
	if err == nil {
		err = state.repository.SaveDetectionTasks(ctx, raw, actor, "detection.task.saved")
	}
	if err == nil {
		state.values[value.ID] = value
		state.next[value.ID] = nextAnimationTime(detectionTiming(value), time.Now())
		delete(state.errors, value.ID)
	}
	state.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.DetectionTasks(), nil
}
func (s *Service) DeleteDetectionTask(ctx context.Context, id string, version int, actor string) ([]DetectionTaskView, error) {
	state := s.detectionTasks
	state.mu.Lock()
	value, ok := state.values[id]
	if !ok || value.Version != version {
		state.mu.Unlock()
		return nil, errors.New("检测任务已变化，请刷新后重试")
	}
	if state.active[id] != "" {
		state.mu.Unlock()
		return nil, errors.New("任务正在执行，请等待结束或取消后再删除")
	}
	values := []DetectionTask{}
	for key, item := range state.values {
		if key != id {
			values = append(values, item)
		}
	}
	raw, err := json.Marshal(values)
	if err == nil {
		err = state.repository.SaveDetectionTasks(ctx, raw, actor, "detection.task.deleted")
	}
	if err == nil {
		delete(state.values, id)
		delete(state.next, id)
		delete(state.last, id)
		delete(state.errors, id)
	}
	state.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.DetectionTasks(), nil
}
