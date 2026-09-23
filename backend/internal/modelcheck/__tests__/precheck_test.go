package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAnimationPrecheckAsksOnlyCandyQuestionAndSeparatesVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name, candy, verdict string
		fail                 bool
	}{
		{"passed", "21", "passed", false},
		{"wrong candy", "22", "not_passed", false},
		{"other answer is degraded", "答案无法确定", "not_passed", false},
		{"request failed", "", "error", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var prompts []string
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input     string `json:"input"`
					Reasoning struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				prompts = append(prompts, body.Input)
				if body.Reasoning.Effort != "low" {
					t.Errorf("effort = %q", body.Reasoning.Effort)
				}
				if r.Header.Get("Authorization") != "Bearer "+fixtureSecret || r.Header.Get("X-Request-ID") == "" {
					t.Error("missing bound credential or request ID")
				}
				if tc.fail {
					w.WriteHeader(403)
					_, _ = fmt.Fprintf(w, `{"error":{"message":%q}}`, fixtureSecret)
					return
				}
				_, _ = fmt.Fprintf(w, `{"output_text":%q}`, tc.candy)
			})
			var input modelcheck.AnimationRequest
			if err := json.Unmarshal([]byte(`{"mode":"precheck","targets":[{"account_id":"1","model":"gpt-6-astra"}],"timeout_seconds":5}`), &input); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			raw, _ := json.Marshal(task)
			var report struct {
				Result struct {
					Animations []struct {
						SVG      string `json:"svg"`
						Precheck struct {
							Verdict   string `json:"verdict"`
							Questions []any  `json:"questions"`
						} `json:"precheck"`
					} `json:"animations"`
				} `json:"result"`
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			if len(report.Result.Animations) != 1 || report.Result.Animations[0].Precheck.Verdict != tc.verdict {
				t.Fatalf("task = %s", raw)
			}
			if len(prompts) != 1 || !strings.Contains(prompts[0], "圆形苹果") {
				t.Fatalf("prompts = %v", prompts)
			}
			if report.Result.Animations[0].SVG != "" || len(report.Result.Animations[0].Precheck.Questions) != 1 || strings.Contains(string(raw), fixtureSecret) {
				t.Fatalf("unexpected result = %s", raw)
			}
			statuses, err := f.service.AccountStatuses(context.Background())
			if err != nil || len(statuses) != 0 {
				t.Fatal("precheck must not change account behavioral status")
			}
		})
	}
}

func TestPrecheckSchedulePersistsModeAndRunsOnlyCandyQuestion(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		prompts = append(prompts, body.Input)
		mu.Unlock()
		_, _ = fmt.Fprint(w, `{"output_text":"21"}`)
	})
	var schedule modelcheck.AnimationSchedule
	if err := json.Unmarshal([]byte(`{"mode":"precheck","account_id":"1","model":"gpt-6-astra","enabled":true,"interval_minutes":10,"timeout_seconds":5,"version":0}`), &schedule); err != nil {
		t.Fatal(err)
	}
	views, err := f.service.SaveAnimationSchedule(context.Background(), schedule, "isolated-test")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(restarted.AnimationSchedules())
	if !strings.Contains(string(raw), `"mode":"precheck"`) {
		t.Fatalf("persisted schedules = %s", raw)
	}
	next, _ := time.Parse(time.RFC3339Nano, views[0].NextAt)
	f.service.RunDueAnimations(context.Background(), next)
	task := finished(t, f)
	if task.Operation != "account-model-precheck" || task.Status != "succeeded" {
		t.Fatalf("task = %#v", task)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(prompts) != 1 {
		t.Fatalf("prompts = %v", prompts)
	}
}

func TestPrecheckRejectsUnknownModeBeforeCreatingTasks(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid request reached upstream") })
	var input modelcheck.AnimationRequest
	_ = json.Unmarshal([]byte(`{"mode":"juice","targets":[{"account_id":"1","model":"gpt-6-astra"}],"timeout_seconds":5}`), &input)
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err == nil {
		t.Fatal("unsupported detection mode accepted")
	}
}

func TestPrecheckCancellationStopsBeforeSecondQuestionAndKeepsAccountReserved(t *testing.T) {
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		started <- struct{}{}
		<-r.Context().Done()
	})
	input := modelcheck.AnimationRequest{Mode: "precheck", Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: "gpt-6-astra"}}, TimeoutSeconds: 5}
	queued, err := f.service.EnqueueAnimation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("precheck did not start")
	}
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err == nil {
		t.Fatal("animation overlapped a precheck for the same account")
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("precheck could not be cancelled")
	}
	task := finished(t, f)
	if task.Status != "cancelled" || calls.Load() != 1 {
		t.Fatalf("status = %s, calls = %d", task.Status, calls.Load())
	}
}

func TestCustomPrecheckHidesCredentialsAndDoesNotBindAnAccount(t *testing.T) {
	f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) { t.Error("account endpoint called") })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"output_text":%q}`, "Cannot provide a cutoff date: "+fixtureSecret)
	}))
	defer server.Close()
	input := modelcheck.AnimationRequest{Mode: "precheck", TimeoutSeconds: 5, Custom: &modelcheck.AnimationCustomEndpoint{BaseURL: server.URL, APIKey: fixtureSecret, Platform: "openai", Model: "gpt-6-astra"}}
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	raw, _ := json.Marshal(task)
	if task.Status != "failed" || strings.Contains(string(raw), fixtureSecret) || strings.Contains(string(raw), "api_key") {
		t.Fatalf("task = %s", raw)
	}
	rows := task.Result["animations"].([]modelcheck.AnimationResult)
	if !strings.HasPrefix(rows[0].AccountID, "custom-") || rows[0].Precheck.Verdict != "error" {
		t.Fatalf("results = %#v", rows)
	}
	if rows[0].Endpoint != server.URL || rows[0].Platform != "openai" {
		t.Fatal("custom precheck lost endpoint metadata")
	}
}

func TestPrecheckRedactsCredentialFromResponseModel(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		answer := "21"
		if body.Input == "你的知识截至日期是什么时候" {
			answer = "我无法提供日期。"
		}
		_, _ = fmt.Fprintf(w, `{"output_text":%q,"model":%q}`, answer, "model-"+fixtureSecret)
	})
	input := modelcheck.AnimationRequest{Mode: "precheck", Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: "gpt-6-astra"}}, TimeoutSeconds: 5}
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	raw, _ := json.Marshal(task)
	if strings.Contains(string(raw), fixtureSecret) {
		t.Fatal("upstream response model exposed the credential")
	}
}
