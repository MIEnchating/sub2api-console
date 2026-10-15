package modelrequest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// ReadBody collects a bounded reply and releases an SSE connection on its
// explicit terminal event. It does not decide business success: failed and
// incomplete events are returned intact for the caller's validation/redaction.
func ReadBody(body io.Reader, stream bool, limit int64) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: limit + 1}
	reader := bufio.NewReader(limited)
	var raw bytes.Buffer
	if !stream {
		return readRemaining(reader, &raw, limited)
	}
	for {
		first, err := reader.Peek(1)
		if err != nil {
			return raw.Bytes(), err
		}
		if !bytes.ContainsRune([]byte(" \r\n\t"), rune(first[0])) {
			break
		}
		value, _ := reader.ReadByte()
		raw.WriteByte(value)
		if limited.N <= 0 {
			return nil, errors.New("上游响应过大")
		}
	}
	first, _ := reader.Peek(1)
	if first[0] == '{' || first[0] == '[' {
		return readRemaining(reader, &raw, limited)
	}
	var data []byte
	eventType := ""
	for {
		line, err := reader.ReadString('\n')
		raw.WriteString(line)
		if limited.N <= 0 {
			return nil, errors.New("上游响应过大")
		}
		trimmed := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if strings.HasPrefix(trimmed, "event:") {
			eventType = strings.TrimSpace(trimmed[6:])
		}
		if strings.HasPrefix(trimmed, "data:") {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, strings.TrimSpace(trimmed[5:])...)
			if string(data) == "[DONE]" {
				return readBuffered(reader, &raw)
			}
			var event map[string]any
			if json.Unmarshal(data, &event) == nil {
				if event["type"] == nil && eventType != "" {
					event["type"] = eventType
				}
				if terminalEvent(event) {
					return readBuffered(reader, &raw)
				}
				data = nil
			}
		}
		if trimmed == "" {
			data = nil
			eventType = ""
		}
		if err == io.EOF {
			return raw.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// Keep events already received with the terminal event, including late errors.
// Read only the buffered bytes so an open connection cannot delay completion.
func readBuffered(reader *bufio.Reader, raw *bytes.Buffer) ([]byte, error) {
	_, err := io.CopyN(raw, reader, int64(reader.Buffered()))
	return raw.Bytes(), err
}

func readRemaining(reader io.Reader, raw *bytes.Buffer, limited *io.LimitedReader) ([]byte, error) {
	_, err := io.Copy(raw, reader)
	if limited.N <= 0 {
		return nil, errors.New("上游响应过大")
	}
	return raw.Bytes(), err
}

func terminalEvent(event map[string]any) bool {
	kind, _ := event["type"].(string)
	switch kind {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "message_stop", "error":
		return true
	}
	if event["error"] != nil {
		return true
	}
	if candidates, ok := event["candidates"].([]any); ok && len(candidates) > 0 {
		candidate, _ := candidates[0].(map[string]any)
		finish, _ := candidate["finishReason"].(string)
		return finish != "" && finish != "FINISH_REASON_UNSPECIFIED"
	}
	return false
}
