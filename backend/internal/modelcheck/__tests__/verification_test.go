package modelcheck_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestVerificationAcceptsNewModelAndDoesNotTreatFixedAnswerAsIdentityProof(t *testing.T) {
	f := setup(t, 1, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "claude-new-model", "content": []any{map[string]any{"type": "text", "text": "**21**"}}, "stop_reason": "end_turn"})
	})
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"claude-new-model"}, Rounds: 1, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	raw, _ := json.Marshal(row)
	var report struct {
		Checker      string `json:"checker"`
		Authenticity struct {
			Status string `json:"status"`
		} `json:"authenticity"`
		Capability struct {
			Status string `json:"status"`
		} `json:"capability"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Checker != "verification" || report.Authenticity.Status != "observed" || report.Capability.Status != "failed" {
		t.Fatalf("fixed response must not verify identity or reasoning: %s", raw)
	}
}

func TestVerificationRateLimitLeavesCapabilityInconclusive(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}, Rounds: 1, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	raw, _ := json.Marshal(row)
	var report struct {
		Verdict    string `json:"verdict"`
		Capability struct {
			Status string `json:"status"`
		} `json:"capability"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "ERROR" || report.Capability.Status != "inconclusive" {
		t.Fatalf("rate limit is not reasoning failure: %s", raw)
	}
}

func TestVerificationRecordsProtocolAndSeparatesEmptyResponses(t *testing.T) {
	f := setup(t, 1, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "claude-new-model", "content": []any{}, "stop_reason": "end_turn"})
	})
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"claude-new-model"}, Rounds: 1, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	raw, _ := json.Marshal(row)
	var report struct {
		Probes []struct {
			ErrorCode  string `json:"error_code"`
			HTTPStatus int    `json:"http_status"`
			Protocol   string `json:"protocol"`
		} `json:"probes"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Probes) == 0 || report.Probes[0].ErrorCode != "empty_response" || report.Probes[0].HTTPStatus != 200 || report.Probes[0].Protocol != "anthropic_messages" {
		t.Fatalf("missing protocol or empty-response distinction: %s", raw)
	}
}

func TestVerificationMarkdownAnswerAndAmbiguousAnswerHaveDifferentResults(t *testing.T) {
	for _, tc := range []struct{ name, prefix, suffix, want string }{
		{"bold", "**", "**", "passed"},
		{"inline code", "`", "`", "passed"},
		{"explanation", "答案是", "", "failed"},
		{"multiple answers", "", " 或 21", "failed"},
		{"unclosed bold", "**", "", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input string `json:"input"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				var a, b, c, d int
				answer := "NO"
				if n, _ := fmt.Sscanf(body.Input, "计算 (%d + %d) × %d - %d。只输出最终整数。", &a, &b, &c, &d); n == 4 {
					answer = tc.prefix + strconv.Itoa((a+b)*c-d) + tc.suffix
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"model": "new-model", "output_text": answer, "status": "completed"})
			})
			_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
			if err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["tests"].([]map[string]any)[0]
			probes := row["probes"].([]any)
			if probes[0].(map[string]any)["status"] != tc.want {
				t.Fatalf("answer parsing: %#v", probes[0])
			}
		})
	}
}

func TestVerificationSameTaskUsesSameQuestionsAndSerializesModelsPerAccount(t *testing.T) {
	var mu sync.Mutex
	prompts := map[string][]string{}
	active := map[string]int{}
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		for model, count := range active {
			if model != body.Model && count > 0 {
				t.Error("different models overlap on the same account")
			}
		}
		active[body.Model]++
		defer func() { mu.Lock(); active[body.Model]--; mu.Unlock() }()
		prompts[body.Model] = append(prompts[body.Model], body.Input)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "NO", "model": body.Model})
	})
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"model-a", "model-b"}, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	finished(t, f)
	mu.Lock()
	defer mu.Unlock()
	slices.Sort(prompts["model-a"])
	slices.Sort(prompts["model-b"])
	if len(prompts["model-a"]) != 7 || !slices.Equal(prompts["model-a"], prompts["model-b"]) {
		t.Fatalf("task questions differ: %v", prompts)
	}
}

func TestVerificationStructuredJSONRejectsDuplicateKeys(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprint(duplicate), func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Input string }
				_ = json.NewDecoder(r.Body).Decode(&body)
				answer := "NO"
				if strings.HasPrefix(body.Input, "只返回 JSON 对象") {
					key := regexp.MustCompile(`[0-9a-f]{16}`).FindString(body.Input)
					numbers := regexp.MustCompile(`整数 ([0-9]+)\+([0-9]+)`).FindStringSubmatch(body.Input)
					if len(numbers) != 3 {
						t.Error("invalid structured question")
						w.WriteHeader(500)
						return
					}
					a, _ := strconv.Atoi(numbers[1])
					b, _ := strconv.Atoi(numbers[2])
					answer = fmt.Sprintf(`{"value":%d,"code":%q}`, a+b, key)
					if duplicate {
						answer = fmt.Sprintf(`{"value":0,"value":%d,"code":%q}`, a+b, key)
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"output_text": answer, "model": "new-model"})
			})
			_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
			if err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["tests"].([]map[string]any)[0]
			probe := row["probes"].([]any)[5].(map[string]any)
			want := "passed"
			if duplicate {
				want = "failed"
			}
			if probe["status"] != want {
				t.Fatalf("JSON output contract: %v", probe)
			}
		})
	}
}

