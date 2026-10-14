package modelcheck_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func promptTemplates() modelcheck.AnimationPromptSettings {
	return modelcheck.AnimationPromptSettings{DefaultID: "whale", Templates: []modelcheck.AnimationPromptTemplate{
		{ID: "bird", Name: "鹈鹕", Prompt: "生成鹈鹕动画"},
		{ID: "whale", Name: "鲸鱼", Prompt: "生成鲸鱼动画"},
	}}
}

func TestAnimationPromptTemplatesPersistDefaultAndResolveSelection(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected generation") })
	saved, err := f.service.SaveAnimationPrompt(t.Context(), promptTemplates(), "test")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.AnimationPromptSettings(); !reflect.DeepEqual(got, saved) {
		t.Fatalf("not restored: %+v", got)
	}
	for _, tc := range []struct{ id, want string }{{"", "生成鲸鱼动画"}, {"bird", "生成鹈鹕动画"}, {"whale", "生成鲸鱼动画"}} {
		got, err := restarted.ResolveAnimationPrompt(tc.id)
		if err != nil || got != tc.want {
			t.Fatalf("selection %q = %q, %v", tc.id, got, err)
		}
	}
	if _, err := restarted.ResolveAnimationPrompt("deleted"); err == nil {
		t.Fatal("missing selection silently fell back")
	}
	// Neither caller-owned input nor returned snapshots may mutate live settings.
	saved.Templates[0].Prompt = "changed"
	snapshot := restarted.AnimationPromptSettings()
	snapshot.Templates[0].Prompt = "changed"
	if got, _ := restarted.ResolveAnimationPrompt("bird"); got != "生成鹈鹕动画" {
		t.Fatal("settings alias leaked")
	}
}

func TestAnimationPromptTemplatesRejectInvalidCollectionsWithoutChangingSettings(t *testing.T) {
	cases := []struct {
		name   string
		change func(*modelcheck.AnimationPromptSettings)
	}{
		{"empty", func(v *modelcheck.AnimationPromptSettings) { v.Templates = []modelcheck.AnimationPromptTemplate{} }},
		{"missing default", func(v *modelcheck.AnimationPromptSettings) { v.DefaultID = "missing" }},
		{"duplicate ID", func(v *modelcheck.AnimationPromptSettings) { v.Templates[1].ID = "bird" }},
		{"duplicate name", func(v *modelcheck.AnimationPromptSettings) { v.Templates[1].Name = " 鹈鹕 " }},
		{"blank name", func(v *modelcheck.AnimationPromptSettings) { v.Templates[1].Name = " " }},
		{"long name", func(v *modelcheck.AnimationPromptSettings) { v.Templates[1].Name = strings.Repeat("名", 81) }},
		{"blank prompt", func(v *modelcheck.AnimationPromptSettings) { v.Templates[1].Prompt = " " }},
		{"long prompt", func(v *modelcheck.AnimationPromptSettings) { v.Templates[1].Prompt = strings.Repeat("词", 10001) }},
		{"control character", func(v *modelcheck.AnimationPromptSettings) { v.Templates[1].Prompt = "bad\x00" }},
		{"too many", func(v *modelcheck.AnimationPromptSettings) {
			v.Templates = make([]modelcheck.AnimationPromptTemplate, 33)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected generation") })
			before := f.service.AnimationPromptSettings()
			value := promptTemplates()
			tc.change(&value)
			if _, err := f.service.SaveAnimationPrompt(t.Context(), value, "test"); err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if !reflect.DeepEqual(f.service.AnimationPromptSettings(), before) {
				t.Fatal("invalid write changed settings")
			}
		})
	}
}

func TestAnimationPromptLegacyConfigurationKeepsExistingContentAndVersion(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected generation") })
	f.catalog.promptRaw = []byte(`{"prompt":"原有动画提示词","version":7}`)
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	got := restarted.AnimationPromptSettings()
	if got.Version != 7 || got.Prompt != "原有动画提示词" || len(got.Templates) != 1 || got.DefaultID != got.Templates[0].ID {
		t.Fatalf("legacy lost: %+v", got)
	}
}

func TestAnimationPromptLegacySavePreservesOtherTemplatesAndDefaultSelection(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected generation") })
	saved, err := f.service.SaveAnimationPrompt(t.Context(), promptTemplates(), "test")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.service.SaveAnimationPrompt(t.Context(), modelcheck.AnimationPromptSettings{Prompt: "新版鲸鱼动画", Version: saved.Version}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if updated.DefaultID != "whale" || len(updated.Templates) != 2 || updated.Templates[0] != saved.Templates[0] || updated.Prompt != "新版鲸鱼动画" {
		t.Fatalf("legacy write lost configuration: %+v", updated)
	}
}

func TestAnimationPromptDefaultSwitchAffectsOnlyNewResolutionAndFailedWritesKeepCollection(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected generation") })
	saved, err := f.service.SaveAnimationPrompt(t.Context(), promptTemplates(), "test")
	if err != nil {
		t.Fatal(err)
	}
	value := saved
	value.DefaultID = "bird"
	switched, err := f.service.SaveAnimationPrompt(t.Context(), value, "test")
	if err != nil || switched.Prompt != "生成鹈鹕动画" {
		t.Fatalf("default not switched: %+v %v", switched, err)
	}
	if _, err := f.service.SaveAnimationPrompt(t.Context(), saved, "test"); err == nil {
		t.Fatal("stale collection accepted")
	}
	f.catalog.failSave = true
	value = switched
	value.DefaultID = "whale"
	if _, err := f.service.SaveAnimationPrompt(t.Context(), value, "test"); err == nil {
		t.Fatal("write failure ignored")
	}
	if !reflect.DeepEqual(f.service.AnimationPromptSettings(), switched) {
		t.Fatal("failed collection write changed live state")
	}
}
