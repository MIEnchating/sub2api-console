package modelrequest_test

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestStreamingEveryReplyRearmsTimeoutWithoutFixedClientDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		client := &http.Client{Timeout: 5 * time.Second, Transport: transport(func(r *http.Request) (*http.Response, error) {
			reader, writer := io.Pipe()
			context.AfterFunc(r.Context(), func() { writer.CloseWithError(r.Context().Err()) })
			go func() {
				time.Sleep(3 * time.Second)
				io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
				time.Sleep(3 * time.Second)
				io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"again\"}\n\n")
			}()
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader}, nil
		})}
		response, err := modelrequest.Open(t.Context(), client, modelrequest.Request{BaseURL: "https://example.invalid", Protocol: modelrequest.Responses, Parameters: modelrequest.Parameters{Model: "test", Prompt: "hi", Stream: true}, Timeout: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, err = io.ReadAll(response.Body)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 11*time.Second {
			t.Fatalf("elapsed=%s err=%v", time.Since(started), err)
		}
	})
}

func TestNonstreamJSONFallbackAndHeartbeatsKeepOverallDeadline(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		stream bool
		body   string
	}{
		{"json", false, `{"output_text":"hello"}`},
		{"json fallback", true, `{"output_text":"hello"}`},
		{"heartbeat", true, ": keepalive\n\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				started := time.Now()
				client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
					reader, writer := io.Pipe()
					context.AfterFunc(r.Context(), func() { writer.CloseWithError(r.Context().Err()) })
					go func() { time.Sleep(3 * time.Second); io.WriteString(writer, fixture.body) }()
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: reader}, nil
				})}
				response, err := modelrequest.Open(t.Context(), client, modelrequest.Request{BaseURL: "https://example.invalid", Protocol: modelrequest.Responses, Parameters: modelrequest.Parameters{Model: "test", Prompt: "hi", Stream: fixture.stream}, Timeout: 5 * time.Second})
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				_, err = io.ReadAll(response.Body)
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 5*time.Second {
					t.Fatalf("elapsed=%s err=%v", time.Since(started), err)
				}
			})
		})
	}
}

func TestExecutionQueueAndNestedSenderDoNotShortenStreamingIdleTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Virtual time represents scheduling, before execution has acquired a slot.
		time.Sleep(20 * time.Second)
		ctx, cancel := modelrequest.Start(t.Context(), time.Second, true, 5*time.Second)
		defer cancel()
		started := time.Now()
		client := &http.Client{Timeout: time.Second, Transport: transport(func(r *http.Request) (*http.Response, error) {
			reader, writer := io.Pipe()
			context.AfterFunc(r.Context(), func() { writer.CloseWithError(r.Context().Err()) })
			go func() {
				time.Sleep(500 * time.Millisecond)
				io.WriteString(writer, "data: {\"type\":\"response.reasoning_text.delta\",\"delta\":\"thinking\"}\n\n")
			}()
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: reader}, nil
		})}
		response, err := modelrequest.Open(ctx, client, modelrequest.Request{BaseURL: "https://example.invalid", Protocol: modelrequest.Responses, Parameters: modelrequest.Parameters{Model: "test", Prompt: "hi", Stream: true}, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, err = io.ReadAll(response.Body)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 5500*time.Millisecond {
			t.Fatalf("elapsed=%s err=%v", time.Since(started), err)
		}
	})
}

func TestParentCancellationStopsAStreamAfterRenewal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, cancel := context.WithCancel(t.Context())
		defer cancel()
		started := time.Now()
		client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			reader, writer := io.Pipe()
			context.AfterFunc(r.Context(), func() { writer.CloseWithError(r.Context().Err()) })
			go func() {
				io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
				time.Sleep(time.Second)
				cancel()
			}()
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: reader}, nil
		})}
		response, err := modelrequest.Open(parent, client, modelrequest.Request{BaseURL: "https://example.invalid", Protocol: modelrequest.Responses, Parameters: modelrequest.Parameters{Model: "test", Prompt: "hi", Stream: true}, Timeout: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, err = io.ReadAll(response.Body)
		if !errors.Is(err, context.Canceled) || time.Since(started) != time.Second {
			t.Fatalf("elapsed=%s err=%v", time.Since(started), err)
		}
	})
}

func TestStreamRenewalPreservesStableParentContextDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, parentCancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer parentCancel()
		ctx, cancel := modelrequest.Start(parent, 5*time.Second, true)
		defer cancel()
		deadline, _ := ctx.Deadline()
		client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			time.Sleep(3 * time.Second)
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))}, nil
		})}
		response, err := modelrequest.Open(ctx, client, modelrequest.Request{BaseURL: "https://example.invalid", Protocol: modelrequest.Responses, Parameters: modelrequest.Parameters{Model: "test", Prompt: "hi", Stream: true}})
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if _, err := io.ReadAll(response.Body); err != nil {
			t.Fatal(err)
		}
		after, _ := ctx.Deadline()
		idleDeadline, _ := modelrequest.ExecutionDeadline(ctx)
		if !deadline.Equal(after) || time.Until(idleDeadline) != 5*time.Second {
			t.Fatalf("stable deadline changed or idle timer failed to renew: deadline=%s after=%s idle=%s", deadline, after, idleDeadline)
		}
	})
}
