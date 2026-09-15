package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

type attemptOutcome struct {
	measuredFirstToken bool
	failureCode        string
	status             *int
	content            bool
	reason             string
	model              string
	unavailable        bool
}

func probeAttempt(ctx context.Context, account *adminclient.AccountProbe, target Target, config Config) attemptOutcome {
	requestContext, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	response, err := account.Open(requestContext, *target.Model, config.Prompt)
	if err != nil {
		var unavailable *adminclient.ProbeUnavailableError
		if errors.As(err, &unavailable) {
			return attemptOutcome{reason: err.Error(), unavailable: true, failureCode: unavailable.Code}
		}
		return attemptOutcome{reason: probeReadError(requestContext, err)}
	}
	defer response.Body.Close()
	status := response.StatusCode
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 500))
		return attemptOutcome{status: &status, reason: failure(status, string(body))}
	}
	result := readProbeResponse(response.Body, response.Header.Get("Content-Type"))
	if upstreamStatus, found := upstreamStatusFromError(result.reason); found {
		status = upstreamStatus
	}
	result.status = &status
	if requestContext.Err() != nil && !result.content {
		result.reason = "主动探测超时"
	}
	result.reason = limitedText(account.Redact(result.reason))
	result.model = limitedText(account.Redact(result.model))
	return result
}

func probeReadError(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return "主动探测超时"
	}
	return "上游连接失败或响应中断，请检查接口地址和网络后重试"
}

func readProbeResponse(body io.Reader, contentType string) attemptOutcome {
	reader := bufio.NewReader(io.LimitReader(body, (4<<20)+1))
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		decoder := json.NewDecoder(reader)
		decoder.UseNumber()
		var payload any
		if err := decoder.Decode(&payload); err != nil {
			return attemptOutcome{reason: "上游探活响应不是有效 JSON，请检查接口配置"}
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return attemptOutcome{reason: "上游探活响应包含无效尾随数据"}
		}
		return finishProbeResponse(probeEventOutcome(payload))
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	result := attemptOutcome{}
	var data []string
	consume := func() (attemptOutcome, bool) {
		if len(data) == 0 {
			return result, false
		}
		raw := strings.Join(data, "\n")
		data = nil
		if strings.TrimSpace(raw) == "[DONE]" {
			return result, true
		}
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var payload any
		if err := decoder.Decode(&payload); err != nil {
			return attemptOutcome{reason: "上游探活流返回无效 JSON", model: result.model}, true
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return attemptOutcome{reason: "上游探活流包含无效尾随数据", model: result.model}, true
		}
		event := probeEventOutcome(payload)
		if event.model != "" {
			result.model = event.model
		}
		if event.reason != "" || event.content {
			event.model = result.model
			return event, true
		}
		return result, false
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			event, done := consume()
			if done {
				return finishProbeStream(event)
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			data = append(data, line)
			event, done := consume()
			if done {
				return finishProbeResponse(event)
			}
		}
	}
	if scanner.Err() != nil {
		return attemptOutcome{reason: "上游探活流读取中断或响应过大，请稍后重试", model: result.model}
	}
	if event, done := consume(); done {
		return finishProbeStream(event)
	}
	return finishProbeResponse(result)
}

func finishProbeStream(result attemptOutcome) attemptOutcome {
	result.measuredFirstToken = result.content
	return finishProbeResponse(result)
}

func finishProbeResponse(result attemptOutcome) attemptOutcome {
	if !result.content && result.reason == "" {
		result.reason = "上游探活流未返回有效文本，请检查模型和接口配置"
	}
	return result
}

func probeEventOutcome(value any) attemptOutcome {
	reason := eventError(value)
	return attemptOutcome{model: eventModel(value), reason: reason, content: reason == "" && eventHasContent(value)}
}

func eventModel(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"model", "modelVersion"} {
		if model, ok := object[key].(string); ok && strings.TrimSpace(model) != "" {
			return model
		}
	}
	for _, key := range []string{"response", "message"} {
		if model := eventModel(object[key]); model != "" {
			return model
		}
	}
	return ""
}

func eventHasContent(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	switch object["type"] {
	case "response.output_text.delta":
		return nonemptyText(object["delta"])
	case "response.output_text.done":
		return nonemptyText(object["text"])
	case "content_block_delta":
		delta, _ := object["delta"].(map[string]any)
		return delta["type"] == "text_delta" && nonemptyText(delta["text"])
	case "content_block_start":
		block, _ := object["content_block"].(map[string]any)
		return block["type"] == "text" && nonemptyText(block["text"])
	}
	if choices, ok := object["choices"].([]any); ok {
		for _, value := range choices {
			choice, _ := value.(map[string]any)
			for _, key := range []string{"delta", "message"} {
				message, _ := choice[key].(map[string]any)
				if nonemptyText(message["content"]) {
					return true
				}
			}
		}
	}
	if content, ok := object["content"].([]any); ok && hasTextBlocks(content) {
		return true
	}
	if output, ok := object["output"].([]any); ok {
		for _, value := range output {
			item, _ := value.(map[string]any)
			if item["type"] != "message" {
				continue
			}
			if blocks, ok := item["content"].([]any); ok && hasTextBlocks(blocks) {
				return true
			}
		}
	}
	if candidates, ok := object["candidates"].([]any); ok {
		for _, value := range candidates {
			candidate, _ := value.(map[string]any)
			content, _ := candidate["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, value := range parts {
				part, _ := value.(map[string]any)
				if part["thought"] != true && nonemptyText(part["text"]) {
					return true
				}
			}
		}
	}
	return false
}

func hasTextBlocks(blocks []any) bool {
	for _, value := range blocks {
		block, _ := value.(map[string]any)
		if (block["type"] == "text" || block["type"] == "output_text") && nonemptyText(block["text"]) {
			return true
		}
	}
	return false
}

func nonemptyText(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func eventError(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return "上游探活响应格式无效"
	}
	if raw, present := object["error"]; present && raw != nil && raw != "" {
		if detail, ok := raw.(map[string]any); ok {
			if nonemptyText(detail["message"]) {
				return fmt.Sprint(detail["message"])
			}
			return "上游返回错误，请检查模型权限、余额或稍后重试"
		}
		return fmt.Sprint(raw)
	}
	if object["success"] == false {
		return "上游报告探活失败，请检查模型权限、余额或稍后重试"
	}
	eventType, _ := object["type"].(string)
	status, _ := object["status"].(string)
	if strings.Contains(eventType, "error") || strings.Contains(eventType, "failed") || status == "failed" || status == "cancelled" || status == "incomplete" {
		if nonemptyText(object["message"]) {
			return fmt.Sprint(object["message"])
		}
		if response, ok := object["response"].(map[string]any); ok {
			if reason := eventError(response); reason != "" {
				return reason
			}
		}
		return fmt.Sprintf("上游探活未成功完成（%s）", firstNonempty(eventType, status))
	}
	return ""
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
