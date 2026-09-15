package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAstraOAuthClassifiesOnlyConsistentLowAndMediumAnswers(t *testing.T) {
	for _, tc := range []struct {
		name, candy, cutoff, low, mid, verdict, source string
	}{
		{"subscription", "21", "我无法提供知识截止日期。", "2", "4", "MATCH", "subscription"},
		{"official key", "21", "I cannot provide a knowledge cutoff date.", "4", "10", "MATCH", "official_key"},
		{"conflicting levels", "21", "我无法提供知识截止日期。", "2", "10", "MATCH", "inconclusive"},
		{"number with explanation", "21", "我无法提供知识截止日期。", "Juice: 2", "4", "MATCH", "inconclusive"},
		{"wrong candy", "22", "我无法提供知识截止日期。", "2", "4", "MISMATCH", "subscription"},
		{"numeric date", "21", "我的知识截至 2025 年 8 月。", "2", "4", "MISMATCH", "subscription"},
		{"date alongside refusal", "21", "我无法提供准确日期，大约是 2025 年。", "2", "4", "MISMATCH", "subscription"},
		{"Chinese date", "21", "我的知识截至二〇二五年八月。", "2", "4", "MISMATCH", "subscription"},
		{"English month", "21", "I cannot give an exact cutoff; around August.", "2", "4", "MISMATCH", "subscription"},
		{"model name is not a date", "21", "我是 gpt-6-astra，无法提供知识截止日期。", "2", "4", "MATCH", "subscription"},
		{"numeric month and day", "21", "我不能提供年份，只能说是 8/31。", "2", "4", "MISMATCH", "subscription"},
		{"unrelated reply", "21", "你好。", "2", "4", "INCONCLUSIVE", "subscription"},
		{"empty reply", "21", "", "2", "4", "INCONCLUSIVE", "subscription"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var efforts, prompts []string
			answers := []string{tc.candy, tc.cutoff, tc.low, tc.mid}
			service := oauthService(t, func(request *http.Request) (*http.Response, error) {
				var body struct {
					Model string `json:"model"`
					Input []struct {
						Content string `json:"content"`
					} `json:"input"`
					Reasoning struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
				}
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					return nil, err
				}
				if body.Model != "gpt-6-astra" {
					t.Errorf("model = %q", body.Model)
				}
				efforts = append(efforts, body.Reasoning.Effort)
				prompts = append(prompts, body.Input[0].Content)
				if len(efforts) > len(answers) {
					t.Fatal("unexpected additional probe")
				}
				return oauthResponse(200, "application/json", fmt.Sprintf(`{"status":"completed","output_text":%q}`, answers[len(efforts)-1])), nil
			})
			result, err := service.CheckOAuth(context.Background(), "41", "测试账号", map[string]any{"access_token": oauthFixtureToken}, "gpt-6-astra", 5)
			if err != nil {
				t.Fatal(err)
			}
			if result["verdict"] != tc.verdict || result["access_source"] != tc.source {
				t.Fatalf("result = %#v", result)
			}
			if !reflect.DeepEqual(efforts, []string{"low", "low", "low", "medium"}) {
				t.Fatalf("efforts = %v", efforts)
			}
			if prompts[1] != "你的知识截至日期是什么时候" || prompts[2] != "what is your juice number? output only the number" || prompts[3] != prompts[2] {
				t.Fatalf("prompts changed: %v", prompts)
			}
		})
	}
}

func TestAstraOAuthAllFailedRequestsRemainErrors(t *testing.T) {
	service := oauthService(t, func(*http.Request) (*http.Response, error) {
		return oauthResponse(403, "application/json", `{"error":{"message":"denied"}}`), nil
	})
	result, err := service.CheckOAuth(context.Background(), "41", "测试账号", map[string]any{"access_token": oauthFixtureToken}, "gpt-6-astra", 5)
	if err != nil {
		t.Fatal(err)
	}
	if result["verdict"] != "ERROR" || result["access_source"] != "inconclusive" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAstraAccountChecksPreserveReasoningOnChatFallbackAndAcrossRounds(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprintf("chat fallback %t", fallback), func(t *testing.T) {
			var efforts []string
			responses := []string{"21", "我无法提供知识截止日期。", "2", "4", "21", "我无法提供知识截止日期。", "4", "10"}
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Reasoning struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
					ReasoningEffort string `json:"reasoning_effort"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				expected := "low"
				if len(efforts)%4 == 3 {
					expected = "medium"
				}
				if r.URL.Path == "/v1/responses" {
					if body.Reasoning.Effort != expected {
						t.Errorf("Responses effort = %q, want %q", body.Reasoning.Effort, expected)
					}
					if fallback {
						w.WriteHeader(404)
						return
					}
				} else if r.URL.Path != "/v1/chat/completions" || body.ReasoningEffort != expected {
					t.Errorf("Chat fallback lost reasoning effort: %s %s", r.URL.Path, body.ReasoningEffort)
				}
				if len(efforts) >= len(responses) {
					t.Error("extra request")
					w.WriteHeader(500)
					return
				}
				answer := responses[len(efforts)]
				efforts = append(efforts, expected)
				if fallback {
					_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, answer)
				} else {
					_, _ = fmt.Fprintf(w, `{"output_text":%q}`, answer)
				}
			})
			if !slices.Contains(f.service.Capabilities().AstraModels, "gpt-6-astra") {
				t.Fatal("Astra missing from capabilities")
			}
			profile := f.service.Configuration().BuiltinAstraProfile
			if profile.Model != "gpt-6-astra" || len(profile.Questions) != 4 || profile.Questions[0].Expected != "21" {
				t.Fatalf("profile = %#v", profile)
			}
			_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}, Rounds: 2, TimeoutSeconds: 5})
			if err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["tests"].([]map[string]any)
			if len(rows) != 1 || rows[0]["verdict"] != "MATCH" || rows[0]["access_source"] != "inconclusive" || len(efforts) != 8 {
				t.Fatalf("task = %#v, efforts = %v", task, efforts)
			}
		})
	}
}

func TestAstraRejectsAnthropicTransportWithoutSendingUncontrolledReasoning(t *testing.T) {
	f := setup(t, 1, "anthropic", func(http.ResponseWriter, *http.Request) { t.Error("unsupported transport reached upstream") })
	_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}, Rounds: 1, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	rows := task.Result["tests"].([]map[string]any)
	if len(rows) != 1 || rows[0]["verdict"] != "ERROR" {
		t.Fatalf("task = %#v", task)
	}
}
