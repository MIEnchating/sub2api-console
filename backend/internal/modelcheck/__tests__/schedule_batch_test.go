package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestScheduleBatchConflictAndStorageFailureLeaveEveryPlanUnchanged(t *testing.T) {
	f := setup(t, 2, "openai", func(http.ResponseWriter, *http.Request) { t.Error("saving must not generate") })
	values := []modelcheck.AnimationSchedule{
		{AccountID: "1", Mode: "precheck", Model: "test-model", Enabled: true, IntervalMinutes: 30, TimeoutSeconds: 5},
		{AccountID: "2", Mode: "precheck", Model: "test-model", Enabled: true, IntervalMinutes: 60, TimeoutSeconds: 5},
	}
	before, err := f.service.SaveAnimationSchedules(context.Background(), values, "test")
	if err != nil {
		t.Fatal(err)
	}
	values[0].Version = 1
	values[0].IntervalMinutes = 120
	if _, err := f.service.SaveAnimationSchedules(context.Background(), values, "test"); err == nil {
		t.Fatal("stale batch accepted")
	}
	if !reflect.DeepEqual(before, f.service.AnimationSchedules()) {
		t.Fatal("conflict partially applied")
	}
	values[1].Version = 1
	f.catalog.failSave = true
	if _, err := f.service.SaveAnimationSchedules(context.Background(), values, "test"); err == nil {
		t.Fatal("storage failure ignored")
	}
	if !reflect.DeepEqual(before, f.service.AnimationSchedules()) {
		t.Fatal("storage failure partially applied")
	}
	f.catalog.failSave = false
	values[0].Enabled = false
	values[1].Enabled = false
	views, err := f.service.SaveAnimationSchedules(context.Background(), values, "test")
	if err != nil {
		t.Fatal(err)
	}
	if views[0].Enabled || views[1].Enabled || views[0].Version != 2 || views[1].Version != 2 {
		t.Fatalf("batch disable failed: %+v", views)
	}
}

func TestScheduleBatchInvalidAccountDuplicateAndTimeNeverPartiallySave(t *testing.T) {
	f := setup(t, 2, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid settings generated") })
	valid := modelcheck.AnimationSchedule{AccountID: "1", Model: "test-model", Enabled: true, IntervalMinutes: 60, TimeoutSeconds: 5}
	for _, tc := range []struct {
		name    string
		changes string
	}{
		{"unknown account", `{"account_id":"99"}`},
		{"duplicate account and mode", `{}`},
		{"invalid clock", `{"account_id":"2","schedule_type":"daily","daily_time":"25:00","timezone":"Asia/Shanghai"}`},
		{"unapproved timezone", `{"account_id":"2","schedule_type":"daily","daily_time":"09:00","timezone":"Local"}`},
		{"ambiguous plan", `{"account_id":"2","daily_time":"09:00"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := valid
			if err := json.Unmarshal([]byte(tc.changes), &invalid); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.SaveAnimationSchedules(context.Background(), []modelcheck.AnimationSchedule{valid, invalid}, "test"); err == nil {
				t.Fatal("invalid batch accepted")
			}
			if len(f.service.AnimationSchedules()) != 0 {
				t.Fatal("invalid batch partially persisted")
			}
		})
	}
}

func TestDailySchedulerStartsOnlyAtDeadlineAndDoesNotRepeatOnCompletion(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG) })
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	views, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{AccountID: "1", Enabled: true, Model: "test-model", ScheduleType: "daily", DailyTime: "09:00", Timezone: "Asia/Shanghai", TimeoutSeconds: 5}, "test")
	if err != nil {
		t.Fatal(err)
	}
	due, err := time.Parse(time.RFC3339Nano, views[0].NextAt)
	if err != nil {
		t.Fatal(err)
	}
	f.service.RunDueAnimations(context.Background(), due.Add(-time.Second))
	if len(runner.runs) != 0 {
		t.Fatal("daily run started early")
	}
	f.service.RunDueAnimations(context.Background(), due)
	if len(runner.runs) != 1 {
		t.Fatal("daily run not started")
	}
	runner.runs[0](context.Background())
	finished(t, f)
	next, _ := time.Parse(time.RFC3339Nano, f.service.AnimationSchedules()[0].NextAt)
	if !next.Equal(due.Add(24 * time.Hour)) {
		t.Fatalf("completion changed next daily run: %s", next)
	}
	f.service.RunDueAnimations(context.Background(), due.Add(time.Hour))
	if len(runner.runs) != 1 {
		t.Fatal("daily run repeated")
	}
}

func TestLegacyCombinedPlanLoadsAsTwoIndependentPlansWithoutLosingVersion(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("loading must not generate") })
	raw := []byte(`[{"account_id":"1","mode":"both","precheck_questions":["candy"],"enabled":true,"model":"test-model","interval_minutes":30,"timeout_seconds":5,"version":7}]`)
	if err := f.catalog.SaveAnimationConfiguration(context.Background(), raw, "test"); err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	plans := restarted.AnimationSchedules()
	if len(plans) != 2 || plans[0].Mode != "animation" || plans[1].Mode != "precheck" || plans[0].Version != 7 || plans[1].Version != 7 {
		t.Fatalf("legacy settings lost: %+v", plans)
	}
	if plans[0].IntervalMinutes != 30 || plans[1].IntervalMinutes != 30 || len(plans[0].PrecheckQuestions) != 0 || len(plans[1].PrecheckQuestions) != 1 {
		t.Fatalf("legacy values changed: %+v", plans)
	}
}
