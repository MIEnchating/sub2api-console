package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestBehaviorFingerprintSeparatesDistributionFromCapabilityAndIdentity(t *testing.T) {
	counts := map[string]int{}
	var mu sync.Mutex
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		counts[body.Input]++
		count := counts[body.Input]
		mu.Unlock()
		answer := "0"
		switch {
		case strings.Contains(body.Input, "one color"):
			answer = " BLUE "
		case strings.Contains(body.Input, "一种颜色"):
			answer = "红"
		case strings.Contains(body.Input, "heads or tails"):
			answer = "heads"
			if count%2 == 0 {
				answer = "tails"
			}
		case strings.Contains(body.Input, "正面和反面"):
			answer = "正面"
		case strings.Contains(body.Input, "one direction"):
			answer = "north"
		case strings.Contains(body.Input, "一个方向"):
			answer = "北"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": answer, "model": "model-a"})
	})
	result, err := f.service.RunEndpointVerification(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated-key", Model: "model-a", Platform: "openai"}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	var report modelcheck.VerificationReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	fp := report.BehaviorFingerprint
	if fp == nil || fp.Status != "collected" || fp.Valid != 200 || len(fp.Cells) != 8 || report.Total != 217 || report.CapabilityTotal != 7 || report.Capability.Status != "failed" || fp.Identity != "inconclusive" {
		t.Fatalf("incorrect report dimensions: %+v", fp)
	}
	if fp.Parameters.Temperature != "provider_default" || fp.Parameters.ReasoningEffort != "medium" || fp.Parameters.MaxOutputTokens != 4096 || !fp.Parameters.Stream {
		t.Fatal("sampling parameters not recorded")
	}
	if fp.Cells[0].SelfJSD == nil || *fp.Cells[0].SelfJSD != 0 || fp.Cells[0].Counts["0"] != 25 {
		t.Fatal("constant distribution must have zero self divergence")
	}
	if fp.Cells[4].SelfJSD == nil || *fp.Cells[4].SelfJSD != 1 || fp.Cells[4].Counts["heads"] != 13 || fp.Cells[4].Counts["tails"] != 12 {
		t.Fatal("disjoint odd/even distributions must have unit divergence")
	}
	if fp.Cells[2].Counts["blue"] != 25 {
		t.Fatal("trim and case normalization lost")
	}
	if report.Probes[17].ID != "behavior_number_en_01" || report.Probes[24].ID != "behavior_direction_zh_01" || report.Probes[25].ID != "behavior_number_en_02" {
		t.Fatal("cells must be interleaved")
	}
}

func TestBehaviorFingerprintRejectsInvalidSamplesAndMixedMetadata(t *testing.T) {
	for _, scenario := range []string{"invalid", "empty", "mixed_model", "mixed_cells", "missing_model", "limited_samples", "sufficient_partial", "rate_limit", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			samples := 0
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Input string }
				_ = json.NewDecoder(r.Body).Decode(&body)
				answer, model := "0", "model-a"
				if strings.HasPrefix(body.Input, "Choose") || strings.HasPrefix(body.Input, "从") {
					samples++
					switch scenario {
					case "invalid":
						answer = "0 or 1"
					case "empty":
						answer = ""
					case "missing_model":
						model = ""
					case "mixed_cells":
						if strings.HasPrefix(body.Input, "从") {
							model = "model-b"
						}
					case "mixed_model":
						if (samples-1)/8%2 == 1 {
							model = "model-b"
						}
					case "sufficient_partial":
						if samples > 80 {
							w.WriteHeader(429)
							return
						}
					case "limited_samples":
						if samples > 32 {
							w.WriteHeader(429)
							return
						}
					case "rate_limit":
						w.WriteHeader(429)
						return
					case "cancelled":
						cancel()
						return
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"output_text": answer, "model": model})
			})
			result, err := f.service.RunEndpointVerification(ctx, modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated-key", Model: "model-a", Platform: "openai"}, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(result)
			var report modelcheck.VerificationReport
			_ = json.Unmarshal(raw, &report)
			fp := report.BehaviorFingerprint
			if fp == nil || fp.Identity != "inconclusive" {
				t.Fatal("missing identity boundary")
			}
			cell := fp.Cells[0]
			if scenario != "mixed_cells" && scenario != "sufficient_partial" && cell.SelfJSD != nil {
				t.Fatalf("invalid/incomparable/insufficient samples produced JSD: %+v", cell)
			}
			switch scenario {
			case "invalid":
				if cell.Valid != 0 || cell.Invalid != 25 {
					t.Fatalf("ambiguous answers counted: %+v", cell)
				}
			case "empty":
				if cell.Errors != 25 {
					t.Fatalf("empty replies counted as valid: %+v", cell)
				}
			case "mixed_model", "mixed_cells", "missing_model":
				if fp.Status != "incomparable" {
					t.Fatal("metadata mismatch accepted")
				}
			case "sufficient_partial":
				if cell.Valid != 10 || cell.SelfJSD == nil || *cell.SelfJSD != 0 || cell.Missing != 14 || cell.Errors != 1 {
					t.Fatalf("minimum valid coverage lost: %+v", cell)
				}
			case "limited_samples":
				if cell.Valid != 4 || cell.Missing != 20 || cell.Errors != 1 {
					t.Fatalf("missing samples not preserved: %+v", cell)
				}
			case "rate_limit", "cancelled":
				if samples != 1 || len(report.Probes) != 18 || cell.Errors != 1 || cell.Missing != 24 {
					t.Fatalf("stop must preserve partial samples: %s", raw)
				}
			}
		})
	}
}

