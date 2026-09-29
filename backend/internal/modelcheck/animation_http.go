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

func sendAnimation(ctx context.Context, client *http.Client, credential directCredential, model, requestID string, usage *AnimationUsage, onFirstOutput func()) (string, string, error) {
	path := "/v1/responses"
	body := map[string]any{"model": model, "input": animationPrompt, "max_output_tokens": 16384, "stream": true, "reasoning": map[string]string{"effort": "low"}}
	if credential.Platform == "anthropic" {
		path = "/v1/messages"
		body = map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": animationPrompt}}, "max_tokens": 16384, "stream": true, "output_config": map[string]string{"effort": "low"}}
	}
	text, responseModel, status, raw, retryAfter, err := animationHTTP(ctx, client, credential, path, body, requestID, usage, onFirstOutput)
	if err != nil {
		return "", "", animationVisibleError(err, credential.Secret)
	}
	if credential.Platform != "anthropic" && responsesEndpointUnsupported(status, raw) {
		path = "/v1/chat/completions"
		body = map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": animationPrompt}}, "max_tokens": 16384, "stream": true}
		body["reasoning_effort"] = "low"
		body["stream_options"] = map[string]bool{"include_usage": true}
		text, responseModel, status, raw, retryAfter, err = animationHTTP(ctx, client, credential, path, body, requestID, usage, onFirstOutput)
		if err != nil {
			return "", "", animationVisibleError(err, credential.Secret)
		}
	}
	if status < 200 || status >= 300 {
		return "", "", animationRetryableStatusError(status, retryAfter, raw, directStatusError(status, raw, credential.Secret))
	}
	if strings.TrimSpace(text) == "" {
		return "", "", errors.New("上游没有返回动画文本，请检查模型能力后重试")
	}
	return text, responseModel, nil
}

func animationHTTP(ctx context.Context, client *http.Client, credential directCredential, path string, body map[string]any, requestID string, usage *AnimationUsage, onFirstOutput func()) (string, string, int, []byte, http.Header, error) {
	endpoint, err := directEndpoint(credential.BaseURL, path)
	if err != nil {
		return "", "", 0, nil, nil, errors.New("账号 Base URL 无效")
	}
	raw, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", "", 0, nil, nil, errors.New("动画检测请求创建失败")
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
		return "", "", 0, nil, nil, animationRetryableReadError(animationReadError(err))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, err = io.ReadAll(io.LimitReader(response.Body, maximumDirectResponseBytes+1))
		if err != nil {
			return "", "", response.StatusCode, nil, response.Header, animationRetryableStatusError(response.StatusCode, response.Header, raw, animationReadError(err))
		}
		if len(raw) > maximumDirectResponseBytes {
			return "", "", response.StatusCode, nil, response.Header, errors.New("动画检测响应过大，请更换模型后重试")
		}
		return "", "", response.StatusCode, raw, response.Header, nil
	}
	reader := bufio.NewReader(response.Body)
	jsonResponse, err := generationResponseIsJSON(reader)
	if err != nil {
		return "", "", response.StatusCode, nil, response.Header, animationRetryableReadError(animationReadError(err))
	}
	var text, model string
	if !jsonResponse {
		text, model, err = animationStream(reader, path, usage, onFirstOutput)
	} else {
		raw, err = io.ReadAll(io.LimitReader(reader, maximumDirectResponseBytes+1))
		if err != nil {
			return "", "", response.StatusCode, nil, response.Header, animationRetryableReadError(animationReadError(err))
		}
		if len(raw) > maximumDirectResponseBytes {
			return "", "", response.StatusCode, nil, response.Header, errors.New("动画检测响应过大，请更换模型后重试")
		}
		var payload map[string]any
		if json.Unmarshal(raw, &payload) != nil {
			err = errors.New("上游返回的不是有效 JSON 或事件流")
		} else {
			usage.read(payload)
			text, model, err = animationPayload(payload, path)
			if err == nil && strings.TrimSpace(text) != "" && onFirstOutput != nil {
				onFirstOutput()
			}
		}
	}
	return text, model, response.StatusCode, nil, response.Header, animationRetryableReadError(err)
}

