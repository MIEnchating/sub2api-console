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

func TestDailyMultipleTimesDispatchEachTimeAndWrapToTomorrow(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG) })
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	var value modelcheck.AnimationSchedule
	if err := json.Unmarshal([]byte(`{"account_id":"1","enabled":true,"model":"test-model","schedule_type":"daily","daily_times":["23:45","09:00","14:00"],"timezone":"Asia/Shanghai","timeout_seconds":5}`), &value); err != nil {
		t.Fatal(err)
	}
	views, err := f.service.SaveAnimationSchedule(context.Background(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	zone, _ := time.LoadLocation("Asia/Shanghai")
	due, err := time.Parse(time.RFC3339Nano, views[0].NextAt)
	if err != nil {
		t.Fatal(err)
	}
	for index := range 3 {
		f.service.RunDueAnimations(context.Background(), due.Add(-time.Second))
		if len(runner.runs) != index {
			t.Fatal("daily time ran early or repeated")
		}
		f.service.RunDueAnimations(context.Background(), due)
		if len(runner.runs) != index+1 {
			t.Fatal("daily time not dispatched")
		}
		runner.runs[index](context.Background())
		finished(t, f)
		next, _ := time.Parse(time.RFC3339Nano, f.service.AnimationSchedules()[0].NextAt)
		want := map[string]time.Duration{"09:00": 5 * time.Hour, "14:00": 9*time.Hour + 45*time.Minute, "23:45": 9*time.Hour + 15*time.Minute}[due.In(zone).Format("15:04")]
		if want == 0 || !next.Equal(due.Add(want)) {
			t.Fatalf("next time after %s: %s", due, next)
		}
		due = next
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(restarted.AnimationSchedules()[0])
	var stored struct {
		DailyTimes []string `json:"daily_times"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(stored.DailyTimes) != "[09:00 14:00 23:45]" {
		t.Fatalf("times not retained: %s", raw)
	}
}

func TestDailyMultipleTimesRejectInvalidOrAmbiguousPlans(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("settings generated") })
	for _, fields := range []string{
		`"daily_times":[]`, `"daily_times":["09:00","09:00"]`,
		`"daily_times":["09:00","25:00"]`, `"daily_time":"09:00","daily_times":["14:00"]`,
	} {
		t.Run(fields, func(t *testing.T) {
			var value modelcheck.AnimationSchedule
			if err := json.Unmarshal([]byte(`{"account_id":"1","enabled":true,"model":"test-model","schedule_type":"daily","timezone":"Asia/Shanghai","timeout_seconds":5,`+fields+`}`), &value); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.SaveAnimationSchedule(context.Background(), value, "test"); err == nil {
				t.Fatal("invalid daily times accepted")
			}
			if len(f.service.AnimationSchedules()) != 0 {
				t.Fatal("invalid plan persisted")
			}
		})
	}
}
