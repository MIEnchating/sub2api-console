package modelrequest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
)

func TestStreamBodyRetainsBufferedFailureAfterTerminal(t *testing.T) {
	for _, terminal := range []string{`[DONE]`, `{"type":"response.completed","response":{"status":"completed"}}`} {
		t.Run(terminal, func(t *testing.T) {
			body := "data: " + terminal + "\n\ndata: {\"type\":\"error\",\"error\":{\"message\":\"generation failed\"}}\n\n"
			raw, err := modelrequest.ReadBody(strings.NewReader(body), true, 1024)
			if err != nil || string(raw) != body {
				t.Fatalf("buffered upstream failure was discarded: raw=%s err=%v", raw, err)
			}
		})
	}
}

type terminalResponseReader struct{ remaining string }

func (r *terminalResponseReader) Read(p []byte) (int, error) {
	if r.remaining == "" {
		return 0, errors.New("must not wait for connection EOF after terminal event")
	}
	n := copy(p, r.remaining)
	r.remaining = r.remaining[n:]
	return n, nil
}

func TestStreamBodyReturnsCompletedReplyWithoutAnotherNetworkRead(t *testing.T) {
	body := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"hello\"}}\n\n"
	raw, err := modelrequest.ReadBody(&terminalResponseReader{remaining: body}, true, 1024)
	if err != nil || !strings.Contains(string(raw), "hello") {
		t.Fatalf("completed stream did not return promptly: raw=%s err=%v", raw, err)
	}
}

func TestStreamBodyReturnsTerminalEventAndKeepsCompleteReply(t *testing.T) {
	for _, terminal := range []string{
		`{"type":"response.completed","response":{"status":"completed"}}`,
		`{"type":"message_stop"}`,
		`{"candidates":[{"finishReason":"STOP"}]}`,
		`[DONE]`,
	} {
		t.Run(terminal, func(t *testing.T) {
			body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: " + terminal + "\n\n"
			raw, err := modelrequest.ReadBody(strings.NewReader(body), true, 1024)
			if err != nil || !strings.Contains(string(raw), terminal) || !strings.Contains(string(raw), "hello") {
				t.Fatalf("raw=%s err=%v", raw, err)
			}
		})
	}
}

func TestNonstreamBodyRetainsExactJSONAndRejectsOversizeReply(t *testing.T) {
	input := "{\n\"output_text\":\"reply\",\"value\":9007199254740993\n}"
	raw, err := modelrequest.ReadBody(strings.NewReader(input), false, 1024)
	if err != nil || string(raw) != input {
		t.Fatalf("raw=%s err=%v", raw, err)
	}
	if _, err := modelrequest.ReadBody(strings.NewReader(input), false, 10); err == nil {
		t.Fatal("oversized response accepted")
	}
}
