package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamerror"
)

type attemptOutcome struct {
	responseText         string
	ignoredPlaceholder   bool
	placeholderText      string
	placeholderPreview   string
	placeholderTruncated bool
	firstTokenAt         time.Time
	receivedContent      bool
	failureCode          string
	status               *int
	content              bool
	reason               string
	model                string
	unavailable          bool
}

func probeAttempt(ctx context.Context, account *adminclient.AccountProbe, target Target, config Config, protocol string) attemptOutcome {
	requestContext, cancel := modelrequest.Start(ctx, config.Timeout, true)
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
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return attemptOutcome{status: &status, reason: limitedText(account.Redact(failure(status, string(body))))}
	}
	result := readProbeResponse(response.Body, response.Header.Get("Content-Type"), protocol)
	if result.status != nil {
		status = *result.status
	} else if upstreamStatus, found := upstreamStatusFromError(result.reason); found {
		status = upstreamStatus
	} else if result.reason != "" && upstreamerror.IsCapacity(result.reason) {
		status = http.StatusServiceUnavailable
	}
	result.status = &status
	if requestContext.Err() != nil && !result.content {
		result.reason = "主动探测超时"
	}
	result.responseText = account.Redact(result.responseText)
	if text := []rune(result.responseText); len(text) > 2000 {
		result.responseText = string(text[:2000]) + "…（内容已截断）"
	}
	result.reason = limitedText(account.Redact(result.reason))
	result.model = limitedText(account.Redact(result.model))
	if result.ignoredPlaceholder {
		preview := account.Redact(result.placeholderText)
		if result.placeholderTruncated {
			preview += "…"
		}
		result.placeholderPreview = strconv.QuoteToGraphic(preview)
		result.placeholderText = ""
	}
	return result
}

func probeReadError(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return "主动探测超时"
	}
	return "上游连接失败或响应中断，请检查接口地址和网络后重试"
}

func readProbeResponse(body io.Reader, contentType, protocol string) attemptOutcome {
	limited := &io.LimitedReader{R: body, N: (4 << 20) + 1}
	reader := bufio.NewReader(limited)
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
		if limited.N <= 0 {
			return attemptOutcome{reason: "上游探活响应过大，请检查模型后重试"}
		}
		return finishProbeResponse(probeEventOutcome(payload))
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	result := attemptOutcome{}
	var output modelrequest.TextAccumulator
	completed, chatStopped := false, false
	eventType := ""
	var data []string
	fail := func(reason string) (attemptOutcome, bool) {
		result.reason = reason
		return finishProbeResponse(result), true
	}
	consume := func(streamed bool) (attemptOutcome, bool) {
		if len(data) == 0 {
			eventType = ""
			return result, false
		}
		raw := strings.Join(data, "\n")
		data = nil
		if strings.TrimSpace(raw) == "[DONE]" {
			if protocol != "chat_completions" {
				return fail("上游探活流未完整结束，缺少协议完成事件，请重试")
			}
			completed = true
			return finishProbeResponse(result), true
		}
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var payload any
		if err := decoder.Decode(&payload); err != nil {
			return fail("上游探活流返回无效 JSON")
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return fail("上游探活流包含无效尾随数据")
		}
		if object, ok := payload.(map[string]any); ok && object["type"] == nil && eventType != "" {
			object["type"] = eventType
		}
		eventType = ""
		output.Add(payload)
		result.responseText = output.String()
		event := probeEventOutcome(payload)
		if !result.receivedContent && event.ignoredPlaceholder {
			result.ignoredPlaceholder = true
			appendPlaceholderText(&result, event.placeholderText)
			result.placeholderTruncated = result.placeholderTruncated || event.placeholderTruncated
		}
		if event.model != "" {
			result.model = event.model
		}
		if event.receivedContent {
			if streamed && !result.receivedContent {
				result.firstTokenAt = time.Now()
			}
			result.receivedContent, result.content = true, true
		}
		if event.reason != "" {
			result.status = event.status
			return fail(event.reason)
		}
		if !streamed {
			// Some upstreams ignore stream=true and send one complete JSON body.
			result.firstTokenAt = time.Time{}
			completed = true
			return result, false
		}
		done, stopped := probeStreamCompletion(payload, protocol)
		completed = completed || done
		chatStopped = chatStopped || stopped
		if completed {
			return finishProbeResponse(result), true
		}
		return result, false
	}
	for scanner.Scan() {
		if limited.N <= 0 {
			result, _ = fail("上游探活响应过大，请检查模型后重试")
			return result
		}
		line := scanner.Text()
		if line == "" {
			event, done := consume(true)
			if done {
				return event
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			data = append(data, line)
			event, done := consume(false)
			if done {
				return finishProbeResponse(event)
			}
		}
	}
	if scanner.Err() != nil || limited.N <= 0 {
		result, _ = fail("上游探活流读取中断或响应过大，请稍后重试")
		return result
	}
	if event, done := consume(true); done {
		return event
	}
	if !completed && !chatStopped {
		result, _ = fail("上游探活流未完整结束，缺少协议完成事件，请重试")
		return result
	}
	return finishProbeResponse(result)
}

func finishProbeResponse(result attemptOutcome) attemptOutcome {
	if !result.content && result.reason == "" {
		result.reason = "上游探活流未返回有效文本，请检查模型和接口配置"
		if result.ignoredPlaceholder {
			result.reason = "上游探活仅返回占位字符，未收到有效正文，请检查上游响应后重试"
		}
	}
	if result.reason != "" {
		result.content = false
	}
	return result
}

func probeEventOutcome(value any) attemptOutcome {
	reason := eventError(value)
	if reason == "" {
		reason = probeTerminationError(value)
	}
	content := eventHasContent(value)
	placeholder := eventPlaceholderText(value)
	result := attemptOutcome{responseText: modelrequest.Text(value), ignoredPlaceholder: placeholder != "", model: eventModel(value), reason: reason, receivedContent: content, content: reason == "" && content}
	appendPlaceholderText(&result, placeholder)
	if reason != "" {
		result.status = eventErrorStatus(value)
	}
	return result
}

func eventPlaceholderText(value any) string {
	var text string
	eventHasText(value, func(candidate any) bool {
		if !placeholderText(candidate) {
			return false
		}
		text, _ = candidate.(string)
		return true
	})
	return text
}

func appendPlaceholderText(result *attemptOutcome, text string) {
	const limit = 80
	remaining := limit - utf8.RuneCountInString(result.placeholderText)
	for _, character := range text {
		if remaining == 0 {
			result.placeholderTruncated = true
			return
		}
		result.placeholderText += string(character)
		remaining--
	}
}

func eventErrorStatus(value any) *int {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"error", "response"} {
		if status := eventErrorStatus(object[key]); status != nil {
			return status
		}
	}
	for _, key := range []string{"status_code", "status", "code"} {
		status, err := strconv.Atoi(fmt.Sprint(object[key]))
		if err == nil && status >= 400 && status <= 599 {
			return &status
		}
	}
	return nil
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
	return eventHasText(value, meaningfulText)
}

