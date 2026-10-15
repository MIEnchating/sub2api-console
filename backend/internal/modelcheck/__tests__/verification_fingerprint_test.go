package modelcheck_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestVerificationFingerprintMeasuresRepeatedTextsWithoutOfficialComparison(t *testing.T) {
	for _, mode := range []string{"stable", "unstable", "missing", "rate_limited", "constant_usage"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			counts := map[string]int{}
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input    string
					Messages []struct{ Content string }
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if len(body.Messages) > 0 {
					body.Input = body.Messages[0].Content
				}
				sample := strings.HasPrefix(body.Input, "[分词指纹 v1]")
				tokens := 100
				if strings.Contains(body.Input, "机器学习") {
					tokens += 30
				}
				if strings.Contains(body.Input, "The quick") {
					tokens += 24
				}
				if strings.Contains(body.Input, "function sum") {
					tokens += 42
				}
				if strings.Contains(body.Input, "😀") {
					tokens += 50
				}
				if sample {
					tokens += 17
				}
				if sample {
					if mode == "rate_limited" {
						w.WriteHeader(429)
						return
					}
					if mode == "unstable" {
						mu.Lock()
						counts[body.Input]++
						tokens += counts[body.Input]
						mu.Unlock()
					}
				}
				if mode == "constant_usage" {
					tokens = 100
				}
				response := map[string]any{"output_text": "OK", "model": "new-model"}
				if !(sample && mode == "missing") {
					response["usage"] = map[string]any{"input_tokens": tokens, "output_tokens": 1}
				}
				_ = json.NewEncoder(w).Encode(response)
			})
			ids := []string{"1"}
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: ids, Models: []string{"new-model"}, TimeoutSeconds: 5, Fingerprint: true})
			if err != nil {
				t.Fatal(err)
			}
			runner.runs[0](t.Context())
			row := finished(t, f).Result["tests"].([]map[string]any)[0]
			raw, _ := json.Marshal(row)
			var report struct {
				Fingerprint struct {
					Status  string
					Samples []any
				}
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			wantFingerprint := "stable"
			switch mode {
			case "unstable":
				wantFingerprint = "unstable"
			case "missing", "constant_usage":
				wantFingerprint = "unavailable"
			case "rate_limited":
				wantFingerprint = "incomplete"
			}
			if report.Fingerprint.Status != wantFingerprint {
				t.Fatalf("want %s got %s", wantFingerprint, raw)
			}
			if row["reference"] != nil || row["reference_comparisons"] != nil {
				t.Fatal("retired comparison must not be generated")
			}
			if mode == "rate_limited" && len(row["probes"].([]any)) != 8 {
				t.Fatal("rate limiting must stop subsequent fingerprint requests")
			}
			if mode == "stable" && len(report.Fingerprint.Samples) != 5 {
				t.Fatalf("missing text measurements: %s", raw)
			}
			if fmt.Sprint(row["capability_total"]) != "7" {
				t.Fatalf("fingerprint must not count as capability question: %s", raw)
			}
			if mode != "rate_limited" && len(row["probes"].([]any)) != 17 {
				t.Fatalf("expected seven capability and ten real sampling requests: %s", raw)
			}
		})
	}
}

func TestVerificationFingerprintUsesNativeClaudeAndGeminiMetadata(t *testing.T) {
	for _, platform := range []string{"claude", "gemini"} {
		t.Run(platform, func(t *testing.T) {
			f := setup(t, 1, platform, func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				tokens := 100
				if strings.Contains(string(raw), "机器学习") {
					tokens = 130
				}
				if platform == "claude" {
					if !strings.HasSuffix(r.URL.Path, "/messages") {
						t.Errorf("unexpected Claude path: %s", r.URL.Path)
					}
					fmt.Fprintf(w, `{"model":"new-model","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":%d,"output_tokens":1}}`, tokens)
				} else {
					if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
						t.Errorf("unexpected Gemini path: %s", r.URL.Path)
					}
					fmt.Fprintf(w, `{"modelVersion":"new-model","candidates":[{"content":{"parts":[{"text":"OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":%d,"candidatesTokenCount":1}}`, tokens)
				}
			})
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, Fingerprint: true, TimeoutSeconds: 5})
			if err != nil {
				t.Fatal(err)
			}
			runner.runs[0](t.Context())
			row := finished(t, f).Result["tests"].([]map[string]any)[0]
			raw, _ := json.Marshal(row)
			var report modelcheck.VerificationReport
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			if report.Fingerprint.Status != "stable" || len(report.Fingerprint.Samples) != 5 {
				t.Fatalf("native sampling incomplete: %s", raw)
			}
			sample := report.Fingerprint.Samples[1]
			if sample.Delta == nil || *sample.Delta != 30 {
				t.Fatalf("native input metadata lost: %+v", sample)
			}
		})
	}
}
