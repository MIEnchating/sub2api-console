package modelcheck_test

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAnimationAllSelectedAccountsExceedTwenty(t *testing.T) {
	f := setup(t, 25, "openai", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":%q}}\n\n", fixtureSVG)
	})
	ids := make([]string, 25)
	for i := range ids {
		ids[i] = strconv.Itoa(i + 1)
	}
	if _, err := f.service.EnqueueAnimation(context.Background(), request(ids...)); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	results := task.Result["animations"].([]modelcheck.AnimationResult)
	if task.Status != "succeeded" || len(results) != 25 {
		t.Fatalf("incomplete all-account task: status=%s results=%d", task.Status, len(results))
	}
	seen := make(map[string]bool)
	for _, result := range results {
		seen[result.AccountID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			t.Errorf("missing account %s", id)
		}
	}
}
