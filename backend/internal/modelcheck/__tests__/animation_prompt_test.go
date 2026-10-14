package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestManagedAnimationSendsSavedPromptAndRecordsIt(t *testing.T) {
	const prompt = "生成蓝色鲸鱼动画，只输出完整 HTML。"
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Input != prompt {
			t.Errorf("prompt = %q, want %q", body.Input, prompt)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": htmlAnimation})
	})
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	value := managedTask()
	value.Precheck = false
	value.Prompt = "旧任务提示词"
	if _, err := f.service.SaveAnimationPrompt(context.Background(), modelcheck.AnimationPromptSettings{Prompt: prompt}, "test"); err != nil {
		t.Fatal(err)
	}
	plans, err := f.service.SaveDetectionTask(context.Background(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Prompt != prompt {
		t.Fatalf("recorded prompt = %q", row.Prompt)
	}
}

func (c *catalog) LoadAnimationPrompt(context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.promptRaw...), nil
}
func (c *catalog) SaveAnimationPrompt(_ context.Context, raw []byte, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSave {
		return fmt.Errorf("isolated storage failure")
	}
	c.promptRaw = append([]byte(nil), raw...)
	return nil
}

func TestAnimationPromptPersistsAndRejectsStaleInvalidOrFailedWrites(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected generation") })
	initial := f.service.AnimationPromptSettings()
	saved, err := f.service.SaveAnimationPrompt(context.Background(), modelcheck.AnimationPromptSettings{Prompt: "  蓝色鲸鱼  ", Version: initial.Version}, "test")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.AnimationPromptSettings(); !reflect.DeepEqual(got, saved) || got.Prompt != "蓝色鲸鱼" {
		t.Fatalf("not restored: %+v", got)
	}
	for _, value := range []modelcheck.AnimationPromptSettings{
		{Prompt: "stale", Version: initial.Version},
		{Prompt: " ", Version: saved.Version},
		{Prompt: strings.Repeat("中", 10001), Version: saved.Version},
		{Prompt: "bad\x00", Version: saved.Version},
	} {
		if _, err := f.service.SaveAnimationPrompt(context.Background(), value, "test"); err == nil {
			t.Fatalf("invalid update accepted: version=%d", value.Version)
		}
	}
	f.catalog.failSave = true
	if _, err := f.service.SaveAnimationPrompt(context.Background(), modelcheck.AnimationPromptSettings{Prompt: "failed", Version: saved.Version}, "test"); err == nil {
		t.Fatal("write failure ignored")
	}
	if got := f.service.AnimationPromptSettings(); !reflect.DeepEqual(got, saved) {
		t.Fatalf("failed update changed state: %+v", got)
	}
}

func TestScheduledAnimationUsesSharedPromptInsteadOfLegacyOverride(t *testing.T) {
	const prompt = "统一动画提示词"
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Input != prompt {
			t.Errorf("sent prompt = %q", body.Input)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": htmlAnimation})
	})
	_, err := f.service.SaveAnimationPrompt(context.Background(), modelcheck.AnimationPromptSettings{Prompt: prompt}, "test")
	if err != nil {
		t.Fatal(err)
	}
	schedules, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{AccountID: "1", Model: "test-model", Prompt: "旧账号提示词", Enabled: true, IntervalMinutes: 1, TimeoutSeconds: 5}, "test")
	if err != nil {
		t.Fatal(err)
	}
	next, err := time.Parse(time.RFC3339Nano, schedules[0].NextAt)
	if err != nil {
		t.Fatal(err)
	}
	f.service.RunDueAnimations(context.Background(), next)
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Prompt != prompt {
		t.Fatalf("recorded prompt = %q", row.Prompt)
	}
}

func TestManagedAnimationRecoveryKeepsPromptSnapshotAfterSharedSettingsChange(t *testing.T) {
	const prompt = "启动时的鲸鱼提示词"
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Input != prompt {
			t.Errorf("resumed prompt = %q", body.Input)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": htmlAnimation})
	})
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	saved, err := f.service.SaveAnimationPrompt(context.Background(), modelcheck.AnimationPromptSettings{Prompt: prompt}, "test")
	if err != nil {
		t.Fatal(err)
	}
	value := managedTask()
	value.Precheck = false
	value.Prompt = "旧计划提示词"
	plans, err := f.service.SaveDetectionTask(context.Background(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	f.service.UseTaskRunner(&deferredRunner{})
	queued, err := f.service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SaveAnimationPrompt(context.Background(), modelcheck.AnimationPromptSettings{Prompt: "新任务提示词", Version: saved.Version}, "test"); err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	restarted.UseTaskRunner(f.runner)
	if err := restarted.ResumeTask(context.Background(), queued); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Prompt != prompt {
		t.Fatalf("recorded resumed prompt = %q", row.Prompt)
	}
}
