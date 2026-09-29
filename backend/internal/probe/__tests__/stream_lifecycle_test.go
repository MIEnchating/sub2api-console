package probe_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

// Delays model network arrival times under synctest's virtual clock. No real
// network, sleeps or elapsed-wall-time assertions are used in these tests.
type probeStreamChunk struct {
	after time.Duration
	data  string
	err   error
	wait  bool
}

type probeStreamBody struct {
	ctx    context.Context
	chunks []probeStreamChunk
	reader *strings.Reader
	closed bool
}

func (b *probeStreamBody) Read(p []byte) (int, error) {
	if b.reader != nil && b.reader.Len() > 0 {
		return b.reader.Read(p)
	}
	if len(b.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := b.chunks[0]
	b.chunks = b.chunks[1:]
	if chunk.wait {
		<-b.ctx.Done()
		return 0, b.ctx.Err()
	}
	if chunk.after > 0 {
		timer := time.NewTimer(chunk.after)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-b.ctx.Done():
			return 0, b.ctx.Err()
		}
	}
	if chunk.err != nil {
		return 0, chunk.err
	}
	b.reader = strings.NewReader(chunk.data)
	return b.reader.Read(p)
}

func (b *probeStreamBody) Close() error { b.closed = true; return nil }

type probeLifecycleTransport struct {
	body *probeStreamBody
}

func (transport probeLifecycleTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "probe-lifecycle.test" {
		return nil, fmt.Errorf("unexpected probe host: %s", r.URL.Host)
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41" {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"data":{"id":41,"type":"apikey","platform":"openai","credentials":{"base_url":"https://probe-lifecycle.test","api_key":"lifecycle-test-key"}}}`))}, nil
	}
	if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
		return nil, fmt.Errorf("unexpected probe endpoint: %s", r.URL.Path)
	}
	transport.body.ctx = r.Context()
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: transport.body}, nil
}

func TestProbeWaitsForCompletionWhileKeepingFirstTextLatency(t *testing.T) {
	store := directProbeStore(t)
	body := &probeStreamBody{chunks: []probeStreamChunk{
		{after: 100 * time.Millisecond, data: probeTextEvent},
		{after: 200 * time.Millisecond, data: probeTextEvent},
		{after: 2 * time.Second, data: probeCompletedEvent},
		{wait: true}, // Completion must not wait for the server to close HTTP.
	}}
	original := http.DefaultTransport
	http.DefaultTransport = probeLifecycleTransport{body: body}
	t.Cleanup(func() { http.DefaultTransport = original })
	synctest.Test(t, func(t *testing.T) {
		summary, err := probe.New(store, protectionTarget{endpoint: "https://probe-lifecycle.test"}, nil).RunNow(t.Context(), probe.Request{AccountID: pointer("41")})
		if err != nil || len(summary.Results) != 1 {
			t.Fatalf("probe result: %+v, %v", summary, err)
		}
		result := summary.Results[0]
		if result.Result != "通过" || result.DurationMS != 2300 || result.LatencyP50 == nil || *result.LatencyP50 != "100" || !result.MeasuredFirstToken {
			t.Fatalf("expected full completion at 2300ms and first text at 100ms: %+v", result)
		}
		if !body.closed || body.ctx.Err() != context.Canceled {
			t.Fatal("completed probe did not release its upstream response")
		}
	})
}

func TestProbeRejectsReadFailureOrTimeoutAfterFirstText(t *testing.T) {
	for _, scenario := range []struct {
		name, result, reason string
		tail                 probeStreamChunk
	}{
		{name: "transport interruption", result: "失败", reason: "读取中断", tail: probeStreamChunk{err: io.ErrUnexpectedEOF}},
		{name: "request deadline", result: "超时", reason: "超时", tail: probeStreamChunk{wait: true}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := directProbeStore(t)
			body := &probeStreamBody{chunks: []probeStreamChunk{{data: probeTextEvent}, scenario.tail}}
			original := http.DefaultTransport
			http.DefaultTransport = probeLifecycleTransport{body: body}
			t.Cleanup(func() { http.DefaultTransport = original })
			synctest.Test(t, func(t *testing.T) {
				summary, err := probe.New(store, protectionTarget{endpoint: "https://probe-lifecycle.test"}, nil).RunNow(t.Context(), probe.Request{AccountID: pointer("41")})
				if err != nil || len(summary.Results) != 1 {
					t.Fatalf("probe result: %+v, %v", summary, err)
				}
				result := summary.Results[0]
				if result.Result != scenario.result || result.FailureReason == nil || !strings.Contains(*result.FailureReason, scenario.reason) || result.LatencyP50 != nil {
					t.Fatalf("partial text masked failed request: %+v", result)
				}
				if !body.closed {
					t.Fatal("failed probe did not close the upstream response")
				}
			})
		})
	}
}
