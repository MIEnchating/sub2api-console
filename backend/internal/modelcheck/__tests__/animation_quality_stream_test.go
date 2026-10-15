package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAutoQualityOAuthStreamCompletesWithoutConnectionEOF(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	deltaSent, finish := make(chan struct{}), make(chan struct{})
	defer close(finish)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.Header.Get("X-Request-ID"), "-quality") {
			return oauthResponse(200, "application/json", fmt.Sprintf(`{"status":"completed","output_text":%q}`, htmlAnimation)), nil
		}
		var body struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		if !body.Stream || r.Header.Get("Accept") != "text/event-stream" {
			t.Error("review must request streaming")
		}
		reader, writer := io.Pipe()
		context.AfterFunc(r.Context(), func() { _ = writer.CloseWithError(r.Context().Err()) })
		go func() {
			_, _ = fmt.Fprintf(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", validQualityReview)
			close(deltaSent)
			select {
			case <-finish:
				_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"review\"}}\n\n")
			case <-r.Context().Done():
			}
			// Deliberately keep the connection open after the complete event.
		}()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader}, nil
	}))
	if _, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-deltaSent:
	case <-time.After(3 * time.Second):
		t.Fatal("review did not stream output")
	}
	select {
	case task := <-f.tasks.terminal:
		t.Fatalf("partial review became final before completion: %+v", task)
	default:
	}
	finish <- struct{}{}
	task := finishedWithin(t, f, 8*time.Second)
	row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "succeeded" || row.Quality == nil || row.Quality.Status != "normal" {
		t.Fatalf("complete review waited for EOF: %+v", row)
	}
}

func TestAutoQualityAPIKeyStreamRequiresCompleteEvent(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(fmt.Sprintf("complete=%t", complete), func(t *testing.T) {
			var calls atomic.Int32
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body struct {
					Model  string `json:"model"`
					Stream bool   `json:"stream"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body.Model != "review" {
					_ = json.NewEncoder(w).Encode(map[string]string{"output_text": htmlAnimation})
					return
				}
				if !body.Stream {
					t.Error("automatic review did not request a stream")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", validQualityReview)
				w.(http.Flusher).Flush()
				if complete {
					_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				}
			})
			if _, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if row.Status != "succeeded" || calls.Load() != 2 || row.Quality == nil {
				t.Fatalf("review changed generation or retried: %+v calls=%d", row, calls.Load())
			}
			if complete && row.Quality.Status != "normal" {
				t.Fatalf("complete review rejected: %+v", row.Quality)
			}
			if !complete && (row.Quality.Status != "unknown" || row.Quality.Reason != "review_failed") {
				t.Fatalf("truncated stream became a verdict: %+v", row.Quality)
			}
		})
	}
}

func TestAutoQualityOAuthTruncatedStreamFailsWithoutRetry(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	var calls atomic.Int32
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if !strings.HasSuffix(r.Header.Get("X-Request-ID"), "-quality") {
			return oauthResponse(200, "application/json", fmt.Sprintf(`{"status":"completed","output_text":%q}`, htmlAnimation)), nil
		}
		return oauthResponse(200, "text/event-stream", fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", validQualityReview)), nil
	}))
	if _, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "succeeded" || calls.Load() != 2 || row.Quality == nil || row.Quality.Reason != "review_failed" {
		t.Fatalf("partial review accepted or retried: %+v calls=%d", row.Quality, calls.Load())
	}
}
