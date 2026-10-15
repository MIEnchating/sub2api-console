package modelcheck

import (
	"context"
	"errors"
	"maps"
	"time"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type qualityRecheckRepository interface {
	AnimationQualityRecheck(context.Context, string) (*q.Recheck, error)
	SaveAnimationQualityRecheck(context.Context, string, q.Recheck, int) (q.Recheck, error)
}
type qualityTaskReader interface {
	Get(context.Context, string) (taskstore.Task, error)
}
type AnimationQualityRecheckRequest struct {
	AccountID string `json:"account_id"`
	RequestID string `json:"request_id"`
	Version   int    `json:"version"`
}
type AnimationQualityRecheckTask struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *Service) withQualityRecheck(ctx context.Context, sample q.Sample) (review AnimationQualityReview, err error) {
	review = qualityReview(sample)
	defer func() {
		if err == nil {
			sample.Prediction = review.Prediction
			review.PredictionSHA256 = q.ReviewFingerprint(sample)
		}
	}()
	repo, ok := s.accounts.(qualityRecheckRepository)
	if !ok {
		return review, nil
	}
	state, err := repo.AnimationQualityRecheck(ctx, sample.ID)
	if err != nil || state == nil {
		return review, err
	}
	if state.SourceSHA256 != sample.SourceSHA256 || state.PromptSHA256 != sample.PromptSHA256 {
		return review, ErrQualityReview
	}
	review.RecheckVersion = state.Version
	if state.Prediction != nil {
		review.Prediction = *state.Prediction
	}
	reader, ok := s.tasks.(qualityTaskReader)
	if !ok {
		return review, errors.New("复核任务读取服务尚未就绪")
	}
	task, err := reader.Get(ctx, state.TaskID)
	if errors.Is(err, taskstore.ErrNotFound) {
		return review, nil
	}
	if err != nil {
		return review, err
	}
	review.Recheck = &AnimationQualityRecheckTask{TaskID: task.ID, Status: task.Status, Message: task.Message}
	return review, nil
}

