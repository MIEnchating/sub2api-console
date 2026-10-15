package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

type qualityQueueTransport func(*http.Request) (*http.Response, error)

func (f qualityQueueTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQualityQueueDoesNotConsumeNextRequestTimeout(t *testing.T) {
	// Saturate all three standalone slots, then queue a fourth source-only
	// review. Real execution deadlines release slots; no sleeps or polling.
	original := http.DefaultTransport
	entered := make(chan struct{}, 3)
	remaining := make(chan time.Duration, 1)
	var reviewCalls atomic.Int32
	http.DefaultTransport = qualityQueueTransport(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.Header.Get("X-Request-ID"), "-quality") {
			return original.RoundTrip(r)
		}
		if reviewCalls.Add(1) <= 3 {
			entered <- struct{}{}
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		deadline, ok := modelrequest.ExecutionDeadline(r.Context())
		if !ok {
			return nil, context.DeadlineExceeded
		}
		remaining <- time.Until(deadline)
		output := validQualityReview
		raw, _ := json.Marshal(map[string]any{"output_text": output})
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	f, service, _ := recheckFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("recheck regenerated an animation") })
	enqueue := func(index int) {
		t.Helper()
		source := qualityTask()
		source.ID = fmt.Sprintf("queue-source-%d", index)
		if err := f.tasks.Store.Save(t.Context(), source); err != nil {
			t.Fatal(err)
		}
		if _, err := service.EnqueueAnimationQualityRecheck(t.Context(), source.ID, modelcheck.AnimationQualityRecheckRequest{AccountID: "1", RequestID: "request-1"}, "operator"); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		enqueue(i)
	}
	for range 3 {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("reviews did not use three animation slots")
		}
	}
	enqueue(3)
	failed, succeeded := 0, 0
	for range 4 {
		task := finishedWithin(t, f, 10*time.Second)
		if task.Status == "succeeded" {
			succeeded++
		} else if task.Status == "failed" && strings.Contains(task.Message, "评审请求超时") {
			failed++
		} else {
			t.Errorf("unexpected result: %+v", task)
		}
	}
	select {
	case budget := <-remaining:
		if budget < 4*time.Second {
			t.Errorf("queue consumed request budget: remaining %s of configured 5s", budget)
		}
	default:
		t.Error("queued review timed out without sending a request")
	}
	if failed != 3 || succeeded != 1 || reviewCalls.Load() != 4 {
		t.Errorf("failed=%d succeeded=%d requests=%d; want 3, 1, 4", failed, succeeded, reviewCalls.Load())
	}
}
