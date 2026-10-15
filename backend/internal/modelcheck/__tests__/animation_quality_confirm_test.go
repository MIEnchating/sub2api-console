package modelcheck_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestQualityConfirmationExportsGeneratedReasonAndEvidence(t *testing.T) {
	f, service, _ := qualityFixture(t)
	task := qualityTask()
	const output = `{"label":"normal","reason":"车轮旋转有实现支持。","basis":[{"requirement":"车轮转动","evidence":"@keyframes spin{to{transform:rotate(360deg)}}","assessment":"旋转关键帧支持车轮转动要求。"}],"issues":[]}`
	prediction := q.ParseReview(output, q.PelicanPrompt, htmlAnimation)
	prediction.Source = "model_review"
	if prediction.Status != "normal" {
		t.Fatalf("invalid fixture: %+v", prediction)
	}
	task.Result["animations"].([]any)[0].(map[string]any)["quality"] = prediction
	if err := f.tasks.Store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	review, err := service.AnimationQuality(t.Context(), task.ID, "1", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfirmAnimationQuality(t.Context(), task.ID, confirmationRequest(review), "operator"); err != nil {
		t.Fatal(err)
	}
	page, err := service.AnimationQualitySamples(t.Context(), "", 50)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("export: %+v %v", page, err)
	}
	// Exercise the serialized export contract, including the confirmation snapshot.
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var exported q.SamplePage
	if err = json.Unmarshal(raw, &exported); err != nil {
		t.Fatal(err)
	}
	sample := exported.Items[0]
	confirmed := sample.Annotation.ConfirmedPrediction
	if confirmed == nil || sample.Annotation.Reason != prediction.Explanation || confirmed.Explanation != prediction.Explanation || confirmed.RubricVersion != q.ReviewRubricVersion || !reflect.DeepEqual(confirmed.Basis, prediction.Basis) || !reflect.DeepEqual(sample.Prediction.Basis, prediction.Basis) {
		t.Fatalf("generated reason or source evidence lost: %+v", sample)
	}
}

func confirmationRequest(review modelcheck.AnimationQualityReview) modelcheck.AnimationQualityConfirmationRequest {
	version := 0
	if review.Annotation != nil {
		version = review.Annotation.Version
	}
	return modelcheck.AnimationQualityConfirmationRequest{AccountID: "1", RequestID: "request-1", Version: version, RecheckVersion: review.RecheckVersion, PredictionSHA256: review.PredictionSHA256}
}

func TestQualityConfirmationCollectsAutomaticLabelWithoutManualInput(t *testing.T) {
	for _, label := range []string{"normal", "suspicious"} {
		t.Run(label, func(t *testing.T) {
			f, service, repo := qualityFixture(t)
			task := qualityTask()
			task.Status = "running"
			row := task.Result["animations"].([]any)[0].(map[string]any)
			row["phase"] = "completed"
			prediction := q.Prediction{Status: label, Source: "model_review", Explanation: "主体与运动评审理由", ModelVersion: "review-model", RubricVersion: q.ReviewRubricVersion}
			row["quality"] = prediction
			if err := f.tasks.Store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			before, _ := f.tasks.Get(t.Context(), task.ID)
			review, err := service.AnimationQuality(t.Context(), task.ID, "1", "request-1")
			if err != nil {
				t.Fatal(err)
			}
			page, err := repo.AnimationQualitySamples(t.Context(), "", 50)
			if err != nil || len(page.Items) != 0 {
				t.Fatalf("unconfirmed sample: %+v %v", page, err)
			}
			request := confirmationRequest(review)
			saved, err := service.ConfirmAnimationQuality(t.Context(), task.ID, request, "operator")
			if err != nil {
				t.Fatal(err)
			}
			if saved.Annotation == nil || saved.Annotation.Source != "confirmed_auto" || saved.Annotation.Label != label || saved.Annotation.Reason != prediction.Explanation || saved.Annotation.UpdatedBy != "operator" || saved.Annotation.UpdatedAt == "" || saved.Annotation.PredictionSHA256 != review.PredictionSHA256 || !reflect.DeepEqual(saved.Annotation.ConfirmedPrediction, &prediction) {
				t.Fatalf("wrong confirmation: %+v", saved)
			}
			if _, err = service.ConfirmAnimationQuality(t.Context(), task.ID, request, "operator"); !errors.Is(err, business.ErrQualityAnnotationConflict) {
				t.Fatalf("duplicate submission: %v", err)
			}
			page, err = service.AnimationQualitySamples(t.Context(), "", 50)
			if err != nil || len(page.Items) != 1 || page.Items[0].HTML != htmlAnimation || page.Items[0].Prompt != q.PelicanPrompt || page.Items[0].Annotation.SourceGroup != "sha256:"+q.Hash(htmlAnimation) {
				t.Fatalf("export: %+v %v", page, err)
			}
			after, _ := f.tasks.Get(t.Context(), task.ID)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("confirmation changed generation or batch")
			}
		})
	}
}

