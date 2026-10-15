package modelcheck_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type reviewingProgressStore struct {
	modelcheck.TaskStore
	snapshots chan taskstore.Task
}

func (s *reviewingProgressStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.TaskStore.Save(ctx, task); err != nil {
		return err
	}
	rows, _ := task.Result["animations"].([]modelcheck.AnimationResult)
	for _, row := range rows {
		if task.Status == "running" && row.Phase == "reviewing" {
			select {
			case s.snapshots <- task:
			default:
			}
			break
		}
	}
	return nil
}

func TestAnimationPublishesArtifactBeforeQualityReviewFinishes(t *testing.T) {
	for _, managed := range []bool{false, true} {
		name := "account"
		if managed {
			name = "managed"
		}
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body.Model == "review" {
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"output_text": htmlAnimation})
			})
			t.Cleanup(unblock)
			store := &reviewingProgressStore{TaskStore: f.tasks, snapshots: make(chan taskstore.Task, 16)}
			service, err := modelcheck.New(store, credentials{}, f.catalog, credentials{})
			if err != nil {
				t.Fatal(err)
			}
			service.UseTaskRunner(f.runner)
			if _, err := service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator"); err != nil {
				t.Fatal(err)
			}
			if managed {
				f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
				value := managedTask()
				value.Precheck = false
				plans, err := service.SaveDetectionTask(t.Context(), value, "operator")
				if err != nil {
					t.Fatal(err)
				}
				_, err = service.RunDetectionTask(t.Context(), plans[0].ID, plans[0].Version)
				if err != nil {
					t.Fatal(err)
				}
			} else if _, err := service.EnqueueAnimation(t.Context(), request("1")); err != nil {
				t.Fatal(err)
			}
			select {
			case snapshot := <-store.snapshots:
				row := snapshot.Result["animations"].([]modelcheck.AnimationResult)[0]
				if row.Status != "succeeded" || row.HTML == "" || row.Source != htmlAnimation || row.Prompt == "" || row.Quality != nil || row.CompletedAt == "" {
					t.Fatalf("generated artifact unavailable during review: %+v", row)
				}
				if snapshot.Progress != 0 || snapshot.Result["completed"] != 0 {
					t.Fatalf("review counted as completed: %+v", snapshot)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("generated artifact not published before review finishes")
			}
			unblock()
			final := finished(t, f)
			rows := final.Result["animations"].([]modelcheck.AnimationResult)
			if final.Status != "succeeded" || len(rows) != 1 || rows[0].Phase != "completed" || rows[0].HTML == "" || rows[0].Quality.Reason != "review_failed" {
				t.Fatalf("review failure changed generation result: %+v", final)
			}
		})
	}
}
