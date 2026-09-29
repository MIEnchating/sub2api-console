package modelcheck_test

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAnimationFirstOutputTimeoutStartsAfterQueueAndStopsAfterDelta(t *testing.T) {
	var calls atomic.Int32
	f := setup(t, 4, "openai", func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", fixtureSVG)
		w.(http.Flusher).Flush()
		if call <= 3 {
			timer := time.NewTimer(5500 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	})
	queued, err := f.service.EnqueueAnimation(context.Background(), request("1", "2", "3", "4"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case task := <-f.tasks.terminal:
		rows, ok := task.Result["animations"].([]modelcheck.AnimationResult)
		if !ok {
			t.Fatalf("animation results missing: %#v", task.Result)
		}
		if task.Status != "succeeded" || len(rows) != 4 {
			t.Fatalf("queued request was timed out: task=%#v rows=%#v", task, rows)
		}
	case <-time.After(12 * time.Second):
		t.Fatalf("task %s did not complete", queued.ID)
	}
}
