package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

type recheckCatalog struct {
	*catalog
	repo *business.Store
}

func (c recheckCatalog) AnimationQualitySample(ctx context.Context, id string) (*q.Sample, error) {
	return c.repo.AnimationQualitySample(ctx, id)
}
func (c recheckCatalog) SaveAnimationQualitySample(ctx context.Context, v q.Sample, version int) (q.Sample, error) {
	return c.repo.SaveAnimationQualitySample(ctx, v, version)
}
func (c recheckCatalog) AnimationQualitySamples(ctx context.Context, after string, limit int) (q.SamplePage, error) {
	return c.repo.AnimationQualitySamples(ctx, after, limit)
}
func (c recheckCatalog) AnimationQualityAnnotations(ctx context.Context, ids []string) (map[string]q.AnnotationRecord, error) {
	return c.repo.AnimationQualityAnnotations(ctx, ids)
}
func (c recheckCatalog) AnimationQualityRechecks(ctx context.Context, ids []string) (map[string]q.Recheck, error) {
	return c.repo.AnimationQualityRechecks(ctx, ids)
}
func (c recheckCatalog) AnimationQualityRecheck(ctx context.Context, id string) (*q.Recheck, error) {
	return c.repo.AnimationQualityRecheck(ctx, id)
}
func (c recheckCatalog) SaveAnimationQualityRecheck(ctx context.Context, id string, v q.Recheck, version int) (q.Recheck, error) {
	return c.repo.SaveAnimationQualityRecheck(ctx, id, v, version)
}

