package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDetectionTimeoutDefaultsAndPersistence(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) {})
	for _, mode := range []string{"animation", "precheck", "review"} {
		initial, err := f.service.DetectionTimeout(mode)
		if err != nil || initial.TimeoutSeconds != 120 {
			t.Fatalf("%s: %+v %v", mode, initial, err)
		}
	}
	saved, err := f.service.SaveDetectionTimeout(t.Context(), "animation", modelcheck.DetectionTimeout{TimeoutSeconds: 75}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.DetectionTimeout("animation")
	if err != nil || got != saved || got.Version != 1 {
		t.Fatalf("not persisted %+v %v", got, err)
	}
	other, _ := reopened.DetectionTimeout("precheck")
	if other.TimeoutSeconds != 120 {
		t.Fatal("animation changed precheck")
	}
}

func TestDetectionTimeoutRejectsInvalidAndStaleWrites(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) {})
	for _, mode := range []string{"animation", "precheck", "review"} {
		t.Run(mode, func(t *testing.T) {
			saved, err := f.service.SaveDetectionTimeout(t.Context(), mode, modelcheck.DetectionTimeout{TimeoutSeconds: 60}, "operator")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.service.SaveDetectionTimeout(t.Context(), mode, modelcheck.DetectionTimeout{TimeoutSeconds: 90}, "operator"); !errors.Is(err, modelcheck.ErrDetectionTimeoutConflict) {
				t.Fatalf("stale save: %v", err)
			}
			for _, v := range []int{0, 4, 121} {
				if _, err = f.service.SaveDetectionTimeout(t.Context(), mode, modelcheck.DetectionTimeout{TimeoutSeconds: v, Version: saved.Version}, "operator"); !errors.Is(err, modelcheck.ErrDetectionTimeout) {
					t.Fatalf("accepted %d: %v", v, err)
				}
			}
		})
	}
	if _, err := f.service.DetectionTimeout("invalid"); !errors.Is(err, modelcheck.ErrDetectionTimeout) {
		t.Fatal(err)
	}
}

func TestDetectionTimeoutStorageFailurePreservesAppliedValue(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) {})
	f.catalog.failSave = true
	for _, mode := range []string{"animation", "review"} {
		if _, err := f.service.SaveDetectionTimeout(t.Context(), mode, modelcheck.DetectionTimeout{TimeoutSeconds: 60}, "operator"); err == nil {
			t.Fatal("storage error ignored")
		}
		got, _ := f.service.DetectionTimeout(mode)
		if got.TimeoutSeconds != 120 || got.Version != 0 {
			t.Fatalf("failed save changed state: %+v", got)
		}
	}
}

func TestReviewTimeoutSavePreservesReviewerAndExistingSavedValue(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("timeout setting must not call upstream") })
	initial, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review-model", TimeoutSeconds: 60}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := reopened.DetectionTimeout("review")
	if got.TimeoutSeconds != 60 {
		t.Fatal("existing value replaced by default")
	}
	got.TimeoutSeconds = 90
	saved, err := reopened.SaveDetectionTimeout(t.Context(), "review", got, "operator")
	if err != nil {
		t.Fatal(err)
	}
	initial.TimeoutSeconds = 90
	initial.Version = saved.Version
	if reopened.AnimationQualitySettings() != initial {
		t.Fatalf("reviewer changed: %+v", reopened.AnimationQualitySettings())
	}
}

func TestSavedDetectionTimeoutCapturedWhenRequestOmitsTimeout(t *testing.T) {
	for _, mode := range []string{"animation", "precheck"} {
		for _, explicit := range []int{0, 30} {
			t.Run(mode+"/explicit="+strconv.Itoa(explicit), func(t *testing.T) {
				f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewEncoder(w).Encode(map[string]any{"output_text": htmlAnimation})
				})
				if _, err := f.service.SaveDetectionTimeout(t.Context(), mode, modelcheck.DetectionTimeout{TimeoutSeconds: 75}, "operator"); err != nil {
					t.Fatal(err)
				}
				req := request("1")
				req.Mode = mode
				req.TimeoutSeconds = explicit
				task, err := f.service.EnqueueAnimation(t.Context(), req)
				if err != nil {
					t.Fatal(err)
				}
				var recovered struct {
					Request modelcheck.AnimationRequest `json:"request"`
				}
				if err = taskstore.RecoveryInput(task, &recovered); err != nil {
					t.Fatal(err)
				}
				want := 75
				if explicit != 0 {
					want = explicit
				}
				if recovered.Request.TimeoutSeconds != want {
					t.Fatalf("captured timeout %d, want %d", recovered.Request.TimeoutSeconds, want)
				}
			})
		}
	}
}

func (c *catalog) LoadDetectionTimeouts(context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.timeoutRaw...), nil
}
func (c *catalog) SaveDetectionTimeouts(_ context.Context, raw []byte, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSave {
		return errors.New("storage failure")
	}
	c.timeoutRaw = append([]byte(nil), raw...)
	return nil
}
