package modelcheck

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func sendAnimation(ctx context.Context, client *http.Client, credential directCredential, model, requestID string) (string, string, error) {
	path := "/v1/responses"
	body := map[string]any{"model": model, "input": animationPrompt, "max_output_tokens": 8192, "stream": true}
	if credential.Platform == "anthropic" {
		path = "/v1/messages"
		body = map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": animationPrompt}}, "max_tokens": 8192, "stream": true}
	}
	text, responseModel, status, raw, err := animationHTTP(ctx, client, credential, path, body, requestID)
	if err != nil {
		return "", "", err
	}
	if credential.Platform != "anthropic" && responsesEndpointUnsupported(status, raw) {
		path = "/v1/chat/completions"
		body = map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": animationPrompt}}, "max_tokens": 8192, "stream": true}
		text, responseModel, status, raw, err = animationHTTP(ctx, client, credential, path, body, requestID)
		if err != nil {
			return "", "", err
		}
	}
	if status < 200 || status >= 300 {
		return "", "", directStatusError(status, raw, credential.Secret)
	}
	if strings.TrimSpace(text) == "" {
		return "", "", errors.New("上游没有返回动画文本，请检查模型能力后重试")
	}
	return text, responseModel, nil
}

func animationHTTP(ctx context.Context, client *http.Client, credential directCredential, path string, body map[string]any, requestID string) (string, string, int, []byte, error) {
	endpoint, err := directEndpoint(credential.BaseURL, path)
	if err != nil {
		return "", "", 0, nil, errors.New("账号 Base URL 无效")
	}
	raw, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", "", 0, nil, errors.New("动画检测请求创建失败")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream, application/json")
	request.Header.Set("X-Request-ID", requestID)
	if credential.Platform == "anthropic" {
		request.Header.Set("x-api-key", credential.Secret)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else {
		request.Header.Set("Authorization", "Bearer "+credential.Secret)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", "", 0, nil, safeTransportError(err)
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(response.Body, maximumDirectResponseBytes+1))
	if err != nil {
		return "", "", response.StatusCode, nil, safeTransportError(err)
	}
	if len(raw) > maximumDirectResponseBytes {
		return "", "", response.StatusCode, nil, errors.New("动画检测响应过大，请更换模型后重试")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", response.StatusCode, raw, nil
	}
	var text, model string
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") || bytes.HasPrefix(bytes.TrimSpace(raw), []byte("data:")) || bytes.HasPrefix(bytes.TrimSpace(raw), []byte("event:")) {
		text, model, err = animationStream(raw, path)
	} else {
		var payload map[string]any
		if json.Unmarshal(raw, &payload) != nil {
			err = errors.New("上游返回的不是有效 JSON 或事件流")
		} else {
			text, model, err = animationPayload(payload, path)
		}
	}
	return text, model, response.StatusCode, nil, err
}

func animationPayload(payload map[string]any, path string) (string, string, error) {
	if payload["error"] != nil || payload["success"] == false || stringField(payload, "type") == "error" || stringField(payload, "status") == "failed" {
		return "", "", errors.New("上游报告生成失败，请检查模型权限、余额或稍后重试")
	}
	if stringField(payload, "status") == "incomplete" || stringField(payload, "stop_reason") == "max_tokens" {
		return "", "", errors.New("动画生成被截断，请更换模型后重试")
	}
	model := stringField(payload, "model")
	if path == "/v1/messages" {
		return anthropicResponseText(payload), model, nil
	}
	if path == "/v1/chat/completions" {
		if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
			choice, _ := choices[0].(map[string]any)
			if reason := stringField(choice, "finish_reason"); reason == "length" || reason == "content_filter" {
				return "", "", errors.New("动画生成被截断或拦截，请更换模型后重试")
			}
		}
		return openAIChatText(payload), model, nil
	}
	return openAIResponseText(payload), model, nil
}

func animationStream(raw []byte, path string) (string, string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), maximumDirectResponseBytes)
	var output strings.Builder
	model := ""
	completed := false
	var data []string
	process := func() error {
		if len(data) == 0 {
			return nil
		}
		value := strings.Join(data, "\n")
		data = nil
		if value == "[DONE]" {
			completed = true
			return nil
		}
		var event map[string]any
		if json.Unmarshal([]byte(value), &event) != nil {
			return errors.New("上游返回了无效的动画事件流")
		}
		kind := stringField(event, "type")
		if event["error"] != nil || kind == "error" || kind == "response.failed" || kind == "response.incomplete" {
			return errors.New("上游报告生成失败或内容截断，请稍后重试")
		}
		if m := stringField(event, "model"); m != "" {
			model = m
		}
		switch path {
		case "/v1/responses":
			if kind == "response.output_text.delta" {
				output.WriteString(stringFieldRaw(event, "delta"))
			}
			if response, ok := event["response"].(map[string]any); ok {
				if m := stringField(response, "model"); m != "" {
					model = m
				}
				if kind == "response.completed" {
					text, _, err := animationPayload(response, path)
					if err != nil {
						return err
					}
					if output.Len() == 0 {
						output.WriteString(text)
					}
					completed = true
				}
			}
		case "/v1/messages":
			if message, ok := event["message"].(map[string]any); ok {
				model = stringField(message, "model")
			}
			delta, _ := event["delta"].(map[string]any)
			if stringField(delta, "stop_reason") == "max_tokens" {
				return errors.New("动画生成被截断，请更换模型后重试")
			}
			if kind == "content_block_delta" && stringField(delta, "type") == "text_delta" {
				output.WriteString(stringFieldRaw(delta, "text"))
			}
			if kind == "message_stop" {
				completed = true
			}
		default:
			choices, _ := event["choices"].([]any)
			if len(choices) > 0 {
				choice, _ := choices[0].(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				output.WriteString(stringFieldRaw(delta, "content"))
				reason := stringField(choice, "finish_reason")
				if reason == "length" || reason == "content_filter" {
					return errors.New("动画生成被截断或拦截，请更换模型后重试")
				}
				if reason == "stop" {
					completed = true
				}
			}
		}
		if output.Len() > 128<<10 {
			return errors.New("生成的动画文本过大，请更换模型后重试")
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := process(); err != nil {
				return "", "", err
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", errors.New("动画事件流读取失败")
	}
	if err := process(); err != nil {
		return "", "", err
	}
	if !completed {
		return "", "", errors.New("动画事件流提前中断，请重试")
	}
	return output.String(), model, nil
}

func stringFieldRaw(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}
