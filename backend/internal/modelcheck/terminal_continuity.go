package modelcheck

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"slices"
	"time"
)

const terminalContinuitySkill = "sub2api-terminal-continuity"

type TerminalContinuityRequest struct {
	Rounds         int               `json:"rounds,omitempty"`
	Targets        []AnimationTarget `json:"targets"`
	TimeoutSeconds int               `json:"timeout_seconds"`
}

type TerminalContinuityResult struct {
	Incomplete    bool                      `json:"incomplete,omitempty"`
	RoundResults  []TerminalContinuityRound `json:"round_results,omitempty"`
	AccountID     string                    `json:"account_id"`
	AccountName   string                    `json:"account_name"`
	Model         string                    `json:"model"`
	ResponseModel string                    `json:"response_model,omitempty"`
	RequestID     string                    `json:"request_id"`
	Verdict       string                    `json:"verdict"`
	Response      string                    `json:"response,omitempty"`
	Error         string                    `json:"error,omitempty"`
	DurationMS    int64                     `json:"duration_ms"`
	CompletedAt   string                    `json:"completed_at"`
}

func (s *Service) TerminalContinuityHistory(ctx context.Context) ([]taskstore.Task, error) {
	values, err := s.tasks.ListBySkill(ctx, terminalContinuitySkill, 20)
	if err != nil {
		return nil, err
	}
	managed, err := s.tasks.ListBySkill(ctx, animationSkill, 200)
	if err != nil {
		return nil, err
	}
	for _, task := range managed {
		if task.Operation != "managed-model-detection" {
			continue
		}
		enabled := false
		switch configuration := task.Result["configuration"].(type) {
		case DetectionTask:
			enabled = configuration.Terminal
		case map[string]any:
			enabled, _ = configuration["terminal"].(bool)
		}
		if enabled {
			values = append(values, task)
		}
	}
	slices.SortFunc(values, func(a, b taskstore.Task) int {
		left, _ := time.Parse(time.RFC3339Nano, a.CreatedAt)
		right, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
		return right.Compare(left)
	})
	if len(values) > 20 {
		values = values[:20]
	}
	return values, nil
}

func (s *Service) EnqueueTerminalContinuity(context.Context, TerminalContinuityRequest) (taskstore.Task, error) {
	return taskstore.Task{}, errors.New("终端检测已移除，请使用动画检测或前置检测")
}
