package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func (c *catalog) LoadAnimationQualitySettings(context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.qualityRaw...), nil
}
func (c *catalog) SaveAnimationQualitySettings(_ context.Context, raw []byte, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSave {
		return errors.New("storage unavailable")
	}
	c.qualityRaw = append([]byte(nil), raw...)
	return nil
}
func TestAutoQualityGeneratesReasonAndPreservesGeneration(t *testing.T) {
	calls := 0
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		output := htmlAnimation
		if body.Model == "review-model" {
			if !strings.Contains(body.Input, "original_prompt") || !strings.Contains(body.Input, "source") {
				t.Error("review missing original source or prompt")
			}
			output = `{"label":"suspicious","reason":"脚踏连接缺失，需要人工查看动画确认","basis":[{"requirement":"踩踏动作","evidence":"<circle class=\"wheel\" r=\"20\" cx=\"100\" cy=\"100\"/>","assessment":"原文只有轮圈，未体现脚踏连接和踩踏联动。"}],"issues":[{"severity":"major","scope":"motion","evidence":"<circle class=\"wheel\" r=\"20\" cx=\"100\" cy=\"100\"/>","impact":"只有轮圈旋转，没有骑车主体和踩踏联动"}]}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": output, "model": body.Model})
	})
	if _, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review-model", TimeoutSeconds: 5}, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
	raw, _ := json.Marshal(row.Quality)
	var q map[string]any
	_ = json.Unmarshal(raw, &q)
	if calls != 2 || row.Status != "succeeded" || row.Source != htmlAnimation || q["status"] != "suspicious" || !strings.Contains(row.Quality.Explanation, "脚踏连接缺失，需要人工查看动画确认") || len(row.Quality.Basis) != 1 || q["source"] != "model_review" {
		t.Fatalf("calls=%d row=%+v quality=%s", calls, row, raw)
	}
	persisted, err := f.tasks.Get(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := json.Marshal(persisted.Result)
	if !strings.Contains(string(saved), "model_review") {
		t.Fatal("automatic annotation not persisted")
	}
}
func TestAutoQualityInvalidResponseDoesNotRetryGeneration(t *testing.T) {
	for _, output := range []string{`{"label":"passed","reason":"ok"}`, `{"label":"normal","reason":""}`, `{"label":"normal","reason":"ok","extra":true}`, fixtureSecret} {
		t.Run(output, func(t *testing.T) {
			calls := 0
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				calls++
				value := htmlAnimation
				if calls > 1 {
					value = output
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"output_text": value})
			})
			_, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if calls != 2 || row.Status != "succeeded" || row.Quality.Status != "unknown" {
				t.Fatalf("calls=%d row=%+v", calls, row)
			}
			raw, _ := json.Marshal(row)
			if strings.Contains(string(raw), fixtureSecret) {
				t.Fatal("secret persisted")
			}
		})
	}
}
func TestQualitySettingsPersistRejectStaleAndValidateReviewer(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("saving settings must not call upstream") })
	v := modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 30}
	saved, err := f.service.SaveAnimationQualitySettings(t.Context(), v, "operator")
	if err != nil || saved.Version != 1 {
		t.Fatalf("%+v %v", saved, err)
	}
	if _, err = f.service.SaveAnimationQualitySettings(t.Context(), v, "operator"); !errors.Is(err, modelcheck.ErrQualitySettingsConflict) {
		t.Fatal(err)
	}
	reopened, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil || reopened.AnimationQualitySettings() != saved {
		t.Fatalf("settings not restored: %v", err)
	}
	v.Version = 1
	v.AccountID = "999"
	if _, err = f.service.SaveAnimationQualitySettings(t.Context(), v, "operator"); !errors.Is(err, modelcheck.ErrQualitySettings) {
		t.Fatal(err)
	}
	f.catalog.excluded = map[string]bool{"1": true}
	v.AccountID = "1"
	if _, err = f.service.SaveAnimationQualitySettings(t.Context(), v, "operator"); !errors.Is(err, modelcheck.ErrQualitySettings) {
		t.Fatal(err)
	}
	v.Enabled = false
	if _, err = f.service.SaveAnimationQualitySettings(t.Context(), v, "operator"); err != nil {
		t.Fatal(err)
	}
}

func TestAutoQualityHTTPFailurePreservesSuccessfulAnimation(t *testing.T) {
	calls := 0
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": htmlAnimation})
	})
	_, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "succeeded" || row.Quality.Reason != "review_failed" || calls != 2 {
		t.Fatalf("%+v calls=%d", row, calls)
	}
}
func TestAutoQualitySettingsChangedDuringReviewDiscardsOldPrediction(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		output := htmlAnimation
		if body.Model == "review" {
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			output = `{"label":"normal","reason":"结构完整","basis":[{"requirement":"车轮转动","evidence":"@keyframes spin{to{transform:rotate(360deg)}}","assessment":"CSS关键帧提供车轮转动实现。"}],"issues":[]}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": output})
	})
	settings, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-t.Context().Done():
		t.Fatal("review not reached")
	}
	settings.Enabled = false
	if _, err = f.service.SaveAnimationQualitySettings(t.Context(), settings, "operator"); err != nil {
		t.Fatal(err)
	}
	close(release)
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "succeeded" || row.Quality.Reason != "review_settings_changed" || row.Quality.Explanation != "" {
		t.Fatalf("%+v", row)
	}
}
