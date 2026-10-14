package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"net/http"
	"strings"
	"testing"
)

func TestPrecheckAcceptsStreamingOnlyEndpoints(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/v1/messages"} {
		t.Run(path, func(t *testing.T) {
			platform := "openai"
			answer := "21"
			if path == "/v1/messages" {
				platform = "anthropic"
				answer = "**21**"
			}
			f := setup(t, 1, platform, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != path {
					w.WriteHeader(404)
					return
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["stream"] != true {
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "streaming is required: this endpoint only accepts stream: true"})
					return
				}
				if body["model"] != "test-model" || !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
					t.Error("missing model or streaming accept header")
				}
				effort := body["reasoning_effort"]
				if path == "/v1/responses" {
					effort = body["reasoning"].(map[string]any)["effort"]
				}
				if path == "/v1/messages" {
					effort = body["output_config"].(map[string]any)["effort"]
				}
				if effort != "medium" {
					t.Errorf("effort=%v", effort)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				switch path {
				case "/v1/responses":
					_, _ = fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"2"}

data: {"type":"response.output_text.delta","delta":"1"}

data: {"type":"response.completed","response":{"status":"completed","model":"returned-model","usage":{"input_tokens":10,"output_tokens":2}}}

`)
				case "/v1/chat/completions":
					_, _ = fmt.Fprint(w, `data: {"model":"returned-model","choices":[{"delta":{"content":"21"},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2}}

data: [DONE]

`)
				default:
					_, _ = fmt.Fprint(w, `data: {"type":"message_start","message":{"model":"returned-model","usage":{"input_tokens":10}}}

data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"**2"}}

data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"1**"}}

data: {"type":"message_delta","usage":{"output_tokens":2}}

data: {"type":"message_stop"}

`)
				}
			})
			input := request("1")
			input.Mode = "precheck"
			if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if row.Precheck.Verdict != "passed" || row.Precheck.Questions[0].Answer != answer || row.ResponseModel != "returned-model" {
				t.Fatalf("unexpected result: %+v, question: %+v", row, row.Precheck.Questions[0])
			}
			if row.Usage.InputTokens == nil || *row.Usage.InputTokens != 10 || row.Usage.OutputTokens == nil || *row.Usage.OutputTokens != 2 {
				t.Fatalf("missing usage: %+v", row.Usage)
			}
		})
	}
}

func TestPrecheckStreamRejectsPartialFailedAndSensitiveAnswers(t *testing.T) {
	for _, scenario := range []string{"partial", "failed", "sensitive answer", "sensitive model", "json"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "json" {
					_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "21", "status": "completed"})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				event := func(value map[string]any) { raw, _ := json.Marshal(value); _, _ = fmt.Fprintf(w, "data: %s\n\n", raw) }
				answer, model := "21", "test-model"
				if scenario == "sensitive answer" {
					answer = fixtureSecret
				}
				if scenario == "sensitive model" {
					model = fixtureSecret
				}
				event(map[string]any{"type": "response.output_text.delta", "delta": answer})
				if scenario == "partial" {
					return
				}
				if scenario == "failed" {
					event(map[string]any{"type": "response.failed", "error": map[string]string{"message": fixtureSecret}})
					return
				}
				event(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "model": model}})
			})
			input := request("1")
			input.Mode = "precheck"
			if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			expected := "error"
			if scenario == "json" || scenario == "sensitive model" {
				expected = "passed"
			}
			if row.Precheck.Verdict != expected {
				t.Fatalf("verdict=%s want %s", row.Precheck.Verdict, expected)
			}
			raw, _ := json.Marshal(row)
			if strings.Contains(string(raw), fixtureSecret) {
				t.Fatal("credential leaked")
			}
			if expected == "error" && row.Precheck.Questions[0].Answer != "" {
				t.Fatal("failed stream published partial answer")
			}
		})
	}
}
