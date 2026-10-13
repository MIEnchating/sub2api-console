package modelcheck

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// A missing list preserves legacy single-model requests and saved schedules.
func normalizeAnimationModels(model string, models []string) ([]string, error) {
	if models == nil {
		models = []string{model}
	}
	if len(models) < 1 || len(models) > 16 {
		return nil, errors.New("请选择 1 到 16 个检测模型")
	}
	values := make([]string, 0, len(models))
	seen := map[string]bool{}
	for _, item := range models {
		item = strings.TrimSpace(item)
		if item == "" || utf8.RuneCountInString(item) > 256 || strings.ContainsFunc(item, unicode.IsControl) {
			return nil, errors.New("请输入 1 到 256 个字符的有效模型 ID")
		}
		if seen[item] {
			return nil, errors.New("检测模型不能重复")
		}
		seen[item] = true
		values = append(values, item)
	}
	return values, nil
}

func animationTargetUnits(targets []AnimationTarget) int {
	total := 0
	for _, target := range targets {
		total += max(1, len(target.Models))
	}
	return total
}

// Multi-model tasks need enough time for every model under the existing slot limit.
func animationTaskTimeout(request AnimationRequest, minimum time.Duration) time.Duration {
	var models []string
	for _, target := range request.Targets {
		if len(target.Models) > len(models) {
			models = target.Models
		}
	}
	if len(models) < 2 {
		return minimum
	}
	animation := request.Mode != precheckMode
	value := DetectionTask{Models: models, Animation: &animation, Precheck: request.Mode == precheckMode || request.Mode == combinedMode, TimeoutSeconds: request.TimeoutSeconds, Concurrency: 3}
	return detectionTaskTimeout(value, len(request.Targets), minimum)
}