func (s *Service) EnqueueAnimationQualityRecheck(ctx context.Context, taskID string, request AnimationQualityRecheckRequest, actor string) (taskstore.Task, error) {
	if request.Version < 0 || !validQualityText(actor, 512, false) {
		return taskstore.Task{}, ErrQualityReview
	}
	sample, err := s.qualitySample(ctx, taskID, request.AccountID, request.RequestID)
	if err != nil {
		return taskstore.Task{}, err
	}
	repo, ok := s.accounts.(qualityRecheckRepository)
	if !ok {
		return taskstore.Task{}, errors.New("复核存储尚未就绪")
	}
	reader, ok := s.tasks.(qualityTaskReader)
	if !ok {
		return taskstore.Task{}, errors.New("复核任务读取服务尚未就绪")
	}
	s.qualityRecheckMu.Lock()
	defer s.qualityRecheckMu.Unlock()
	state, err := repo.AnimationQualityRecheck(ctx, sample.ID)
	if err != nil {
		return taskstore.Task{}, err
	}
	if state == nil {
		state = &q.Recheck{SourceSHA256: sample.SourceSHA256, PromptSHA256: sample.PromptSHA256}
	}
	if state.SourceSHA256 != sample.SourceSHA256 || state.PromptSHA256 != sample.PromptSHA256 {
		return taskstore.Task{}, ErrQualityReview
	}
	if state.TaskID != "" {
		previous, readErr := reader.Get(ctx, state.TaskID)
		if readErr != nil && !errors.Is(readErr, taskstore.ErrNotFound) {
			return taskstore.Task{}, readErr
		}
		if readErr == nil && (previous.Status == "queued" || previous.Status == "running") {
			return previous, nil
		}
	}
	if request.Version != state.Version {
		return taskstore.Task{}, business.ErrQualityRecheckConflict
	}
	settings := s.AnimationQualitySettings()
	if settings.AccountID == "" || settings.Model == "" {
		return taskstore.Task{}, ErrQualitySettings
	}
	if _, err = s.qualityReviewer(ctx, settings.AccountID); err != nil {
		return taskstore.Task{}, ErrQualitySettings
	}
	id, err := randomTaskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: id, Skill: "sub2api-animation-quality", Operation: "animation-quality-recheck", Status: "queued", Message: "质量复核已排队", CreatedAt: now, UpdatedAt: now, Result: map[string]any{
		"source_task_id": taskID, "account_id": request.AccountID, "request_id": request.RequestID, "sample_id": sample.ID, "source_sha256": sample.SourceSHA256, "prompt_sha256": sample.PromptSHA256, "actor": actor, "phase": "queued", "settings_version": settings.Version, "rubric_version": q.ReviewRubricVersion,
	}}
	if err = s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	state.TaskID = task.ID
	saved, err := repo.SaveAnimationQualityRecheck(ctx, sample.ID, *state, state.Version)
	if err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	if err = taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) { s.executeQualityRecheck(parent, task, sample, settings, saved, repo) }); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) executeQualityRecheck(ctx context.Context, task taskstore.Task, sample q.Sample, settings AnimationQualitySettings, state q.Recheck, repo qualityRecheckRepository) {
	ctx, cancel := context.WithTimeout(ctx, s.taskTimeout)
	defer cancel()
	task.Result = maps.Clone(task.Result)
	task.Status, task.Message = "running", "正在复核原始动画"
	task.Result["phase"] = "reviewing"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	current, err := s.qualitySample(ctx, sample.TaskID, sample.AccountID, sample.RequestID)
	task.Status, task.Message = "failed", "原始结果已变化或不可读取，请重新打开后复核"
	if err == nil && current.SourceSHA256 == sample.SourceSHA256 && current.PromptSHA256 == sample.PromptSHA256 {
		prediction := s.reviewAnimationQuality(ctx, AnimationResult{Source: sample.HTML, Prompt: sample.Prompt, RequestID: task.ID}, settings, nil)
		task.Result["prediction"] = prediction
		task.Message = qualityRecheckFailure(prediction.Reason)
		if prediction.Reason == "" && ctx.Err() == nil {
			state.Prediction = &prediction
			_, err = repo.SaveAnimationQualityRecheck(ctx, sample.ID, state, state.Version)
			if err == nil {
				task.Status, task.Message = "succeeded", "质量复核完成"
			} else {
				task.Message = "复核结果保存失败，上一次自动结果已保留，请重试"
			}
		}
	}
	taskstore.MarkCancelled(ctx, &task, "质量复核已取消，上一次自动结果已保留")
	if taskstore.Interrupted(ctx) {
		task.Message = "服务停止导致质量复核中断；为避免重复评审未自动重试，请手动复核"
	}
	task.Progress = 100
	task.Result["phase"] = "completed"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}

func qualityRecheckFailure(reason string) string {
	switch reason {
	case "review_timeout":
		return "评审超时，上一次自动结果已保留；该历史记录未区分排队与请求，可重新评审"
	case "review_request_timeout":
		return "评审请求超时，上一次自动结果已保留；请检查评审服务或调整评审超时后重试"
	case "review_queue_timeout":
		return "评审排队期间任务已超时，未发送评审请求；上一次自动结果已保留，可重新评审"
	case "review_task_timeout":
		return "所属任务已超时，评审已中断；上一次自动结果已保留，可重新评审"
	case "review_account_unavailable":
		return "评审账号不可用或已被排除，上一次自动结果已保留；请重新配置评审账号"
	case "review_settings_changed":
		return "评审期间设置已变化，上一次自动结果已保留；请按当前设置重新复核"
	case "review_invalid_response", "review_insufficient_evidence":
		return "评审未返回有效结论或可靠证据，上一次自动结果已保留；可重新复核或人工标注"
	default:
		return "评审请求失败，上一次自动结果已保留；请检查评审账号和模型设置后重试"
	}
}
