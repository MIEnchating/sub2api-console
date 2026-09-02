package alerting

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type Repository interface {
	EvaluateAlertIncidents(context.Context) (business.AlertEvidenceResult, error)
	RecordAlertEvaluation(context.Context, string, business.AlertEvidenceResult, business.AlertDeliveryResult) (business.AlertEvaluationRecord, error)
}

type Deliverer interface {
	Deliver(context.Context, bool) (business.AlertDeliveryResult, error)
}

type Result struct {
	business.AlertEvaluationRecord
	Source             string                       `json:"source"`
	Findings           int                          `json:"findings"`
	Delivery           business.AlertDeliveryResult `json:"delivery"`
	RemoteWrite        bool                         `json:"remote_write"`
	EvaluationDisabled bool                         `json:"evaluation_disabled,omitempty"`
}

type Service struct {
	repository Repository
	deliverer  Deliverer
	now        func() time.Time
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type TaskService struct {
	evaluator  *Service
	tasks      TaskStore
	taskRunner taskrunner.Runner
	timeout    time.Duration
}

func New(repository Repository, deliverer Deliverer) *Service {
	return &Service{repository: repository, deliverer: deliverer, now: time.Now}
}

func NewTaskService(evaluator *Service, tasks TaskStore) *TaskService {
	return &TaskService{evaluator: evaluator, tasks: tasks, timeout: 10 * time.Minute}
}

func (s *TaskService) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

func (s *TaskService) Enqueue(ctx context.Context) (taskstore.Task, error) {
	id, err := randomTaskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-alert-evaluation", Operation: "evaluate", Status: "queued", Progress: 0,
		Message: "告警检测已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) { s.execute(parent, task) }); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *TaskService) execute(parent context.Context, task taskstore.Task) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, "正在检测告警并发送通知", time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	result, err := s.evaluator.Evaluate(ctx)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		task.Status, task.Message, task.Result = "failed", "告警检测失败", map[string]any{"error": err.Error()}
	} else {
		task.Status = "succeeded"
		if result.Status == "failed" {
			task.Status = "failed"
		}
		task.Message = result.Summary
		if task.Message == "" {
			task.Message = fmt.Sprintf("告警检测完成：当前异常 %d 项，发送 %d 项", result.Findings, result.Delivery.Sent)
		}
		task.Result = map[string]any{
			"run_key": result.RunKey, "event_id": result.EventID, "source": result.Source, "findings": result.Findings,
			"status": result.Status, "summary": result.Summary, "delivery": result.Delivery,
			"remote_write": result.RemoteWrite, "evaluation_disabled": result.EvaluationDisabled,
		}
	}
	taskstore.MarkCancelled(ctx, &task, "告警检测已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func randomTaskID() (string, error) {
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func (s *Service) Evaluate(ctx context.Context) (Result, error) {
	started := s.now().UTC().Format(time.RFC3339Nano)
	evidence, err := s.repository.EvaluateAlertIncidents(ctx)
	if err != nil {
		return Result{}, err
	}
	delivery := business.AlertDeliveryResult{MessageIDs: []string{}}
	if evidence.EvaluationDisabled {
		delivery.Disabled = true
	} else {
		delivery, err = s.deliverer.Deliver(ctx, false)
		if err != nil {
			return Result{}, err
		}
	}
	record, err := s.repository.RecordAlertEvaluation(ctx, started, evidence, delivery)
	if err != nil {
		return Result{}, err
	}
	return Result{
		AlertEvaluationRecord: record, Source: "console-domain-db", Findings: evidence.Findings,
		Delivery: delivery, RemoteWrite: false, EvaluationDisabled: evidence.EvaluationDisabled,
	}, nil
}