func animationPayload(payload map[string]any, path string) (string, string, error) {
	if payload["error"] != nil || payload["success"] == false || stringField(payload, "type") == "error" || stringField(payload, "status") == "failed" {
		return "", "", animationPayloadError(payload, "上游报告生成失败")
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

func animationStream(reader io.Reader, path string, usage *AnimationUsage, onFirstOutput ...func()) (string, string, error) {
	var firstOutput func()
	if len(onFirstOutput) > 0 {
		firstOutput = onFirstOutput[0]
	}
	limited := &io.LimitedReader{R: reader, N: maximumDirectResponseBytes + 1}
	buffered := bufio.NewReader(limited)
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
			if path != "/v1/chat/completions" {
				return errors.New("动画事件流缺少完成事件，请重试")
			}
			completed = true
			return nil
		}
		var event map[string]any
		if json.Unmarshal([]byte(value), &event) != nil {
			return errors.New("上游返回了无效的动画事件流")
		}
		usage.read(event)
		kind := stringField(event, "type")
		if event["error"] != nil || kind == "error" || kind == "response.failed" || kind == "response.incomplete" {
			return animationPayloadError(event, "上游报告生成失败或内容截断")
		}
		if m := stringField(event, "model"); m != "" {
			model = m
		}
		switch path {
		case "/v1/responses":
			if kind == "response.output_text.delta" {
				if delta := stringFieldRaw(event, "delta"); delta != "" {
					output.WriteString(delta)
					if firstOutput != nil {
						firstOutput()
					}
				}
			}
			if response, ok := event["response"].(map[string]any); ok {
				usage.read(response)
				if m := stringField(response, "model"); m != "" {
					model = m
				}
				if kind == "response.completed" || kind == "response.done" {
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
				usage.read(message)
				model = stringField(message, "model")
			}
			delta, _ := event["delta"].(map[string]any)
			if stringField(delta, "stop_reason") == "max_tokens" {
				return errors.New("动画生成被截断，请更换模型后重试")
			}
			if kind == "content_block_delta" && stringField(delta, "type") == "text_delta" {
				if text := stringFieldRaw(delta, "text"); text != "" {
					output.WriteString(text)
					if firstOutput != nil {
						firstOutput()
					}
				}
			}
			if kind == "message_stop" {
				completed = true
			}
		default:
			choices, _ := event["choices"].([]any)
			if len(choices) > 0 {
				choice, _ := choices[0].(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				if content := stringFieldRaw(delta, "content"); content != "" {
					output.WriteString(content)
					if firstOutput != nil {
						firstOutput()
					}
				}
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
	for {
		line, readErr := buffered.ReadString('\n')
		if len(line) == 0 && readErr != nil {
			if readErr == io.EOF {
				break
			}
			return "", "", animationReadError(readErr)
		}
		if limited.N <= 0 {
			return "", "", errors.New("动画检测响应过大，请更换模型后重试")
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			if err := process(); err != nil {
				return "", "", err
			}
			if completed && buffered.Buffered() == 0 {
				return output.String(), model, nil
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			value := strings.Join(data, "\n")
			if path != "/v1/chat/completions" && (value == "[DONE]" || json.Valid([]byte(value))) {
				if err := process(); err != nil {
					return "", "", err
				}
				if completed {
					return output.String(), model, nil
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return "", "", animationReadError(readErr)
		}
	}
	if err := process(); err != nil {
		return "", "", err
	}
	if !completed {
		return "", "", retryableAnimationError{err: errors.New("动画事件流提前中断，请重试")}
	}
	return output.String(), model, nil
}

func stringFieldRaw(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}
