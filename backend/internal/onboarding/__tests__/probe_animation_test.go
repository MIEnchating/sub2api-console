package onboarding_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type animationDependencies struct {
	modelcheck.TaskStore
	modelcheck.CredentialStore
	modelcheck.AccountCatalog
	modelcheck.KeyRevealer
	prompt []byte
}

func (d *animationDependencies) ListBySkill(context.Context, string, int) ([]taskstore.Task, error) {
	return nil, nil
}

func (d *animationDependencies) LoadAnimationPrompt(context.Context) ([]byte, error) {
	return d.prompt, nil
}
func (d *animationDependencies) SaveAnimationPrompt(_ context.Context, raw []byte, _ string) error {
	d.prompt = raw
	return nil
}

func TestProbeAnimationUsesFrozenPromptAndCleansTemporaryKey(t *testing.T) {
	for _, scenario := range []struct{ name, output, status string }{
		{"success", "<!DOCTYPE html><html><body><svg><circle r='10'/></svg></body></html>", "succeeded"},
		{"unsafe resource", "<!DOCTYPE html><html><body><img src='https://outside.test/pixel'></body></html>", "failed"},
		{"credential echo", testKey, "failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+testKey {
					t.Error("missing bound credential")
				}
				var body struct {
					Model, Input string
					Reasoning    struct{ Effort string }
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != "gpt-6-astra" || body.Input != "启动时的动画提示词" || body.Reasoning.Effort != "low" {
					t.Errorf("unexpected generation: %+v", body)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"model": "gpt-6-astra", "output_text": scenario.output, "usage": map[string]int{"input_tokens": 12, "output_tokens": 34}})
			}))
			t.Cleanup(gateway.Close)
			service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
			deps := &animationDependencies{}
			animations, err := modelcheck.New(deps, deps, deps, deps)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := animations.SaveAnimationPrompt(t.Context(), modelcheck.AnimationPromptSettings{Prompt: "启动时的动画提示词"}, "test")
			if err != nil {
				t.Fatal(err)
			}
			service.UseAnimationService(animations)
			if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "gpt-6-astra", "animation"); err != nil {
				t.Fatal(err)
			}
			if _, err := animations.SaveAnimationPrompt(t.Context(), modelcheck.AnimationPromptSettings{Prompt: "后续修改的提示词", Version: saved.Version}, "test"); err != nil {
				t.Fatal(err)
			}
			runner.run(t.Context())
			if tasks.final.Status != scenario.status {
				t.Fatalf("task=%+v", tasks.final)
			}
			if len(keys.created) != 1 || len(keys.deleted) != 1 || keys.created[0] != keys.deleted[0] {
				t.Fatalf("temporary key not cleaned: %+v", keys)
			}
			raw, _ := json.Marshal(tasks.final)
			if strings.Contains(string(raw), testKey) {
				t.Fatal("credential leaked into task")
			}
			result, ok := tasks.final.Result["probe_result"].(onboarding.ProbeResult)
			if !ok || result.Animation == nil {
				t.Fatalf("missing animation result: %+v", tasks.final.Result)
			}
			if scenario.status == "succeeded" && (result.Animation.HTML == "" || result.Animation.Prompt != "启动时的动画提示词" || result.Animation.Usage == nil) {
				t.Fatalf("incomplete animation: %+v", result.Animation)
			}
			if scenario.status == "failed" && result.Animation.HTML != "" {
				t.Fatal("unsafe animation was displayed")
			}
		})
	}
}

func TestProbeAnimationUnavailableDoesNotCreateTemporaryKey(t *testing.T) {
	service, keys, _, _ := modelOptionsFixture(t, "http://127.0.0.1:1", false)
	if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "gpt-6-astra", "animation"); err == nil {
		t.Fatal("unavailable animation service accepted")
	}
	if len(keys.created) != 0 {
		t.Fatal("created a key before validating availability")
	}
}

