package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestPrecheckSelectedQuestionsControlRequestsAndVerdict(t *testing.T) {
	for _, questions := range [][]string{{"candy"}} {
		t.Run(fmt.Sprint(questions), func(t *testing.T) {
			var mu sync.Mutex
			var asked []string
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input string `json:"input"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				id, answer := "candy", "22"
				mu.Lock()
				asked = append(asked, id)
				mu.Unlock()
				_, _ = fmt.Fprintf(w, `{"output_text":%q}`, answer)
			})
			input := modelcheck.AnimationRequest{Mode: "precheck", PrecheckQuestions: questions, Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: "gpt-6-astra"}}, TimeoutSeconds: 5}
			if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["animations"].([]modelcheck.AnimationResult)
			expected := "passed"
			if slices.Contains(questions, "candy") {
				expected = "not_passed"
			}
			if rows[0].Precheck.Verdict != expected || len(rows[0].Precheck.Questions) != len(questions) {
				t.Fatalf("results = %#v", rows)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(asked) != len(questions) {
				t.Fatalf("asked = %v", asked)
			}
			for _, id := range questions {
				if !slices.Contains(asked, id) {
					t.Fatalf("missing question %s", id)
				}
			}
		})
	}
}

func TestPrecheckRejectsEmptyUnknownAndDuplicateSelections(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid selection reached upstream") })
	for _, questions := range [][]string{{}, {"knowledge-cutoff"}, {"candy", "candy"}} {
		input := modelcheck.AnimationRequest{Mode: "precheck", PrecheckQuestions: questions, Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: "gpt-6-astra"}}, TimeoutSeconds: 5}
		if _, err := f.service.EnqueueAnimation(context.Background(), input); err == nil {
			t.Fatalf("selection accepted: %v", questions)
		}
		schedule := modelcheck.AnimationSchedule{Mode: "precheck", PrecheckQuestions: questions, AccountID: "1", Enabled: true, Model: "gpt-6-astra", IntervalMinutes: 10, TimeoutSeconds: 5}
		if _, err := f.service.SaveAnimationSchedule(context.Background(), schedule, "test"); err == nil {
			t.Fatalf("schedule accepted: %v", questions)
		}
	}
}

func TestPrecheckScheduleRestoresAndExecutesSelectedQuestion(t *testing.T) {
	var calls int
	var mu sync.Mutex
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !strings.Contains(body.Input, "圆形苹果") {
			t.Errorf("unexpected question: %s", body.Input)
		}
		mu.Lock()
		calls++
		mu.Unlock()
		_, _ = fmt.Fprint(w, `{"output_text":"无法提供日期"}`)
	})
	schedule := modelcheck.AnimationSchedule{Mode: "precheck", PrecheckQuestions: []string{"candy"}, AccountID: "1", Enabled: true, Model: "gpt-6-astra", IntervalMinutes: 10, TimeoutSeconds: 5}
	if _, err := f.service.SaveAnimationSchedule(context.Background(), schedule, "test"); err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	restarted.UseTaskRunner(f.runner)
	views := restarted.AnimationSchedules()
	if !slices.Equal(views[0].PrecheckQuestions, []string{"candy"}) {
		t.Fatalf("schedules = %#v", views)
	}
	views[0].PrecheckQuestions[0] = "candy"
	next, _ := time.Parse(time.RFC3339Nano, views[0].NextAt)
	restarted.RunDueAnimations(context.Background(), next)
	if task := finished(t, f); task.Status != "succeeded" {
		t.Fatalf("task = %#v", task)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestPrecheckScheduleMigratesRemovedKnowledgeQuestionOnLoad(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) {})
	f.catalog.raw = []byte(`[{"mode":"precheck","precheck_questions":["knowledge-cutoff"],"account_id":"1","enabled":true,"model":"gpt-6-astra","interval_minutes":10,"timeout_seconds":5,"version":1}]`)

	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	views := restarted.AnimationSchedules()
	if len(views) != 1 || !slices.Equal(views[0].PrecheckQuestions, []string{"candy"}) {
		t.Fatalf("schedules = %#v", views)
	}
}