func recheckFixture(t *testing.T, handler http.HandlerFunc) (*fixture, *modelcheck.Service, recheckCatalog) {
	t.Helper()
	f := setup(t, 1, "openai", handler)
	repo, err := business.Open(filepath.Join(t.TempDir(), "quality.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	c := recheckCatalog{catalog: f.catalog, repo: repo}
	service, err := modelcheck.New(f.tasks, credentials{}, c, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	service.UseTaskRunner(f.runner)
	// Manual rechecks work even when automatic review after generation is off.
	if _, err = service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{AccountID: "1", Model: "review-model", TimeoutSeconds: 5}, "operator"); err != nil {
		t.Fatal(err)
	}
	original := qualityTask()
	original.Status = "running"
	original.Result["animations"].([]any)[0].(map[string]any)["phase"] = "completed"
	if err = f.tasks.Store.Save(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	return f, service, c
}

func TestQualityRecheckUsesOriginalWhileBatchRunsAndPreservesHumanAnnotation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	f, service, c := recheckFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "review-model" || !strings.Contains(body.Input, "original_prompt") || !strings.Contains(body.Input, "source") {
			t.Error("not a source-only review")
		}
		close(entered)
		<-release
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": `{"label":"normal","reason":"车轮转动有实现","basis":[{"requirement":"车轮转动","evidence":"@keyframes spin{to{transform:rotate(360deg)}}","assessment":"CSS关键帧提供车轮转动实现。"}],"issues":[]}`})
	})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	request := modelcheck.AnimationQualityRecheckRequest{AccountID: "1", RequestID: "request-1"}
	task, err := service.EnqueueAnimationQualityRecheck(t.Context(), "quality-task", request, "operator")
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	duplicate, err := service.EnqueueAnimationQualityRecheck(t.Context(), "quality-task", request, "operator")
	if err != nil || duplicate.ID != task.ID {
		t.Fatalf("duplicate: %+v %v", duplicate, err)
	}
	read, err := service.AnimationQuality(t.Context(), "quality-task", "1", "request-1")
	if err != nil || read.Recheck == nil || read.Recheck.TaskID != task.ID {
		t.Fatalf("running: %+v %v", read, err)
	}
	saved, err := service.SaveAnimationQualityAnnotation(t.Context(), "quality-task", modelcheck.AnimationQualityAnnotationRequest{AccountID: "1", RequestID: "request-1", Label: "suspicious", Reason: "人工独立判断"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	done := finished(t, f)
	if done.Status != "succeeded" {
		t.Fatalf("%+v", done)
	}
	reopened, err := modelcheck.New(f.tasks, credentials{}, c, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	read, err = reopened.AnimationQuality(t.Context(), "quality-task", "1", "request-1")
	if err != nil || read.Prediction.Status != "normal" || !reflect.DeepEqual(read.Annotation, saved.Annotation) {
		t.Fatalf("reopened: %+v %v", read, err)
	}
	original, err := f.tasks.Get(t.Context(), "quality-task")
	if err != nil || original.Status != "running" {
		t.Fatalf("generation changed: %+v %v", original, err)
	}
	display, err := reopened.AnimationQualityDisplay(t.Context(), original)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(display.Result)
	var payload struct {
		Animations []modelcheck.AnimationResult `json:"animations"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Animations[0].Quality == nil || payload.Animations[0].Quality.Status != "normal" {
		t.Fatalf("display: %s", raw)
	}
	if _, err = reopened.EnqueueAnimationQualityRecheck(t.Context(), "quality-task", request, "operator"); !errors.Is(err, business.ErrQualityRecheckConflict) {
		t.Fatalf("stale request: %v", err)
	}
}

func TestQualityRecheckFailureRetainsPreviousPredictionWithoutGenerationOrRetry(t *testing.T) {
	calls := 0
	f, service, _ := recheckFixture(t, func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(503) })
	task, err := service.EnqueueAnimationQualityRecheck(t.Context(), "quality-task", modelcheck.AnimationQualityRecheckRequest{AccountID: "1", RequestID: "request-1"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	done := finished(t, f)
	if done.Status != "failed" || calls != 1 {
		t.Fatalf("%+v calls=%d", done, calls)
	}
	read, err := service.AnimationQuality(t.Context(), "quality-task", "1", "request-1")
	if err != nil || read.Prediction.Reason != "model_unavailable" || read.Recheck == nil || read.Recheck.TaskID != task.ID || read.Recheck.Status != "failed" {
		t.Fatalf("%+v %v", read, err)
	}
}

func TestQualityRecheckRejectsMissingSourceAndUnconfiguredReviewer(t *testing.T) {
	f, service, _ := recheckFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected upstream") })
	if _, err := service.EnqueueAnimationQualityRecheck(t.Context(), "quality-task", modelcheck.AnimationQualityRecheckRequest{AccountID: "1", RequestID: "wrong"}, "operator"); !errors.Is(err, modelcheck.ErrQualityReview) {
		t.Fatal(err)
	}
	config := service.AnimationQualitySettings()
	config.AccountID = ""
	config.Model = ""
	if _, err := service.SaveAnimationQualitySettings(t.Context(), config, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnqueueAnimationQualityRecheck(t.Context(), "quality-task", modelcheck.AnimationQualityRecheckRequest{AccountID: "1", RequestID: "request-1"}, "operator"); !errors.Is(err, modelcheck.ErrQualitySettings) {
		t.Fatal(err)
	}
	tasks, err := f.tasks.ListBySkill(t.Context(), "sub2api-animation-quality", 10)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("%+v %v", tasks, err)
	}
}

func TestQualityRecheckCancellationDoesNotReplacePrediction(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	f, service, _ := recheckFixture(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	defer close(release)
	task, err := service.EnqueueAnimationQualityRecheck(t.Context(), "quality-task", modelcheck.AnimationQualityRecheckRequest{AccountID: "1", RequestID: "request-1"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if !f.runner.CancelTask(task.ID) {
		t.Fatal("not cancellable")
	}
	done := finished(t, f)
	if done.Status != "cancelled" {
		t.Fatalf("%+v", done)
	}
	read, err := service.AnimationQuality(t.Context(), "quality-task", "1", "request-1")
	if err != nil || read.Prediction.Reason != "model_unavailable" {
		t.Fatalf("%+v %v", read, err)
	}
}
