package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestDetectionTaskRunsOnlySelectedStagesAfterRestart(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		animation, precheck, terminal bool
		stages                        []string
	}{
		{"animation only", true, false, false, []string{"animation"}},
		{"precheck only", false, true, false, []string{"precheck"}},
		{"terminal only", false, false, true, []string{"terminal"}},
		{"precheck and terminal", false, true, true, []string{"precheck", "terminal"}},
		{"precheck and animation", true, true, false, []string{"precheck", "animation"}},
		{"terminal and animation", true, false, true, []string{"terminal", "animation"}},
		{"all stages", true, true, true, []string{"precheck", "terminal", "animation"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requested []string
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input string `json:"input"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				stage, answer := "animation", fixtureSVG
				if strings.Contains(body.Input, "exec_command") {
					stage, answer = "terminal", `{"tool":"exec_command","command":"git status --short"}`
				} else if strings.Contains(body.Input, "圆形苹果") {
					stage, answer = "precheck", "21"
				}
				requested = append(requested, stage)
				fmt.Fprintf(w, `{"output_text":%q}`, answer)
			})
			f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
			data, _ := json.Marshal(managedTask())
			var fields map[string]any
			json.Unmarshal(data, &fields)
			fields["animation"], fields["precheck"], fields["terminal"] = tc.animation, tc.precheck, tc.terminal
			fields["terminal_rounds"], fields["automatic"] = 1, true
			data, _ = json.Marshal(fields)
			var value modelcheck.DetectionTask
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.SaveDetectionTask(context.Background(), value, "test"); err != nil {
				t.Fatal(err)
			}
			restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
			if err != nil {
				t.Fatal(err)
			}
			runner := &deferredRunner{}
			restarted.UseTaskRunner(runner)
			next, err := time.Parse(time.RFC3339Nano, restarted.DetectionTasks()[0].NextAt)
			if err != nil {
				t.Fatal(err)
			}
			restarted.RunDueDetectionTasks(context.Background(), next)
			if len(runner.runs) != 1 {
				t.Fatal("automatic run missing after restart")
			}
			runner.runs[0](context.Background())
			result := finished(t, f)
			if !slices.Equal(requested, tc.stages) {
				t.Fatalf("requested stages = %v, want %v", requested, tc.stages)
			}
			if result.Status != "succeeded" || result.Progress != 100 || result.Result["total"] != len(tc.stages) || result.Result["completed"] != len(tc.stages) {
				t.Fatalf("unexpected progress: %#v", result)
			}
			animations := result.Result["animations"].([]modelcheck.AnimationResult)
			wantAnimations := len(tc.stages)
			if tc.terminal {
				wantAnimations--
			}
			if len(animations) != wantAnimations {
				t.Fatalf("unselected results: %v", animations)
			}
		})
	}
}

func TestDetectionTaskRejectsExplicitlyEmptyStagesWithoutSaving(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid task generated") })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	data, _ := json.Marshal(managedTask())
	var fields map[string]any
	json.Unmarshal(data, &fields)
	fields["animation"], fields["precheck"], fields["terminal"] = false, false, false
	data, _ = json.Marshal(fields)
	var value modelcheck.DetectionTask
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SaveDetectionTask(context.Background(), value, "test"); err == nil || !strings.Contains(err.Error(), "至少选择一项") {
		t.Fatalf("empty selection accepted: %v", err)
	}
	if len(f.service.DetectionTasks()) != 0 || len(f.catalog.plans) != 0 {
		t.Fatal("invalid task persisted")
	}
}

func TestLegacyDetectionTaskWithoutAnimationFieldKeepsAnimationOnRestart(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) {})
	f.catalog.plans = []byte(`[{"id":"legacy","version":3,"name":"旧动画任务","group_ids":["7"],"model":"test-model","precheck":false,"terminal":false,"terminal_rounds":3,"schedule_type":"interval","interval_minutes":60,"timeout_seconds":5}]`)
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(restarted.DetectionTasks())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"animation":true`) {
		t.Fatalf("legacy animation not preserved: %s", raw)
	}
}
