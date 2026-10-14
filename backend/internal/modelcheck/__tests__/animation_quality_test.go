package modelcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func qualityFixture(t *testing.T) (*fixture, *modelcheck.Service, *business.Store) {
	t.Helper()
	f := manualVerdictFixture(t)
	store, err := business.Open(filepath.Join(t.TempDir(), "quality.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := modelcheck.New(f.tasks, credentials{}, store, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	return f, service, store
}
func qualityTask() taskstore.Task {
	task := manualVerdictTask("quality-task")
	row := task.Result["animations"].([]any)[0].(map[string]any)
	row["source"] = htmlAnimation
	row["html"] = "sanitized-preview"
	row["prompt"] = q.PelicanPrompt
	return task
}
func TestQualityAnnotationPreservesOriginalSourceAndOtherEvidence(t *testing.T) {
	f, service, repo := qualityFixture(t)
	task := qualityTask()
	ctx := context.Background()
	if err := f.tasks.Store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	before, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	review, err := service.AnimationQuality(ctx, task.ID, "1", "request-1")
	if err != nil || review.Prediction.Reason != "model_unavailable" || review.Annotation != nil {
		t.Fatalf("%+v %v", review, err)
	}
	input := modelcheck.AnimationQualityAnnotationRequest{AccountID: "1", RequestID: "request-1", Label: "suspicious", Reason: "脚踏连接缺失"}
	saved, err := service.SaveAnimationQualityAnnotation(ctx, task.ID, input, "operator")
	if err != nil {
		t.Fatal(err)
	}
	sample, err := repo.AnimationQualitySample(ctx, saved.SampleID)
	if err != nil || sample.HTML != htmlAnimation || sample.Prompt != q.PelicanPrompt || sample.Annotation.SourceGroup != "sha256:"+q.Hash(htmlAnimation) || sample.Annotation.UpdatedBy != "operator" {
		t.Fatalf("wrong sample: %+v %v", sample, err)
	}
	after, err := f.tasks.Get(ctx, task.ID)
	if err != nil || !reflect.DeepEqual(after.Result, before.Result) {
		t.Fatal("quality annotation changed generation/manual verdict/other evidence")
	}
	if _, err = service.SaveAnimationQualityAnnotation(ctx, task.ID, input, "operator"); !errors.Is(err, business.ErrQualityAnnotationConflict) {
		t.Fatalf("stale write: %v", err)
	}
	input.Version = 1
	input.Label = "unknown"
	if _, err = service.SaveAnimationQualityAnnotation(ctx, task.ID, input, "reviewer"); err != nil {
		t.Fatal(err)
	}
	if err := f.tasks.Store.DeleteTerminalBySkill(ctx, task.Skill, []taskstore.HistorySelection{{ID: task.ID, UpdatedAt: after.UpdatedAt}}); err != nil {
		t.Fatal(err)
	}
	page, err := service.AnimationQualitySamples(ctx, "", 50)
	if err != nil || len(page.Items) != 1 || page.Items[0].Annotation.Version != 2 {
		t.Fatalf("export: %+v %v", page, err)
	}
}
func TestQualityAnnotationRejectsIncompleteOrMismatchedResults(t *testing.T) {
	f, service, _ := qualityFixture(t)
	for _, name := range []string{"source absent", "source truncated", "prompt absent", "wrong request", "wrong account", "failed", "precheck", "duplicate", "active", "wrong skill", "missing reason", "invalid label", "missing completion"} {
		t.Run(name, func(t *testing.T) {
			task := qualityTask()
			task.ID = q.Hash(name)
			row := task.Result["animations"].([]any)[0].(map[string]any)
			request := modelcheck.AnimationQualityAnnotationRequest{AccountID: "1", RequestID: "request-1", Label: "normal", Reason: "可复核理由"}
			want := modelcheck.ErrQualityReview
			switch name {
			case "source absent":
				delete(row, "source")
			case "source truncated":
				row["source_truncated"] = true
			case "prompt absent":
				delete(row, "prompt")
			case "wrong request":
				request.RequestID = "different"
			case "wrong account":
				request.AccountID = "2"
			case "failed":
				row["status"] = "failed"
			case "precheck":
				row["mode"] = "precheck"
			case "duplicate":
				task.Result["animations"] = []any{row, row}
			case "active":
				task.Status = "running"
				want = taskstore.ErrTaskStillActive
			case "wrong skill":
				task.Skill = "unrelated"
			case "missing reason":
				request.Reason = " "
				want = modelcheck.ErrQualityAnnotation
			case "invalid label":
				request.Label = "passed"
				want = modelcheck.ErrQualityAnnotation
			case "missing completion":
				delete(row, "completed_at")
			}
			if err := f.tasks.Store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			if _, err := service.SaveAnimationQualityAnnotation(t.Context(), task.ID, request, "operator"); !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
		})
	}
}
func TestQualityReviewAcceptsOnboardingWithActualCustomPrompt(t *testing.T) {
	f, service, _ := qualityFixture(t)
	task := qualityTask()
	row := task.Result["animations"].([]any)[0].(map[string]any)
	row["account_id"] = ""
	row["prompt"] = "生成鲸鱼 HTML"
	task.Skill = "sub2api-account-onboarding"
	task.Operation = "onboarding-probe-probe"
	task.Result = map[string]any{"probe_result": map[string]any{"animation": row}}
	if err := f.tasks.Store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	review, err := service.SaveAnimationQualityAnnotation(t.Context(), task.ID, modelcheck.AnimationQualityAnnotationRequest{RequestID: "request-1", Label: "normal", Reason: "鲸鱼结构完整"}, "operator")
	if err != nil || review.Prediction.Reason != "unsupported_benchmark" || review.Benchmark != "prompt-"+q.Hash("生成鲸鱼 HTML") {
		t.Fatalf("%+v %v", review, err)
	}
}
func TestSuccessfulGenerationPersistsUnknownWithoutConfiguredModel(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": htmlAnimation})
	})
	if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "succeeded" || row.Quality == nil || row.Quality.Status != "unknown" || row.Quality.Reason != "model_unavailable" || row.Quality.Score != nil {
		t.Fatalf("%+v", row)
	}
	persisted, err := f.tasks.Get(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(persisted.Result)
	var payload struct {
		Animations []modelcheck.AnimationResult `json:"animations"`
	}
	_ = json.Unmarshal(raw, &payload)
	if payload.Animations[0].Quality == nil || payload.Animations[0].Quality.Reason != "model_unavailable" {
		t.Fatal("quality prediction lost on task persistence")
	}
}
