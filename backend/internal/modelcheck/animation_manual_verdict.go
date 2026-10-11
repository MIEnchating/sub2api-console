package modelcheck

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

var ErrInvalidAnimationVerdict = errors.New("手动标记参数无效，请刷新结果后重试")

type AnimationManualVerdict struct {
	Verdict   string `json:"verdict"`
	UpdatedAt string `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
}

type AnimationManualVerdictRequest struct {
	AccountID string `json:"account_id"`
	RequestID string `json:"request_id"`
	Verdict   string `json:"verdict"`
}

type animationResultEditor interface {
	UpdateTerminalResult(context.Context, string, func(taskstore.Task) error) (taskstore.Task, error)
}

func (s *Service) SetAnimationManualVerdict(ctx context.Context, taskID string, request AnimationManualVerdictRequest, actor string) (taskstore.Task, error) {
	for _, value := range []string{taskID, request.AccountID, request.RequestID, actor} {
		if strings.TrimSpace(value) != value || value == "" || len(value) > 512 || strings.ContainsFunc(value, unicode.IsControl) {
			return taskstore.Task{}, ErrInvalidAnimationVerdict
		}
	}
	if request.Verdict != "passed" && request.Verdict != "not_passed" && request.Verdict != "unmarked" {
		return taskstore.Task{}, ErrInvalidAnimationVerdict
	}
	editor, ok := s.tasks.(animationResultEditor)
	if !ok {
		return taskstore.Task{}, errors.New("手动标记存储尚未就绪")
	}
	return editor.UpdateTerminalResult(ctx, taskID, func(task taskstore.Task) error {
		if task.Skill != animationSkill || (task.Operation != "account-model-animation" && task.Operation != "account-model-combined" && task.Operation != "managed-model-detection") {
			return ErrInvalidAnimationVerdict
		}
		rows, ok := task.Result["animations"].([]any)
		if !ok {
			return ErrInvalidAnimationVerdict
		}
		var selected map[string]any
		for _, value := range rows {
			row, ok := value.(map[string]any)
			if !ok || row["account_id"] != request.AccountID || row["request_id"] != request.RequestID {
				continue
			}
			mode, _ := row["mode"].(string)
			if selected != nil || (mode != "" && mode != "animation") || (mode == "" && task.Result["mode"] == precheckMode) || row["precheck"] != nil || (row["status"] != "succeeded" && row["status"] != "failed") {
				return ErrInvalidAnimationVerdict
			}
			completed, _ := row["completed_at"].(string)
			if _, err := time.Parse(time.RFC3339Nano, completed); err != nil {
				return ErrInvalidAnimationVerdict
			}
			selected = row
		}
		if selected == nil {
			return ErrInvalidAnimationVerdict
		}
		selected["manual_verdict"] = AnimationManualVerdict{
			Verdict: request.Verdict, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedBy: actor,
		}
		return nil
	})
}
