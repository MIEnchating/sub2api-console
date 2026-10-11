package probe

import "fmt"

// A text delta proves only that generation started. Availability also requires
// a terminal event, including an explicit output budget stop. Chat keeps reading
// past finish_reason for usage and errors until [DONE] or a clean EOF.
func probeStreamCompletion(value any, protocol string) (completed, chatStopped bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return false, false
	}
	switch protocol {
	case "responses":
		response, _ := object["response"].(map[string]any)
		return object["type"] == "response.completed" ||
			(object["type"] == "response.incomplete" && probeOutputBudgetReached(response)), false
	case "anthropic":
		return object["type"] == "message_stop", false
	case "chat_completions":
		choices, _ := object["choices"].([]any)
		return false, probeAllFinished(choices, "finish_reason")
	case "gemini":
		candidates, _ := object["candidates"].([]any)
		return probeAllFinished(candidates, "finishReason"), false
	}
	return false, false
}

func probeAllFinished(items []any, key string) bool {
	if len(items) == 0 {
		return false
	}
	for _, value := range items {
		item, _ := value.(map[string]any)
		if !nonemptyText(item[key]) {
			return false
		}
	}
	return true
}

func probeTerminationError(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"response", "message", "delta"} {
		if reason := probeTerminationError(object[key]); reason != "" {
			return reason
		}
	}
	if reason, _ := object["stop_reason"].(string); reason != "" && reason != "end_turn" && reason != "stop_sequence" && reason != "tool_use" && reason != "max_tokens" {
		return fmt.Sprintf("上游探活生成未完整结束（%s），请检查模型或输出限制", reason)
	}
	choices, _ := object["choices"].([]any)
	for _, value := range choices {
		choice, _ := value.(map[string]any)
		if reason, _ := choice["finish_reason"].(string); reason != "" && reason != "stop" && reason != "tool_calls" && reason != "function_call" && reason != "length" {
			return fmt.Sprintf("上游探活生成被截断或拦截（%s），请检查模型或输出限制", reason)
		}
	}
	candidates, _ := object["candidates"].([]any)
	for _, value := range candidates {
		candidate, _ := value.(map[string]any)
		if reason, _ := candidate["finishReason"].(string); reason != "" && reason != "STOP" && reason != "MAX_TOKENS" {
			return fmt.Sprintf("上游探活生成未成功完成（%s），请检查模型或输出限制", reason)
		}
	}
	return ""
}

// Only an explicit output budget stop is acceptable for an availability probe.
// Content filters, unknown incomplete reasons and actual errors remain failures;
// the response reader separately requires visible text and a terminal event.
func probeOutputBudgetReached(object map[string]any) bool {
	if object["status"] != "incomplete" {
		return false
	}
	details, _ := object["incomplete_details"].(map[string]any)
	return details["reason"] == "max_output_tokens"
}
