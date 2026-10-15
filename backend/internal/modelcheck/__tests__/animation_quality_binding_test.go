package modelcheck_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAutoQualityDiscardsResultWhenReviewerChangesDuringStream(t *testing.T) {
	for _, change := range []string{"binding", "exclusion"} {
		t.Run(change, func(t *testing.T) {
			var f *fixture
			f = setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				output := htmlAnimation
				if body.Model == "review" {
					f.catalog.mu.Lock()
					if change == "binding" {
						f.catalog.details["1"].Bindings[0].UpstreamKeyID = "92"
					} else {
						f.catalog.excluded = map[string]bool{"1": true}
					}
					f.catalog.mu.Unlock()
					output = validQualityReview
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"output_text": output})
			})
			if _, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if row.Status != "succeeded" || row.Quality == nil || row.Quality.Reason != "review_account_unavailable" || row.Quality.Explanation != "" {
				t.Fatalf("changed reviewer result was accepted: %+v", row.Quality)
			}
		})
	}
}
