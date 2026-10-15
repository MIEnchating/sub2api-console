package modelcheck_test

import (
	"context"
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

func TestAnimationExecutionTimeoutIncludesStreamingButExcludesQueue(t *testing.T) {
	var calls atomic.Int32
	f := setup(t, 4, "openai", func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", fixtureSVG)
		w.(http.Flusher).Flush()
		if call <= 3 {
			<-r.Context().Done()
			return
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
		if task.Status != "partial" || len(rows) != 4 {
			t.Fatalf("expected three stream timeouts and one fresh queued execution: task=%#v", task)
		}
		succeeded, failed := 0, 0
		for _, row := range rows {
			if row.Status == "succeeded" {
				succeeded++
			} else {
				failed++
				if row.SVG != "" || row.HTML != "" || row.RetryCount != 0 {
					t.Errorf("timeout exposed partial output or retried: %+v", row)
				}
			}
		}
		if succeeded != 1 || failed != 3 || calls.Load() != 4 {
			t.Fatalf("succeeded=%d failed=%d calls=%d", succeeded, failed, calls.Load())
		}
	case <-time.After(12 * time.Second):
		t.Fatalf("task %s did not complete", queued.ID)
	}
}

func TestOAuthAnimationExecutionTimeoutContinuesAfterFirstDelta(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	var calls atomic.Int32
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		reader, writer := io.Pipe()
		context.AfterFunc(r.Context(), func() { _ = writer.CloseWithError(r.Context().Err()) })
		go func() {
			_, _ = fmt.Fprintf(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", fixtureSVG)
		}()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader}, nil
	}))
	if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finishedWithin(t, f, 8*time.Second).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "failed" || !strings.Contains(row.Error, "超时") || row.SVG != "" || calls.Load() != 1 {
		t.Fatalf("stream did not time out after first delta: %+v calls=%d", row, calls.Load())
	}
}

func TestAnimationRetrySharesExecutionBudget(t *testing.T) {
	original := http.DefaultTransport
	remaining := make(chan time.Duration, 1)
	var calls atomic.Int32
	http.DefaultTransport = qualityQueueTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": []string{"4"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"busy"}}`))}, nil
		}
		deadline, _ := modelrequest.ExecutionDeadline(r.Context())
		remaining <- time.Until(deadline)
		return nil, context.DeadlineExceeded
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected live HTTP") })
	if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finishedWithin(t, f, 8*time.Second).Result["animations"].([]modelcheck.AnimationResult)[0]
	select {
	case budget := <-remaining:
		if budget <= 0 || budget > 2*time.Second {
			t.Errorf("retry reset the five-second execution budget: %s", budget)
		}
	default:
		t.Error("expected one permitted retry")
	}
	if row.Status != "failed" || row.RetryCount != 1 || calls.Load() != 2 {
		t.Fatalf("unexpected retry outcome: %+v calls=%d", row, calls.Load())
	}
}
