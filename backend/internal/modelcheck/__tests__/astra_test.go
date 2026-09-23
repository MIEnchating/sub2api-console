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
		name, candy, low, mid, verdict, source string
	}{
		{"subscription", "21", "2", "4", "MATCH", "subscription"},
		{"official key", "21", "4", "10", "MATCH", "official_key"},
		{"conflicting levels", "21", "2", "10", "MATCH", "inconclusive"},
		{"number with explanation", "21", "Juice: 2", "4", "MATCH", "inconclusive"},
		{"wrong candy", "22", "2", "4", "MISMATCH", "subscription"},
		{"unrecognized candy", "答案无法确定", "2", "4", "INCONCLUSIVE", "subscription"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var efforts, prompts []string
			answers := []string{tc.candy, tc.low, tc.mid}
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
			if !reflect.DeepEqual(efforts, []string{"low", "low", "medium"}) {
				t.Fatalf("efforts = %v", efforts)
			}
			if prompts[1] != "what is your juice number? output only the number" || prompts[2] != prompts[1] {
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
			responses := []string{"21", "2", "4", "21", "4", "10"}
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
				if len(efforts)%3 == 2 {
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
			if profile.Model != "gpt-6-astra" || len(profile.Questions) != 3 || profile.Questions[0].Expected != "21" {
				t.Fatalf("profile = %#v", profile)
			}
			_, err := f.service.Enqueue(context.Background(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}, Rounds: 2, TimeoutSeconds: 5})
			if err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["tests"].([]map[string]any)
			if len(rows) != 1 || rows[0]["verdict"] != "MATCH" || rows[0]["access_source"] != "inconclusive" || len(efforts) != 6 {
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