func TestProbeAnimationCancellationCleansTemporaryKey(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cancel()
	}))
	t.Cleanup(gateway.Close)
	service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
	deps := &animationDependencies{}
	animations, err := modelcheck.New(deps, deps, deps, deps)
	if err != nil {
		t.Fatal(err)
	}
	service.UseAnimationService(animations)
	if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "gpt-6-astra", "animation"); err != nil {
		t.Fatal(err)
	}
	runner.run(ctx)
	if tasks.final.Status != "cancelled" {
		t.Fatalf("status=%s", tasks.final.Status)
	}
	if len(keys.created) != 1 || len(keys.deleted) != 1 {
		t.Fatalf("cancel left temporary key: %+v", keys)
	}
}

func TestProbeAnimationPreservesExistingKeyAndReportsCleanupFailure(t *testing.T) {
	for _, existing := range []bool{true, false} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"output_text": "<!DOCTYPE html><html><body><svg><circle r='10'/></svg></body></html>"})
			}))
			t.Cleanup(gateway.Close)
			service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, existing)
			if !existing {
				keys.deleteErr = errors.New("isolated cleanup failure")
			}
			deps := &animationDependencies{}
			animations, err := modelcheck.New(deps, deps, deps, deps)
			if err != nil {
				t.Fatal(err)
			}
			service.UseAnimationService(animations)
			if _, err := service.EnqueueProbe(t.Context(), "probe", "options.test", "6", "gpt-6-astra", "animation"); err != nil {
				t.Fatal(err)
			}
			runner.run(t.Context())
			if existing {
				if tasks.final.Status != "succeeded" || len(keys.created) != 0 || len(keys.deleted) != 0 {
					t.Fatalf("existing key changed: task=%+v keys=%+v", tasks.final, keys)
				}
			} else if tasks.final.Status != "succeeded" || tasks.final.Result["probe_result"] == nil {
				t.Fatalf("successful animation must survive cleanup failure: %+v", tasks.final)
			}
		})
	}
}

func TestProbeSelectedAnimationPromptIsFrozenWithoutChangingDefault(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Input != "本次选择的鲸鱼动画" {
			t.Errorf("prompt=%q", body.Input)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": "<!DOCTYPE html><html><body><svg><circle r='10'/></svg></body></html>"})
	}))
	t.Cleanup(gateway.Close)
	service, keys, tasks, runner := modelOptionsFixture(t, gateway.URL, false)
	deps := &animationDependencies{}
	animations, err := modelcheck.New(deps, deps, deps, deps)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := animations.SaveAnimationPrompt(t.Context(), modelcheck.AnimationPromptSettings{DefaultID: "bird", Templates: []modelcheck.AnimationPromptTemplate{
		{ID: "bird", Name: "鹈鹕", Prompt: "默认动画"},
		{ID: "whale", Name: "鲸鱼", Prompt: "本次选择的鲸鱼动画"},
	}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	service.UseAnimationService(animations)
	if _, err := service.EnqueueProbeWithPrompt(t.Context(), "probe", "options.test", "6", "gpt-6-astra", "animation", "whale"); err != nil {
		t.Fatal(err)
	}
	if animations.AnimationPromptSettings().DefaultID != "bird" {
		t.Fatal("selection changed default")
	}
	saved.Templates = saved.Templates[:1]
	if _, err := animations.SaveAnimationPrompt(t.Context(), saved, "test"); err != nil {
		t.Fatal(err)
	}
	runner.run(t.Context())
	if tasks.final.Status != "succeeded" || len(keys.deleted) != 1 {
		t.Fatalf("task=%+v", tasks.final)
	}
	result := tasks.final.Result["probe_result"].(onboarding.ProbeResult)
	if result.Animation.Prompt != "本次选择的鲸鱼动画" {
		t.Fatal("snapshot lost")
	}
	if _, err := service.EnqueueProbeWithPrompt(t.Context(), "probe", "options.test", "6", "gpt-6-astra", "animation", "whale"); err == nil {
		t.Fatal("deleted prompt accepted")
	}
	if len(keys.created) != 1 {
		t.Fatal("invalid selection created a key")
	}
}
