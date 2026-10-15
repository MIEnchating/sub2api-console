package modelcheck_test

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

const validQualityReview = `{"label":"normal","reason":"车轮具备旋转实现","basis":[{"requirement":"车轮转动","evidence":"@keyframes spin{to{transform:rotate(360deg)}}","assessment":"轮圈绑定旋转关键帧。"}],"issues":[]}`

func TestAutoQualityFollowsAnimationConcurrency(t *testing.T) {
	for _, tc := range []struct {
		name       string
		managed    bool
		concurrent int
	}{
		{"standalone three", false, 3},
		{"managed one", true, 1},
		{"managed two", true, 2},
		{"managed four", true, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entered, release := make(chan struct{}, 5), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			var active, peak atomic.Int32
			f := setup(t, 5, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				output := htmlAnimation
				if body.Model == "review" {
					current := active.Add(1)
					defer active.Add(-1)
					for previous := peak.Load(); current > previous; previous = peak.Load() {
						if peak.CompareAndSwap(previous, current) {
							break
						}
					}
					entered <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					output = validQualityReview
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"output_text": output})
			})
			if _, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator"); err != nil {
				t.Fatal(err)
			}
			if tc.managed {
				f.catalog.groupMembers = map[string][]string{"7": {"1", "2", "3", "4", "5"}, "8": {}}
				value := managedTask()
				value.Precheck = false
				value.Animation = new(true)
				value.Concurrency = tc.concurrent
				plans, err := f.service.SaveDetectionTask(t.Context(), value, "operator")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.service.RunDetectionTask(t.Context(), plans[0].ID, plans[0].Version); err != nil {
					t.Fatal(err)
				}
			} else if _, err := f.service.EnqueueAnimation(t.Context(), request("1", "2", "3", "4", "5")); err != nil {
				t.Fatal(err)
			}
			for range tc.concurrent {
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatalf("review peak=%d, want %d concurrent reviews", peak.Load(), tc.concurrent)
				}
			}
			once.Do(func() { close(release) })
			task := finished(t, f)
			if task.Status != "succeeded" || peak.Load() != int32(tc.concurrent) {
				t.Fatalf("status=%s peak=%d, want %d", task.Status, peak.Load(), tc.concurrent)
			}
			rows := task.Result["animations"].([]modelcheck.AnimationResult)
			if len(rows) != 5 {
				t.Fatalf("lost results: %+v", task.Result)
			}
			for _, row := range rows {
				if row.Quality == nil || row.Quality.Status != "normal" {
					t.Errorf("review failed: %+v", row)
				}
			}
		})
	}
}
