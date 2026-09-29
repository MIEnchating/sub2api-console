package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestSelectedModelReachesEveryManualStage(t *testing.T) {
	for _, mode := range []string{"animation", "precheck", "terminal"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model     string `json:"model"`
					Reasoning struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != "selected-model" {
					t.Errorf("requested model overwritten: %q", body.Model)
				}
				if mode == "animation" && body.Reasoning.Effort != "low" {
					t.Error("animation effort missing")
				}
				if mode == "precheck" && body.Reasoning.Effort != "medium" {
					t.Error("precheck effort missing")
				}
				answer := fixtureSVG
				if mode == "precheck" {
					answer = "21"
				}
				if mode == "terminal" {
					answer = `{"tool":"exec_command","command":"git status --short"}`
				}
				fmt.Fprintf(w, `{"output_text":%q}`, answer)
			})
			input := request("1")
			input.Targets[0].Model = " selected-model "
			input.Mode = mode
			var err error
			if mode == "terminal" {
				_, err = f.service.EnqueueTerminalContinuity(context.Background(), modelcheck.TerminalContinuityRequest{Targets: input.Targets, TimeoutSeconds: 5})
			} else {
				_, err = f.service.EnqueueAnimation(context.Background(), input)
			}
			if err != nil {
				t.Fatal(err)
			}
			final := finished(t, f)
			if final.Status != "succeeded" {
				t.Fatalf("task failed: %#v", final)
			}
			if mode == "terminal" {
				if final.Result["checks"].([]modelcheck.TerminalContinuityResult)[0].Model != "selected-model" {
					t.Fatal("terminal result lost requested model")
				}
			} else if final.Result["animations"].([]modelcheck.AnimationResult)[0].Model != "selected-model" {
				t.Fatal("result lost requested model")
			}
		})
	}
}

func TestSelectedScheduleModelSurvivesRestartAndExecution(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "schedule-model" {
			t.Errorf("schedule model overwritten: %q", body.Model)
		}
		fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
	})
	_, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{AccountID: "1", Enabled: true, Model: "schedule-model", IntervalMinutes: 10, TimeoutSeconds: 5}, "test")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	schedules := restarted.AnimationSchedules()
	if schedules[0].Model != "schedule-model" {
		t.Fatalf("saved model overwritten: %q", schedules[0].Model)
	}
	runner := &deferredRunner{}
	restarted.UseTaskRunner(runner)
	restarted.RunDueAnimations(context.Background(), time.Now().Add(time.Hour))
	if len(runner.runs) != 1 {
		t.Fatal("schedule not dispatched")
	}
	runner.runs[0](context.Background())
	if final := finished(t, f); final.Result["animations"].([]modelcheck.AnimationResult)[0].Model != "schedule-model" {
		t.Fatal("schedule result lost requested model")
	}
}

func TestSelectedManagedModelSurvivesSaveEditAndRestart(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("saving must not generate") })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	value := managedTask()
	value.Model = " chosen-model "
	plans, err := f.service.SaveDetectionTask(context.Background(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].Model != "chosen-model" {
		t.Fatalf("save overwrote model: %q", plans[0].Model)
	}
	value = plans[0].DetectionTask
	value.Model = "edited-model"
	if _, err := f.service.SaveDetectionTask(context.Background(), value, "test"); err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.DetectionTasks()[0].Model != "edited-model" {
		t.Fatal("restart overwrote model")
	}
}