func eventHasText(value any, matches func(any) bool) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if response, ok := object["response"]; ok && eventHasText(response, matches) {
		return true
	}
	if matches(object["output_text"]) {
		return true
	}
	switch object["type"] {
	case "response.output_text.delta":
		return matches(object["delta"])
	case "response.output_text.done":
		return matches(object["text"])
	case "content_block_delta":
		delta, _ := object["delta"].(map[string]any)
		return delta["type"] == "text_delta" && matches(delta["text"])
	case "content_block_start":
		block, _ := object["content_block"].(map[string]any)
		return block["type"] == "text" && matches(block["text"])
	}
	if choices, ok := object["choices"].([]any); ok {
		for _, value := range choices {
			choice, _ := value.(map[string]any)
			for _, key := range []string{"delta", "message"} {
				message, _ := choice[key].(map[string]any)
				if matches(message["content"]) {
					return true
				}
			}
		}
	}
	if content, ok := object["content"].([]any); ok && hasTextBlocks(content, matches) {
		return true
	}
	if output, ok := object["output"].([]any); ok {
		for _, value := range output {
			item, _ := value.(map[string]any)
			if item["type"] != "message" {
				continue
			}
			if blocks, ok := item["content"].([]any); ok && hasTextBlocks(blocks, matches) {
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
				if part["thought"] != true && matches(part["text"]) {
					return true
				}
			}
		}
	}
	return false
}

func hasTextBlocks(blocks []any, matches func(any) bool) bool {
	for _, value := range blocks {
		block, _ := value.(map[string]any)
		if (block["type"] == "text" || block["type"] == "output_text") && matches(block["text"]) {
			return true
		}
	}
	return false
}

// Only Han text, English letters, ASCII digits and emoji establish first text.
// Empty protocol deltas are metadata; nonempty whitespace is a placeholder.
func meaningfulText(value any) bool {
	text, ok := value.(string)
	return ok && modelrequest.MeaningfulText(text)
}

func placeholderText(value any) bool {
	text, ok := value.(string)
	return ok && text != "" && !meaningfulText(text)
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
			if nonemptyText(detail["message"]) && detail["code"] == nil && detail["type"] == nil {
				return fmt.Sprint(detail["message"])
			}
			if len(detail) > 0 {
				encoded, _ := json.Marshal(detail)
				return string(encoded)
			}
			return "上游返回错误（未提供错误详情）"
		}
		return fmt.Sprint(raw)
	}
	if object["success"] == false {
		if nonemptyText(object["message"]) {
			return fmt.Sprint(object["message"])
		}
		encoded, _ := json.Marshal(object)
		return string(encoded)
	}
	for _, key := range []string{"response", "message"} {
		if nested, ok := object[key].(map[string]any); ok {
			if reason := eventError(nested); reason != "" {
				return reason
			}
		}
	}
	eventType, _ := object["type"].(string)
	status, _ := object["status"].(string)
	outputBudgetReached := probeOutputBudgetReached(object)
	if eventType == "response.incomplete" {
		response, _ := object["response"].(map[string]any)
		outputBudgetReached = probeOutputBudgetReached(response)
	}
	incomplete := (eventType == "response.incomplete" || status == "incomplete") && !outputBudgetReached
	if strings.Contains(eventType, "error") || strings.Contains(eventType, "failed") || incomplete || status == "failed" || status == "cancelled" {
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
