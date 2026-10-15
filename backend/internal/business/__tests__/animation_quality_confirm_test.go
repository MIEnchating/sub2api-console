package business_test

import (
	"errors"
	"path/filepath"
	"testing"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestQualityConfirmationRejectsReviewChangedBeforeSampleCommit(t *testing.T) {
	for _, change := range []string{"started", "completed", "fingerprint"} {
		t.Run(change, func(t *testing.T) {
			store, err := business.Open(filepath.Join(t.TempDir(), "confirm.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			prediction := q.Prediction{Source: "model_review", Status: "normal", Explanation: "主体完整"}
			sample := q.Sample{ID: q.Hash("sample"), HTML: "source", Prompt: "prompt", SourceSHA256: q.Hash("source"), PromptSHA256: q.Hash("prompt"), Prediction: prediction}
			sample.Annotation = q.Annotation{Source: "confirmed_auto", Label: "normal", Reason: prediction.Explanation, UpdatedBy: "operator", SourceGroup: "group", PredictionSHA256: q.ReviewFingerprint(sample), ConfirmedPrediction: &prediction}
			state := q.Recheck{TaskID: "recheck", SourceSHA256: sample.SourceSHA256, PromptSHA256: sample.PromptSHA256}
			if change == "completed" || change == "fingerprint" {
				state.Prediction = &q.Prediction{Source: "model_review", Status: "suspicious", Explanation: "新评审理由"}
			}
			if _, err = store.SaveAnimationQualityRecheck(t.Context(), sample.ID, state, 0); err != nil {
				t.Fatal(err)
			}
			if change == "fingerprint" {
				sample.Annotation.RecheckVersion = 1
			}
			if _, err = store.SaveAnimationQualitySample(t.Context(), sample, 0); !errors.Is(err, business.ErrQualityConfirmationConflict) {
				t.Fatalf("stale confirmation committed: %v", err)
			}
			page, err := store.AnimationQualitySamples(t.Context(), "", 50)
			if err != nil || len(page.Items) != 0 {
				t.Fatalf("sample saved on conflict: %+v %v", page, err)
			}
		})
	}
}
