package modelcheck_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestSchedulesKeepAnimationAndPrecheckIndependent(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("settings must not generate") })
	for _, mode := range []string{"animation", "precheck"} {
		_, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{AccountID: "1", Mode: mode, Enabled: true, Model: "test-model", IntervalMinutes: 60, TimeoutSeconds: 5}, "test")
		if err != nil {
			t.Fatal(err)
		}
	}
	views := f.service.AnimationSchedules()
	if len(views) != 2 {
		t.Fatalf("independent schedules lost: %+v", views)
	}
	stop := views[0].AnimationSchedule
	stop.Enabled = false
	if _, err := f.service.SaveAnimationSchedule(context.Background(), stop, "test"); err != nil {
		t.Fatal(err)
	}
	views = f.service.AnimationSchedules()
	if views[0].Enabled || !views[1].Enabled {
		t.Fatalf("disabling one mode changed the other: %+v", views)
	}
}

func TestDailyScheduleUsesBeijingWallTimeAndSurvivesRestart(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("settings must not generate") })
	var schedule modelcheck.AnimationSchedule
	if err := json.Unmarshal([]byte(`{"account_id":"1","mode":"animation","enabled":true,"model":"test-model","interval_minutes":60,"timeout_seconds":5,"schedule_type":"daily","daily_time":"09:35","timezone":"Asia/Shanghai"}`), &schedule); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SaveAnimationSchedule(context.Background(), schedule, "test"); err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	view := restarted.AnimationSchedules()[0]
	next, err := time.Parse(time.RFC3339Nano, view.NextAt)
	if err != nil {
		t.Fatal(err)
	}
	zone, _ := time.LoadLocation("Asia/Shanghai")
	if next.In(zone).Format("15:04") != "09:35" || !next.After(time.Now()) {
		t.Fatalf("wrong daily deadline: %s", view.NextAt)
	}
}

func runSplitSchedules(t *testing.T, f *fixture, service *modelcheck.Service) []modelcheck.AnimationResult {
	t.Helper()
	runner := &deferredRunner{}
	service.UseTaskRunner(runner)
	views := service.AnimationSchedules()
	if len(views) != 2 || views[0].Mode != "animation" || views[1].Mode != "precheck" {
		t.Fatalf("legacy combination not split: %+v", views)
	}
	var due time.Time
	for _, view := range views {
		next, err := time.Parse(time.RFC3339Nano, view.NextAt)
		if err != nil {
			t.Fatal(err)
		}
		if next.After(due) {
			due = next
		}
	}
	rows := []modelcheck.AnimationResult{}
	for index := range 2 {
		service.RunDueAnimations(context.Background(), due)
		if len(runner.runs) != index+1 {
			t.Fatalf("same-account plans overlapped or were lost: %d", len(runner.runs))
		}
		runner.runs[index](context.Background())
		rows = append(rows, finished(t, f).Result["animations"].([]modelcheck.AnimationResult)...)
	}
	return rows
}
