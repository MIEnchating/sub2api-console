package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestGenerationAcceptsJSONAndStreamsAcrossSupportedProtocols(t *testing.T) {
	cases := []struct {
		name, platform, path, body string
		stream                     bool
	}{
		{"responses JSON", "openai", "/v1/responses", fmt.Sprintf(`{"model":"reported-model","status":"completed","output_text":%q}`, fixtureSVG), false},
		{"responses stream", "openai", "/v1/responses", fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"reported-model\"}}\n\n", fixtureSVG), true},
		{"chat fallback", "openai", "/v1/chat/completions", fmt.Sprintf("data: {\"model\":\"reported-model\",\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", fixtureSVG), true},
		{"anthropic JSON", "anthropic", "/v1/messages", fmt.Sprintf(`{"model":"reported-model","content":[{"type":"text","text":%q}]}`, fixtureSVG), false},
		{"anthropic stream", "anthropic", "/v1/messages", fmt.Sprintf("data: {\"type\":\"message_start\",\"message\":{\"model\":\"reported-model\"}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":%q}}\n\ndata: {\"type\":\"message_stop\"}\n\n", fixtureSVG), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, tc.platform, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					w.WriteHeader(404)
					return
				}
				var body struct {
					Model  string `json:"model"`
					Stream bool   `json:"stream"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "test-model" || !body.Stream {
					t.Error("wrong generation request")
				}
				if r.Header.Get("X-Request-ID") == "" {
					t.Error("missing request ID")
				}
				if tc.platform == "anthropic" {
					if r.Header.Get("x-api-key") != fixtureSecret {
						t.Error("missing Anthropic key")
					}
				} else if r.Header.Get("Authorization") != "Bearer "+fixtureSecret {
					t.Error("missing OpenAI key")
				}
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
				}
				_, _ = w.Write([]byte(tc.body))
			})
			queued, err := f.service.EnqueueAnimation(context.Background(), request("1"))
			if err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			if task.ID != queued.ID || task.Status != "succeeded" || task.Progress != 100 {
				t.Fatalf("task: %#v", task)
			}
			results := task.Result["animations"].([]modelcheck.AnimationResult)
			if len(results) != 1 || !strings.Contains(results[0].SVG, "animateTransform") || results[0].ResponseModel != "reported-model" || results[0].CompletedAt == "" {
				t.Fatalf("results: %#v", results)
			}
			encoded, _ := json.Marshal(task)
			if strings.Contains(string(encoded), fixtureSecret) {
				t.Fatal("credential leaked")
			}
			statuses, err := f.service.AccountStatuses(context.Background())
			if err != nil || len(statuses) != 0 {
				t.Fatal("animation must not change behavior verdicts")
			}
		})
	}
}

func TestGenerationRejectsUnsafeOrIncompleteResponses(t *testing.T) {
	cases := []struct {
		name, body string
		stream     bool
	}{
		{"empty text", `{"output_text":""}`, false},
		{"business failure despite text", fmt.Sprintf(`{"error":{"message":"failed"},"output_text":%q}`, fixtureSVG), false},
		{"truncated JSON", fmt.Sprintf(`{"status":"incomplete","output_text":%q}`, fixtureSVG), false},
		{"incomplete stream", fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", fixtureSVG), true},
		{"late stream error", fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: {\"type\":\"error\"}\n\n", fixtureSVG), true},
		{"malformed SVG", `{"output_text":"<svg><circle>"}`, false},
		{"script", `{"output_text":"<svg><script>alert(1)</script></svg>"}`, false},
		{"external image", `{"output_text":"<svg><image href='https://example.invalid/x'/></svg>"}`, false},
		{"event attribute", `{"output_text":"<svg onload='alert(1)'><circle r='2'/></svg>"}`, false},
		{"HTML embedding", `{"output_text":"<svg><foreignObject/></svg>"}`, false},
		{"external CSS", `{"output_text":"<svg><circle style='fill:url(https://example.invalid/x)'/></svg>"}`, false},
		{"animated resource", `{"output_text":"<svg><animate attributeName='href' to='https://example.invalid/x'/></svg>"}`, false},
		{"secret in generated text", fmt.Sprintf(`{"output_text":"<svg><text>%s</text></svg>"}`, fixtureSecret), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
				}
				_, _ = w.Write([]byte(tc.body))
			})
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["animations"].([]modelcheck.AnimationResult)
			if task.Status != "failed" || rows[0].SVG != "" || rows[0].Error == "" {
				t.Fatalf("unsafe response accepted: %#v", task)
			}
			raw, _ := json.Marshal(task)
			if strings.Contains(string(raw), fixtureSecret) {
				t.Fatal("credential leaked")
			}
		})
	}
}

func TestBatchReportsPartialSuccessAndRedactsHTTPFailure(t *testing.T) {
	f := setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] == "denied-model" {
			w.WriteHeader(403)
			_, _ = fmt.Fprintf(w, `{"error":{"message":"invalid key %s"}}`, fixtureSecret)
			return
		}
		_, _ = fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
	})
	input := request("1", "2")
	input.Targets[1].Model = "denied-model"
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	if task.Status != "partial" || task.Result["completed"] != 2 {
		t.Fatalf("task: %#v", task)
	}
	raw, _ := json.Marshal(task)
	if strings.Contains(string(raw), fixtureSecret) {
		t.Fatal("secret in failed result")
	}
}

func TestInvalidTargetsAreRejectedBeforeCreatingTasks(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid target reached upstream") })
	for _, input := range []modelcheck.AnimationRequest{{}, request("0"), request("1", "1"), request("99"), {Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: " "}}}, {Targets: request("1").Targets, TimeoutSeconds: 121}} {
		if _, err := f.service.EnqueueAnimation(context.Background(), input); err == nil {
			t.Fatalf("accepted invalid request: %#v", input)
		}
	}
	history, err := f.service.AnimationHistory(context.Background())
	if err != nil || len(history) != 0 {
		t.Fatal("invalid requests created tasks")
	}
}

func TestManualPriorityAccountCanGenerateAnimation(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": fixtureSVG})
	})
	priority := int64(3)
	f.catalog.rows[0].ManualPriority = &priority
	f.catalog.details["1"].ManualPriority = &priority
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	if task := finished(t, f); task.Status != "succeeded" {
		t.Fatalf("manual priority animation failed: %#v", task)
	}
	if *f.catalog.details["1"].ManualPriority != 3 {
		t.Fatal("detection changed manual priority")
	}
}
