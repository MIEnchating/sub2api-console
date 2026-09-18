package modelcheck_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAnimationCompletesOnTerminalEventWithoutWaitingForConnectionClose(t *testing.T) {
	for _, tc := range []struct{ name, platform, path, body string }{
		{"responses", "openai", "/v1/responses", fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", fixtureSVG)},
		{"chat", "openai", "/v1/chat/completions", fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\n", fixtureSVG)},
		{"anthropic", "anthropic", "/v1/messages", fmt.Sprintf("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":%q}}\n\ndata: {\"type\":\"message_stop\"}\n\n", fixtureSVG)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, tc.platform, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, tc.body)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			})
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			// Bound a broken implementation without relying on elapsed-time assertions.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := f.service.EnqueueAnimation(ctx, request("1")); err != nil {
				t.Fatal(err)
			}
			runner.runs[0](ctx)
			task := finished(t, f)
			rows := task.Result["animations"].([]modelcheck.AnimationResult)
			if task.Status != "succeeded" || rows[0].SVG == "" {
				t.Fatalf("completed stream waited for connection close: %#v", task)
			}
		})
	}
}

type openStreamBody struct {
	data []byte
	ctx  context.Context
}

func (b *openStreamBody) Read(p []byte) (int, error) {
	if len(b.data) > 0 {
		n := copy(p, b.data)
		b.data = b.data[n:]
		return n, nil
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b *openStreamBody) Close() error { return nil }

func TestOAuthAnimationCompletesOnTerminalEventWithoutWaitingForConnectionClose(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		body := fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", fixtureSVG)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: &openStreamBody{data: []byte(body), ctx: r.Context()}}, nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := f.service.EnqueueAnimation(ctx, request("1")); err != nil {
		t.Fatal(err)
	}
	runner.runs[0](ctx)
	task := finished(t, f)
	if task.Status != "succeeded" {
		t.Fatalf("completed OAuth stream waited for connection close: %#v", task)
	}
}

func TestAnimationStreamingRejectsIncompleteFailedAndOversizedEvents(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"missing protocol completion", fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: [DONE]\n\n", fixtureSVG)},
		{"failed completion", fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\",\"output_text\":%q}}\n\n", fixtureSVG)},
		{"oversized events", strings.Repeat(": heartbeat\n\n", (4<<20)/13+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, tc.body)
			})
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
			if task.Status != "failed" || row.SVG != "" || row.Error == "" {
				t.Fatalf("invalid stream accepted: %#v", row)
			}
			if tc.name == "oversized events" && !strings.Contains(row.Error, "过大") {
				t.Fatalf("response size limit was not enforced: %s", row.Error)
			}
		})
	}
}

func TestCancellingAnimationDuringStreamDiscardsPartialSVG(t *testing.T) {
	started := make(chan struct{})
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", fixtureSVG)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	task, err := f.service.EnqueueAnimation(context.Background(), request("1"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if !f.runner.CancelTask(task.ID) {
		t.Fatal("streaming animation was not cancellable")
	}
	result := finished(t, f)
	row := result.Result["animations"].([]modelcheck.AnimationResult)[0]
	if result.Status != "cancelled" || row.SVG != "" {
		t.Fatalf("cancelled stream published partial SVG: %#v", result)
	}
}