func TestBehaviorFingerprintUsesNativeProtocolAdapters(t *testing.T) {
	for _, platform := range []string{"claude", "gemini", "openai"} {
		t.Run(platform, func(t *testing.T) {
			var calls atomic.Int32
			f := setup(t, 1, platform, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch platform {
				case "claude":
					if !strings.HasSuffix(r.URL.Path, "/messages") {
						t.Error("wrong Claude protocol")
					}
					fmt.Fprint(w, `{"model":"model-a","content":[{"type":"text","text":"0"}],"stop_reason":"end_turn"}`)
				case "gemini":
					if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
						t.Error("wrong Gemini protocol")
					}
					fmt.Fprint(w, `{"modelVersion":"model-a","candidates":[{"content":{"parts":[{"text":"0"}]},"finishReason":"STOP"}]}`)
				default:
					if strings.HasSuffix(r.URL.Path, "/responses") {
						w.WriteHeader(404)
						return
					}
					if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
						t.Error("wrong fallback protocol")
					}
					fmt.Fprint(w, `{"model":"model-a","choices":[{"message":{"content":"0"},"finish_reason":"stop"}]}`)
				}
			})
			native := platform
			if native == "claude" {
				native = "anthropic"
			}
			result, err := f.service.RunEndpointVerification(t.Context(), modelcheck.AnimationCustomEndpoint{BaseURL: *f.catalog.details["1"].BaseURL, APIKey: "isolated-key", Model: "model-a", Platform: native}, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(result)
			var report modelcheck.VerificationReport
			_ = json.Unmarshal(raw, &report)
			cell := report.BehaviorFingerprint.Cells[0]
			if cell.Valid != 25 || cell.SelfJSD == nil || len(cell.Protocols) != 1 {
				t.Fatalf("native samples lost: %+v requests=%d", cell, calls.Load())
			}
		})
	}
}

func TestBehaviorFingerprintManagedTaskIncludesBudgetAndStopsOnAuthorizationFailure(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"model-a"}, Rounds: 2, TimeoutSeconds: 5, Fingerprint: true, BehaviorFingerprint: true})
	if err != nil {
		t.Fatal(err)
	}
	runner.runs[0](t.Context())
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	raw, _ := json.Marshal(row)
	var report modelcheck.VerificationReport
	_ = json.Unmarshal(raw, &report)
	if report.Total != 224 || report.CapabilityTotal != 14 || len(report.ProbePlan) != 224 || len(report.Probes) != 1 || report.BehaviorFingerprint == nil || report.BehaviorFingerprint.Valid != 0 || report.BehaviorFingerprint.Cells[0].Missing != 25 {
		t.Fatal("managed task lost behavior option, budget or unexecuted samples")
	}
}

func TestBehaviorFingerprintOAuthRecordsItsDistinctSamplingProfile(t *testing.T) {
	f, _ := oauthAccountFixture(t, `{"id":1,"type":"oauth","platform":"openai","credentials":{"access_token":"isolated-oauth-access-token"}}`)
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["instructions"] == nil || payload["temperature"] != nil || payload["max_output_tokens"] != nil {
			t.Error("OAuth sampling parameters changed")
		}
		return oauthResponse(401, "application/json", `{}`), nil
	}))
	if _, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}, BehaviorFingerprint: true}); err != nil {
		t.Fatal(err)
	}
	runner.runs[0](t.Context())
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	raw, _ := json.Marshal(row)
	var report modelcheck.VerificationReport
	_ = json.Unmarshal(raw, &report)
	fp := report.BehaviorFingerprint
	if fp == nil || fp.Profile != "verification-medium-v1:oauth" || fp.Parameters.Instructions == "" || fp.Parameters.MaxOutputTokens != 0 || report.Total != 207 || fp.Status != "incomplete" {
		t.Fatalf("OAuth sampling profile or partial evidence lost: %+v", fp)
	}
}
