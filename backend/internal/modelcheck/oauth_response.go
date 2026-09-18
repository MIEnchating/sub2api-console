package modelcheck

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

func decodeOAuthResponse(raw []byte) (map[string]any, error) {
	return readOAuthResponse(bytes.NewReader(raw), false)
}

func readOAuthResponse(body io.Reader, completeOnEvent bool) (map[string]any, error) {
	limited := &io.LimitedReader{R: body, N: maximumDirectResponseBytes + 1}
	reader := bufio.NewReader(limited)
	jsonResponse, err := generationResponseIsJSON(reader)
	if err != nil {
		return nil, animationReadError(err)
	}
	if jsonResponse {
		raw, err := io.ReadAll(reader)
		if limited.N <= 0 {
			return nil, visibleRequestError{message: "OAuth 检测响应过大，请稍后重试"}
		}
		if err != nil {
			return nil, animationReadError(err)
		}
		payload, err := decodeOAuthObject(raw)
		if err != nil {
			return nil, err
		}
		if completeOnEvent {
			return payload, validateOAuthAnimationResponse(payload)
		}
		return payload, validateOAuthResponse(payload)
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maximumDirectResponseBytes)
	var data []string
	var terminal map[string]any
	var deltas strings.Builder
	process := func() error {
		if len(data) == 0 {
			return nil
		}
		value := strings.TrimSpace(strings.Join(data, "\n"))
		data = nil
		if value == "" || value == "[DONE]" {
			return nil
		}
		event, err := decodeOAuthObject([]byte(value))
		if err != nil {
			return err
		}
		kind := stringField(event, "type")
		if event["error"] != nil || kind == "error" || kind == "response.failed" || kind == "response.incomplete" {
			if completeOnEvent {
				return animationPayloadError(event, "OAuth 动画检测上游返回失败或不完整结果")
			}
			return visibleRequestError{message: "OAuth 检测上游返回失败或不完整结果，请稍后重试"}
		}
		switch kind {
		case "response.output_text.delta", "output_text.delta":
			delta, _ := event["delta"].(string)
			deltas.WriteString(delta)
		case "response.output_text.done", "output_text.done":
			if deltas.Len() == 0 {
				text, _ := event["text"].(string)
				deltas.WriteString(text)
			}
		case "response.completed", "response.done":
			var ok bool
			terminal, ok = event["response"].(map[string]any)
			if !ok {
				return visibleRequestError{message: "OAuth 检测完成事件缺少结果，请稍后重试"}
			}
			validator := validateOAuthResponse
			if completeOnEvent {
				validator = validateOAuthAnimationResponse
			}
			if err := validator(terminal); err != nil {
				return err
			}
		}
		return nil
	}
	finish := func() (map[string]any, error) {
		if openAIResponseText(terminal) == "" && deltas.Len() > 0 {
			terminal["output_text"] = deltas.String()
		}
		return terminal, nil
	}
	for scanner.Scan() {
		if limited.N <= 0 {
			return nil, visibleRequestError{message: "OAuth 检测响应过大，请稍后重试"}
		}
		line := scanner.Text()
		if strings.HasPrefix(line, ":") {
			continue
		}
		if line == "" {
			if err := process(); err != nil {
				return nil, err
			}
			if completeOnEvent && terminal != nil {
				return finish()
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			// Some compatible Responses streams omit the blank line between events.
			if len(data) > 0 && json.Valid([]byte(strings.Join(data, "\n"))) {
				if err := process(); err != nil {
					return nil, err
				}
				if completeOnEvent && terminal != nil {
					return finish()
				}
			}
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, animationReadError(err)
	}
	if err := process(); err != nil {
		return nil, err
	}
	if terminal == nil {
		if completeOnEvent {
			return nil, retryableAnimationError{err: visibleRequestError{message: "OAuth 检测响应未正常结束，请稍后重试"}}
		}
		return nil, visibleRequestError{message: "OAuth 检测响应未正常结束，请稍后重试"}
	}
	return finish()
}

func decodeOAuthObject(raw []byte) (map[string]any, error) {
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, visibleRequestError{message: "OAuth 检测响应不是有效的 JSON 对象，请稍后重试"}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, visibleRequestError{message: "OAuth 检测响应包含多余内容，请稍后重试"}
	}
	return payload, nil
}

func validateOAuthResponse(payload map[string]any) error {
	status := stringField(payload, "status")
	if payload["error"] != nil || payload["success"] == false || stringField(payload, "type") == "error" || (status != "" && status != "completed") {
		return visibleRequestError{message: "OAuth 检测上游返回失败或不完整结果，请稍后重试"}
	}
	return nil
}

func validateOAuthAnimationResponse(payload map[string]any) error {
	status := stringField(payload, "status")
	if payload["error"] != nil || payload["success"] == false || stringField(payload, "type") == "error" || (status != "" && status != "completed") {
		return animationPayloadError(payload, "OAuth 动画检测上游返回失败或不完整结果")
	}
	return nil
}
