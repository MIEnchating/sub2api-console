package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelfingerprint"
)

func TestPublicReferenceUsesExactKnowledgeParametersAndPreservesExecutionSuccess(t *testing.T) {
	for _, platform := range []string{"openai", "anthropic"} {
		t.Run(platform, func(t *testing.T) {
			model := "gpt-6-astra"
			if platform == "anthropic" {
				model = "claude-fable-5.1"
			}
			plans := map[string]modelfingerprint.Probe{}
			for _, p := range modelfingerprint.Plan(model) {
				plans[p.Prompt] = p
			}
			var calls atomic.Int32
			f := setup(t, 1, platform, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model           string
					Input           string
					Messages        []struct{ Role, Content string }
					System          string
					Temperature     *float64
					MaxTokens       int `json:"max_tokens"`
					Stream          bool
					ReasoningEffort string `json:"reasoning_effort"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				prompt := body.Input
				if len(body.Messages) > 0 {
					prompt = body.Messages[len(body.Messages)-1].Content
				}
				answer := "0"
				if p, ok := plans[prompt]; ok {
					calls.Add(1)
					if body.Stream || body.Temperature == nil || *body.Temperature != 0 || body.MaxTokens != p.MaxOutputTokens || body.ReasoningEffort != "" {
						t.Errorf("reference parameters differ: %+v", body)
					}
					if platform == "anthropic" {
						if body.System != p.Instructions {
							t.Error("missing native system prompt")
						}
					} else if r.URL.Path != "/v1/chat/completions" || len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[0].Content != p.Instructions {
						t.Error("not using reference chat prompt")
					}
					lines := []string{}
					for i, q := range p.Questions {
						lines = append(lines, fmt.Sprintf("(%d) %.12g", i+1, q.Value))
					}
					answer = strings.Join(lines, "\n")
				}
				if platform == "anthropic" {
					_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": answer}}})
				} else if body.Stream {
					_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "output_text": answer})
				} else {
					_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": answer}}}})
				}
			})
			result, err := f.service.RunEndpointVerification(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated-key", Platform: platform, Model: model}, nil, false, true)
			if err != nil {
				t.Fatal(err)
			}
			var report modelcheck.VerificationReport
			raw, _ := json.Marshal(result)
			_ = json.Unmarshal(raw, &report)
			if report.PublicReference == nil || report.PublicReference.Status != "matched" || int(calls.Load()) != len(plans) || report.Total != 17+int(calls.Load()) || report.CapabilityTotal != 7 {
				t.Fatalf("reference not integrated: %+v", report.PublicReference)
			}
			if result["requests"].(map[string]any)["successful"] != report.Total {
				t.Fatal("wrong answers must not fail requests")
			}
			for _, p := range report.Probes {
				if p.Category == "public_reference" && (p.Status != "observed" || p.ReferenceParameters == nil || p.ReferenceParameters.Stream) {
					t.Fatal("reference sample metadata lost")
				}
			}
		})
	}
}
func TestPublicTokenizerAcceptsIntentionalOneTokenFinishAndDoesNotAuthenticate(t *testing.T) {
	plans := map[string]int64{}
	deltas := map[string]int64{"tok-english": 47, "tok-chinese": 51, "tok-code": 63, "tok-emoji": 92}
	for _, p := range modelfingerprint.Plan("qwen3.7-flash") {
		if p.Prompt == "a" {
			plans[p.Prompt] = 7
		} else {
			plans[p.Prompt] = deltas[p.Cell] + 7
		}
	}
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/responses") {
			fmt.Fprint(w, `{"model":"qwen3.7-flash","output_text":"0"}`)
			return
		}
		var body struct {
			Messages  []struct{ Content string }
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.MaxTokens != 1 || len(body.Messages) != 1 {
			t.Error("tokenizer parameters changed")
		}
		tokens := plans[body.Messages[0].Content]
		fmt.Fprintf(w, `{"model":"qwen3.7-flash","usage":{"prompt_tokens":%d,"completion_tokens":1},"choices":[{"finish_reason":"length","message":{"content":""}}]}`, tokens)
	})
	result, err := f.service.RunEndpointVerification(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated-key", Platform: "openai", Model: "qwen3.7-flash"}, nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	var report modelcheck.VerificationReport
	raw, _ := json.Marshal(result)
	_ = json.Unmarshal(raw, &report)
	if report.PublicReference.Status != "inconclusive" || report.PublicReference.Tokenizer.Status != "consistent" || report.Total != 29 || len(report.Probes) != 29 {
		t.Fatalf("intentional length finish rejected: %+v", report.PublicReference)
	}
	for _, p := range report.Probes {
		if p.Status == "error" {
			t.Fatal(p.Error)
		}
	}
}
func TestPublicReferenceStopsAfterRateLimitAndRetainsPartialEvidence(t *testing.T) {
	var calls atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			calls.Add(1)
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":{"message":"isolated limit"}}`)
			return
		}
		fmt.Fprint(w, `{"model":"gpt-6-astra","output_text":"0"}`)
	})
	result, err := f.service.RunEndpointVerification(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated-key", Platform: "openai", Model: "gpt-6-astra"}, nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	var report modelcheck.VerificationReport
	raw, _ := json.Marshal(result)
	_ = json.Unmarshal(raw, &report)
	if calls.Load() != 1 || len(report.Probes) != 18 || report.PublicReference.Status != "inconclusive" || report.Probes[17].ErrorCode != "rate_limited" {
		t.Fatal("rate limit treated as identity mismatch or retried")
	}
}
func TestPublicReferenceCancellationKeepsCompletedSampleAndUnexecutedBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/responses") {
			fmt.Fprint(w, `{"model":"gpt-6-astra","output_text":"0"}`)
		} else {
			fmt.Fprint(w, `{"model":"gpt-6-astra","choices":[{"finish_reason":"stop","message":{"content":"(1) 2"}}]}`)
		}
	})
	result, err := f.service.RunEndpointVerification(ctx, modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated-key", Platform: "openai", Model: "gpt-6-astra"}, func(r modelcheck.VerificationReport) {
		if len(r.Probes) == 18 {
			cancel()
		}
	}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	var report modelcheck.VerificationReport
	raw, _ := json.Marshal(result)
	_ = json.Unmarshal(raw, &report)
	if len(report.Probes) != 18 || report.Total <= 18 || report.PublicReference.Status != "inconclusive" {
		t.Fatal("cancellation lost partial evidence")
	}
}
func TestPublicReferenceOAuthSkipsIncompatibleParametersWithoutExtraCalls(t *testing.T) {
	f, _ := oauthAccountFixture(t, `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"isolated-oauth-access-token"}}`)
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	var calls atomic.Int32
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return oauthResponse(401, "application/json", `{}`), nil
	}))
	if _, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}, PublicReference: true}); err != nil {
		t.Fatal(err)
	}
	runner.runs[0](t.Context())
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	var report modelcheck.VerificationReport
	raw, _ := json.Marshal(row)
	_ = json.Unmarshal(raw, &report)
	if calls.Load() != 1 || report.Total != 7 || report.PublicReference == nil || report.PublicReference.Status != "inconclusive" || report.PublicReference.Planned != 0 || !strings.Contains(report.PublicReference.Summary, "OAuth") {
		t.Fatal("OAuth cannot silently use incompatible reference")
	}
}
