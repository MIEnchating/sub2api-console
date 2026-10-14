package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestGeminiAnimationAndCandyUseNativeStreamAndPreserveUsage(t *testing.T) {
	for _, mode := range []string{"animation", "precheck"} {
		t.Run(mode, func(t *testing.T) {
			output := fixtureSVG
			if mode == "precheck" {
				output = "21"
			}
			f := setup(t, 1, "gemini", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1beta/models/gemini-test:streamGenerateContent" || r.URL.Query().Get("alt") != "sse" || r.Header.Get("x-goog-api-key") != fixtureSecret || r.Header.Get("Authorization") != "" {
					t.Errorf("incorrect Gemini request: %s", r.URL)
				}
				var body struct {
					Contents []struct {
						Role  string
						Parts []struct{ Text string }
					}
					GenerationConfig struct{ MaxOutputTokens int }
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body.Contents) != 1 || body.Contents[0].Role != "user" || len(body.Contents[0].Parts) != 1 || body.Contents[0].Parts[0].Text == "" || body.GenerationConfig.MaxOutputTokens == 0 {
					t.Errorf("invalid native body: %+v", body)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"thought\":true,\"text\":\"private reasoning\"}]}}]}\n\n")
				fmt.Fprintf(w, "data: {\"modelVersion\":\"gemini-actual\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":%q}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":12,\"candidatesTokenCount\":34,\"thoughtsTokenCount\":5,\"totalTokenCount\":51}}\n\n", output)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			})
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			req := request("1")
			req.Mode, req.Targets[0].Model = mode, "gemini-test"
			if _, err := f.service.EnqueueAnimation(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			runner.runs[0](ctx)
			task := finished(t, f)
			row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
			if task.Status != "succeeded" || row.ResponseModel != "gemini-actual" {
				t.Fatalf("native generation failed: %+v", row)
			}
			if mode == "animation" && row.SVG == "" {
				t.Fatal("missing animation")
			}
			if mode == "precheck" && (row.Precheck == nil || row.Precheck.Verdict != "passed" || row.Precheck.Questions[0].Answer != "21") {
				t.Fatalf("invalid candy result: %+v", row.Precheck)
			}
			if row.Usage == nil || row.Usage.InputTokens == nil || *row.Usage.InputTokens != 12 || row.Usage.OutputTokens == nil || *row.Usage.OutputTokens != 34 || row.Usage.ReasoningTokens == nil || *row.Usage.ReasoningTokens != 5 || row.Usage.TotalTokens == nil || *row.Usage.TotalTokens != 51 {
				t.Fatalf("usage lost: %+v", row.Usage)
			}
			raw, _ := json.Marshal(task)
			if strings.Contains(string(raw), fixtureSecret) || strings.Contains(string(raw), "private reasoning") {
				t.Fatal("sensitive content persisted")
			}
		})
	}
}

func TestGeminiEndpointAnimationAcceptsNativeJSON(t *testing.T) {
	f := setup(t, 1, "gemini", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"modelVersion":"gemini-test","candidates":[{"content":{"parts":[{"text":%q}]},"finishReason":"STOP"}]}`, fixtureSVG)
	})
	result, err := f.service.RunEndpointAnimation(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: fixtureSecret, Platform: "gemini", Model: "gemini-test"}, "生成动画")
	if err != nil || result.SVG == "" {
		t.Fatalf("Gemini endpoint animation failed: %+v %v", result, err)
	}
}

func TestGeminiGenerationRejectsBlockedTruncatedAndIncompleteResponses(t *testing.T) {
	for _, scenario := range []struct{ name, body string }{
		{"safety", `{"candidates":[{"content":{"parts":[{"text":"21"}]},"finishReason":"SAFETY"}]}`},
		{"truncated", `{"candidates":[{"content":{"parts":[{"text":"21"}]},"finishReason":"MAX_TOKENS"}]}`},
		{"blocked prompt", `{"promptFeedback":{"blockReason":"SAFETY"}}`},
		{"thought only", `{"candidates":[{"content":{"parts":[{"thought":true,"text":"21"}]},"finishReason":"STOP"}]}`},
		{"empty", `{"candidates":[]}`},
		{"unfinished", `{"candidates":[{"content":{"parts":[{"text":"21"}]}}]}`},
		{"unknown finish", `{"candidates":[{"finishReason":"UNRECOGNIZED"}]}`},
		{"multiple candidates", `{"candidates":[{"finishReason":"STOP"},{"finishReason":"STOP"}]}`},
		{"business error", fmt.Sprintf(`{"error":{"message":%q}}`, fixtureSecret)},
		{"malformed", `{invalid}`},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", scenario.name, stream), func(t *testing.T) {
				f := setup(t, 1, "gemini", func(w http.ResponseWriter, r *http.Request) {
					if stream {
						fmt.Fprintf(w, "data: %s\n\n", scenario.body)
					} else {
						fmt.Fprint(w, scenario.body)
					}
				})
				result, err := f.service.RunEndpointCandy(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, Platform: "gemini", APIKey: fixtureSecret, Model: "gemini-test"})
				if err == nil || result.Precheck == nil || result.Precheck.Verdict != "error" {
					t.Fatalf("invalid response accepted: %+v %v", result, err)
				}
				raw, _ := json.Marshal(result)
				if strings.Contains(string(raw), fixtureSecret) {
					t.Fatal("secret leaked")
				}
			})
		}
	}
}

func TestGeminiAnimationCancellationDiscardsPartialOutput(t *testing.T) {
	started := make(chan struct{})
	f := setup(t, 1, "gemini", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":%q}]}}]}\n\n", fixtureSVG)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	queued, err := f.service.EnqueueAnimation(t.Context(), request("1"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("task was not cancellable")
	}
	task := finished(t, f)
	row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
	if task.Status != "cancelled" || row.SVG != "" || row.HTML != "" {
		t.Fatalf("partial output published: %+v", row)
	}
}

func TestGeminiUsageDoesNotGuessMissingThinkingTokens(t *testing.T) {
	for _, scenario := range []struct {
		name, usage string
		total       *int64
	}{
		{"missing", ``, nil},
		{"unknown thoughts", `,"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":34}`, nil},
		{"known thoughts", `,"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":34,"thoughtsTokenCount":5}`, new(int64(51))},
		{"zero thoughts", `,"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":34,"thoughtsTokenCount":0}`, new(int64(46))},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := setup(t, 1, "gemini", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"candidates":[{"content":{"parts":[{"text":"21"}]},"finishReason":"STOP"}]%s}`, scenario.usage)
			})
			result, err := f.service.RunEndpointCandy(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, Platform: "gemini", APIKey: fixtureSecret, Model: "gemini-test"})
			if err != nil {
				t.Fatal(err)
			}
			if scenario.total == nil {
				if result.Usage.TotalTokens != nil {
					t.Fatal("missing total was guessed")
				}
			} else if result.Usage.TotalTokens == nil || *result.Usage.TotalTokens != *scenario.total {
				t.Fatalf("total=%+v", result.Usage)
			}
		})
	}
}

func TestGeminiUnsupportedNativeEndpointDoesNotFallbackToOpenAI(t *testing.T) {
	var calls int
	f := setup(t, 1, "gemini", func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "native endpoint unsupported", http.StatusNotFound)
	})
	_, err := f.service.RunEndpointAnimation(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, Platform: "gemini", APIKey: fixtureSecret, Model: "gemini-test"}, "生成动画")
	if err == nil || calls != 1 {
		t.Fatalf("unexpected fallback calls=%d err=%v", calls, err)
	}
}
