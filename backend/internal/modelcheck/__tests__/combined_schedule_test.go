package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestCombinedScheduleRestoresAndKeepsBothResultsWhenPrecheckDoesNotPass(t *testing.T) {
	for _, reply := range []string{"21", "29", "http-error"} {
		t.Run(reply, func(t *testing.T) {
			var mu sync.Mutex
			var order []string
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input string `json:"input"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				animation := strings.Contains(body.Input, "SVG")
				mu.Lock()
				if animation {
					order = append(order, "animation")
				} else {
					order = append(order, "precheck")
				}
				mu.Unlock()
				if animation {
					_, _ = fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
					return
				}
				if reply == "http-error" {
					http.Error(w, "isolated failure", http.StatusBadRequest)
					return
				}
				_, _ = fmt.Fprintf(w, `{"output_text":%q}`, reply)
			})
			schedule := modelcheck.AnimationSchedule{Mode: "both", PrecheckQuestions: []string{"candy"}, AccountID: "1", Enabled: true, Model: "gpt-6-astra", IntervalMinutes: 10, TimeoutSeconds: 5}
			if _, err := f.service.SaveAnimationSchedule(context.Background(), schedule, "test"); err != nil {
				t.Fatal(err)
			}
			restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
			if err != nil {
				t.Fatal(err)
			}
			restarted.UseTaskRunner(f.runner)
			view := restarted.AnimationSchedules()[0]
			if view.Mode != "both" || len(view.PrecheckQuestions) != 1 {
				t.Fatalf("schedule = %#v", view)
			}
			next, _ := time.Parse(time.RFC3339Nano, view.NextAt)
			restarted.RunDueAnimations(context.Background(), next)
			task := finished(t, f)
			rows := task.Result["animations"].([]modelcheck.AnimationResult)
			if len(rows) != 2 || rows[0].Mode != "precheck" || rows[1].Mode != "animation" || rows[1].SVG == "" {
				t.Fatalf("results = %#v", rows)
			}
			if rows[0].RequestID == rows[1].RequestID {
				t.Fatal("phases must have separate request IDs")
			}
			wantVerdict, wantStatus := "passed", "succeeded"
			if reply == "29" {
				wantVerdict = "not_passed"
			}
			if reply == "http-error" {
				wantVerdict, wantStatus = "error", "partial"
			}
			if rows[0].Precheck.Verdict != wantVerdict || task.Status != wantStatus {
				t.Fatalf("verdict=%s status=%s", rows[0].Precheck.Verdict, task.Status)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(order) != 2 || order[0] != "precheck" || order[1] != "animation" {
				t.Fatalf("requests = %v", order)
			}
		})
	}
}

func TestCombinedDetectionCancellationDoesNotStartAnimation(t *testing.T) {
	started := make(chan struct{}, 1)
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Input, "SVG") {
			t.Error("animation started after cancellation")
			return
		}
		started <- struct{}{}
		<-r.Context().Done()
	})
	input := request("1")
	input.Mode, input.PrecheckQuestions = "both", []string{"candy"}
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
		t.Fatal("same account overlapped combined detection")
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("cannot cancel")
	}
	if task := finished(t, f); task.Status != "cancelled" {
		t.Fatalf("status = %s", task.Status)
	}
}

func TestCombinedScheduleRejectsEmptyUnknownAndDuplicateQuestions(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid selection reached upstream") })
	for _, questions := range [][]string{{}, {"juice-low"}, {"candy", "candy"}} {
		value := modelcheck.AnimationSchedule{Mode: "both", PrecheckQuestions: questions, AccountID: "1", Enabled: true, Model: "gpt-6-astra", IntervalMinutes: 10, TimeoutSeconds: 5}
		if _, err := f.service.SaveAnimationSchedule(context.Background(), value, "test"); err == nil {
			t.Fatalf("invalid questions accepted: %v", questions)
		}
	}
	if len(f.service.AnimationSchedules()) != 0 {
		t.Fatal("invalid schedule persisted")
	}
}
