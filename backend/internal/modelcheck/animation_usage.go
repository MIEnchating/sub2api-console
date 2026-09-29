package modelcheck

import (
	"encoding/json"
	"math"
)

// Missing usage remains nil; a reported zero is different from an absent value.
type AnimationUsage struct {
	InputTokens     *int64 `json:"input_tokens,omitempty"`
	OutputTokens    *int64 `json:"output_tokens,omitempty"`
	TotalTokens     *int64 `json:"total_tokens,omitempty"`
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`
}

func cloneAnimationUsage(usage *AnimationUsage) *AnimationUsage {
	if usage == nil {
		return nil
	}
	clone := *usage
	clone.InputTokens = cloneAnimationToken(usage.InputTokens)
	clone.OutputTokens = cloneAnimationToken(usage.OutputTokens)
	clone.TotalTokens = cloneAnimationToken(usage.TotalTokens)
	clone.ReasoningTokens = cloneAnimationToken(usage.ReasoningTokens)
	return &clone
}

func cloneAnimationToken(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func (usage *AnimationUsage) read(payload map[string]any) {
	if usage == nil {
		return
	}
	values, _ := payload["usage"].(map[string]any)
	for _, field := range []struct {
		names  []string
		target **int64
	}{
		{[]string{"input_tokens", "prompt_tokens"}, &usage.InputTokens},
		{[]string{"output_tokens", "completion_tokens"}, &usage.OutputTokens},
		{[]string{"total_tokens"}, &usage.TotalTokens},
	} {
		for _, name := range field.names {
			if value, exists := values[name]; exists {
				*field.target = animationTokenCount(value)
				break
			}
		}
	}
	for _, name := range []string{"output_tokens_details", "completion_tokens_details"} {
		details, _ := values[name].(map[string]any)
		if value, exists := details["reasoning_tokens"]; exists {
			usage.ReasoningTokens = animationTokenCount(value)
			break
		}
	}
}

func animationTokenCount(value any) *int64 {
	var number int64
	switch value := value.(type) {
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return nil
		}
		number = parsed
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1<<53-1 || value != math.Trunc(value) {
			return nil
		}
		number = int64(value)
	default:
		return nil
	}
	if number < 0 || number > 1<<53-1 {
		return nil
	}
	return &number
}

func (usage *AnimationUsage) complete() {
	if usage != nil && usage.TotalTokens == nil && usage.InputTokens != nil && usage.OutputTokens != nil {
		total := *usage.InputTokens + *usage.OutputTokens
		if total <= 1<<53-1 {
			usage.TotalTokens = &total
		}
	}
}