func TestQualityConfirmationRequiresFreshRecheckAndPreservesOriginalSnapshot(t *testing.T) {
	f, service, repo := qualityFixture(t)
	task := qualityTask()
	row := task.Result["animations"].([]any)[0].(map[string]any)
	original := q.Prediction{Source: "model_review", Status: "suspicious", Explanation: "原评审理由"}
	row["quality"] = original
	if err := f.tasks.Store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	review, err := service.SaveAnimationQualityAnnotation(t.Context(), task.ID, modelcheck.AnimationQualityAnnotationRequest{AccountID: "1", RequestID: "request-1", Label: "suspicious", Reason: "人工理由", SourceGroup: "模板甲"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	old := confirmationRequest(review)
	prediction := q.Prediction{Source: "model_review", Status: "normal", Explanation: "重新评审确认主体和运动正常"}
	state, err := repo.SaveAnimationQualityRecheck(t.Context(), review.SampleID, q.Recheck{TaskID: "recheck", SourceSHA256: review.SourceSHA256, PromptSHA256: review.PromptSHA256, Prediction: &prediction}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfirmAnimationQuality(t.Context(), task.ID, old, "operator"); !errors.Is(err, business.ErrQualityConfirmationConflict) {
		t.Fatalf("stale review: %v", err)
	}
	review, err = service.AnimationQuality(t.Context(), task.ID, "1", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := service.ConfirmAnimationQuality(t.Context(), task.ID, confirmationRequest(review), "confirmer")
	if err != nil {
		t.Fatal(err)
	}
	sample, err := repo.AnimationQualitySample(t.Context(), saved.SampleID)
	if err != nil || sample.Annotation.Label != "normal" || sample.Annotation.Version != 2 || sample.Annotation.SourceGroup != "模板甲" || !reflect.DeepEqual(sample.Prediction, original) || !reflect.DeepEqual(sample.Annotation.ConfirmedPrediction, &prediction) {
		t.Fatalf("wrong snapshot: %+v %v", sample, err)
	}
	// Later reviews do not silently change the training label.
	prediction.Status = "suspicious"
	state.Prediction = &prediction
	if _, err = repo.SaveAnimationQualityRecheck(t.Context(), review.SampleID, state, state.Version); err != nil {
		t.Fatal(err)
	}
	page, err := service.AnimationQualitySamples(t.Context(), "", 50)
	if err != nil || page.Items[0].Annotation.Label != "normal" || page.Items[0].Annotation.ConfirmedPrediction.Status != "normal" {
		t.Fatalf("review changed sample: %+v %v", page, err)
	}
	// A manual correction replaces confirmation provenance, never leaves a stale snapshot attached.
	corrected, err := service.SaveAnimationQualityAnnotation(t.Context(), task.ID, modelcheck.AnimationQualityAnnotationRequest{AccountID: "1", RequestID: "request-1", Label: "unknown", Reason: "人工存在分歧", Version: 2}, "operator")
	if err != nil || corrected.Annotation.Source != "manual" || corrected.Annotation.ConfirmedPrediction != nil {
		t.Fatalf("correction: %+v %v", corrected, err)
	}
}

func TestQualityConfirmationRejectsUnavailableOrUnfinishedReview(t *testing.T) {
	for _, name := range []string{"unknown", "no reason", "failure", "local model", "active recheck", "wrong request", "wrong fingerprint"} {
		t.Run(name, func(t *testing.T) {
			f, service, repo := qualityFixture(t)
			task := qualityTask()
			prediction := q.Prediction{Source: "model_review", Status: "normal", Explanation: "主体完整"}
			switch name {
			case "unknown":
				prediction.Status = "unknown"
			case "no reason":
				prediction.Explanation = ""
			case "failure":
				prediction.Reason = "review_timeout"
			case "local model":
				prediction.Source = ""
			}
			task.Result["animations"].([]any)[0].(map[string]any)["quality"] = prediction
			if err := f.tasks.Store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			review, err := service.AnimationQuality(t.Context(), task.ID, "1", "request-1")
			if err != nil {
				t.Fatal(err)
			}
			if name == "active recheck" {
				if err := f.tasks.Store.Save(t.Context(), taskstore.Task{ID: "recheck", Message: "正在复核", Skill: "sub2api-animation-quality", Operation: "animation-quality-recheck", Status: "running", CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, Result: map[string]any{}}); err != nil {
					t.Fatal(err)
				}
				if _, err := repo.SaveAnimationQualityRecheck(t.Context(), review.SampleID, q.Recheck{TaskID: "recheck", SourceSHA256: review.SourceSHA256, PromptSHA256: review.PromptSHA256}, 0); err != nil {
					t.Fatal(err)
				}
				review, err = service.AnimationQuality(t.Context(), task.ID, "1", "request-1")
				if err != nil {
					t.Fatal(err)
				}
			}
			request := confirmationRequest(review)
			want := modelcheck.ErrQualityConfirmation
			if name == "wrong request" {
				request.RequestID = "wrong"
				want = modelcheck.ErrQualityReview
			}
			if name == "wrong fingerprint" {
				request.PredictionSHA256 = q.Hash("other")
				want = business.ErrQualityConfirmationConflict
			}
			if _, err := service.ConfirmAnimationQuality(t.Context(), task.ID, request, "operator"); !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			page, err := repo.AnimationQualitySamples(t.Context(), "", 50)
			if err != nil || len(page.Items) != 0 {
				t.Fatalf("invalid sample stored: %+v %v", page, err)
			}
		})
	}
}

func TestQualityConfirmedSampleDoesNotOverrideLatestRecheckInConfidence(t *testing.T) {
	f, service, repo := qualityFixture(t)
	task := qualityTask()
	task.Result["animations"] = task.Result["animations"].([]any)[:1]
	task.Result["animations"].([]any)[0].(map[string]any)["quality"] = q.Prediction{Source: "model_review", Status: "normal", Explanation: "主体正常"}
	if err := f.tasks.Store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	review, err := service.AnimationQuality(t.Context(), task.ID, "1", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := service.ConfirmAnimationQuality(t.Context(), task.ID, confirmationRequest(review), "operator")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.SaveAnimationQualityRecheck(t.Context(), review.SampleID, q.Recheck{TaskID: "recheck", SourceSHA256: review.SourceSHA256, PromptSHA256: review.PromptSHA256, Prediction: &q.Prediction{Status: "suspicious"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := service.AccountStatuses(t.Context())
	if err != nil || len(statuses) != 1 || statuses[0].Confidence.Short.Failed != 1 {
		t.Fatalf("stale training label used: %+v %v", statuses, err)
	}
	sample, err := repo.AnimationQualitySample(t.Context(), saved.SampleID)
	if err != nil || sample.Annotation.Label != "normal" {
		t.Fatalf("sample mutated: %+v %v", sample, err)
	}
	displayed, err := service.AnimationQualityDisplay(t.Context(), task)
	if err != nil {
		t.Fatal(err)
	}
	row := displayed.Result["animations"].([]any)[0].(map[string]any)
	current, err := service.AnimationQuality(t.Context(), task.ID, "1", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if row["quality_prediction_sha256"] != current.PredictionSHA256 {
		t.Fatal("card fingerprint differs from quality details")
	}
}
