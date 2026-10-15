package business_test

import (
	"errors"
	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"path/filepath"
	"sync"
	"testing"
)

func TestQualityRecheckConcurrentClaimAndLateWriteDoNotOverwriteNewerReview(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "recheck.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := q.Hash("source-task-request")
	state := q.Recheck{TaskID: "first", SourceSHA256: q.Hash("source"), PromptSHA256: q.Hash("prompt")}
	outcomes := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _, err := store.SaveAnimationQualityRecheck(t.Context(), id, state, 0); outcomes <- err })
	}
	wg.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else if errors.Is(err, business.ErrQualityRecheckConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflict=%d", successes, conflicts)
	}
	state.TaskID = "second"
	state.Prediction = &q.Prediction{Status: "normal"}
	saved, err := store.SaveAnimationQualityRecheck(t.Context(), id, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	state.TaskID = "late"
	state.Prediction = &q.Prediction{Status: "suspicious"}
	if _, err = store.SaveAnimationQualityRecheck(t.Context(), id, state, 1); !errors.Is(err, business.ErrQualityRecheckConflict) {
		t.Fatal(err)
	}
	state.SourceSHA256 = q.Hash("changed-source")
	if _, err = store.SaveAnimationQualityRecheck(t.Context(), id, state, 2); !errors.Is(err, business.ErrQualityRecheckConflict) {
		t.Fatal(err)
	}
	loaded, err := store.AnimationQualityRechecks(t.Context(), []string{id, q.Hash("missing")})
	if err != nil || len(loaded) != 1 || loaded[id].TaskID != saved.TaskID || loaded[id].Prediction.Status != "normal" {
		t.Fatalf("%+v %v", loaded, err)
	}
	samples, err := store.AnimationQualitySamples(t.Context(), "", 10)
	if err != nil || len(samples.Items) != 0 {
		t.Fatalf("automatic review became human samples: %+v %v", samples, err)
	}
}