func TestVerificationCancellationPreservesCompletedProbe(t *testing.T) {
	blocked := make(chan struct{})
	var blockedOnce sync.Once
	release := make(chan struct{})
	var calls atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "21"})
			return
		}
		blockedOnce.Do(func() { close(blocked) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	queued, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-time.After(3 * time.Second):
		t.Fatal("second probe did not start")
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("task not cancellable")
	}
	final := finished(t, f)
	rows := final.Result["tests"].([]map[string]any)
	if final.Status != "cancelled" || len(rows) != 1 {
		t.Fatalf("cancelled task lost results: %v", final)
	}
	probes := rows[0]["probes"].([]any)
	if (len(probes) < 2 || len(probes) > 5) || probes[0].(map[string]any)["answer"] != "21" || probes[1].(map[string]any)["error_code"] != "interrupted" {
		t.Fatalf("partial evidence lost: %v", probes)
	}
	for _, probe := range probes[1:] {
		if probe.(map[string]any)["error_code"] != "interrupted" {
			t.Fatal("in-flight cancellation lost")
		}
	}
}

func TestVerificationOperationalFailureIsNotAWrongAnswer(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		status, attempts int
	}{
		{"permission", "{}", "unauthorized", 403, 1},
		{"unsupported", "{}", "unsupported_request", 400, 1},
		{"malformed", "{", "invalid_response", 200, 7},
		{"incomplete", `{"status":"incomplete","output_text":"21"}`, "incomplete", 200, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
			if err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["tests"].([]map[string]any)[0]
			probes := row["probes"].([]any)
			if row["capability"].(map[string]any)["status"] != "inconclusive" || len(probes) != tc.attempts || probes[0].(map[string]any)["error_code"] != tc.code {
				t.Fatalf("request failure was misclassified: %v", row)
			}
		})
	}
}

func TestVerificationSelectedOfficialAccountRunsIndependentlyWithoutComparison(t *testing.T) {
	f := setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "21", "model": "new-model", "usage": map[string]any{"input_tokens": 50, "output_tokens": 1}})
	})
	local, _ := url.Parse(*f.catalog.details["2"].BaseURL)
	official := "https://api.openai.com"
	f.catalog.details["2"].BaseURL = &official
	previous := http.DefaultTransport
	http.DefaultTransport = oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		endpoint := *r.URL
		clone.URL = &endpoint
		if endpoint.Host == "api.openai.com" {
			clone.URL.Scheme = local.Scheme
			clone.URL.Host = local.Host
		}
		return previous.RoundTrip(clone)
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1", "2"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	rows := finished(t, f).Result["tests"].([]map[string]any)
	for _, row := range rows {
		if _, ok := row["reference"]; ok {
			t.Fatalf("removed comparison still emitted: %v", row["reference"])
		}
		if _, ok := row["reference_comparisons"]; ok {
			t.Fatal("comparison details still emitted")
		}
		for _, item := range row["evidence"].([]any) {
			if strings.Contains(item.(map[string]any)["id"].(string), "reference") {
				t.Fatal("comparison evidence still generated")
			}
		}
	}

}

func TestVerificationAllCorrectAnswersPassCapabilityWithoutCertifyingAlias(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		prompt := body.Input
		numbers := regexp.MustCompile(`[0-9]+`).FindAllString(prompt, -1)
		integer := func(i int) int { n, _ := strconv.Atoi(numbers[i]); return n }
		answer := ""
		switch {
		case strings.HasPrefix(prompt, "计算"):
			answer = strconv.Itoa((integer(0)+integer(1))*integer(2) - integer(3))
		case strings.HasPrefix(prompt, "A、B、C"):
			answer = strconv.Itoa((integer(0)-integer(1))/4 + integer(1))
		case strings.HasPrefix(prompt, "执行整数伪代码"):
			x := integer(0)
			for i := 1; i <= 4; i++ {
				if i%2 == 0 {
					x *= 2
				} else {
					x += i
				}
			}
			answer = strconv.Itoa(x)
		case strings.HasPrefix(prompt, "有向图"):
			paths := []int{integer(0) + integer(2), integer(1) + integer(3), integer(0) + integer(4) + integer(3)}
			slices.Sort(paths)
			answer = strconv.Itoa(paths[0])
		case strings.HasPrefix(prompt, "普通记录"):
			matches := regexp.MustCompile(`校验码是 ([0-9]+)`).FindStringSubmatch(prompt)
			answer = matches[1]
		case strings.HasPrefix(prompt, "只返回 JSON"):
			key := regexp.MustCompile(`[0-9a-f]{16}`).FindString(prompt)
			matches := regexp.MustCompile(`整数 ([0-9]+)\+([0-9]+)`).FindStringSubmatch(prompt)
			a, _ := strconv.Atoi(matches[1])
			b, _ := strconv.Atoi(matches[2])
			raw, _ := json.Marshal(map[string]any{"code": key, "value": a + b})
			answer = string(raw)
		default:
			t.Error("unrecognized challenge")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": answer, "model": "provider-alias"})
	})
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	if row["verdict"] != "NO_ANOMALIES" || row["capability"].(map[string]any)["status"] != "passed" || row["authenticity"].(map[string]any)["status"] != "evidence_mismatch" {
		t.Fatalf("capability and identity conflated: %v", row)
	}
}
