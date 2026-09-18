package onboarding_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

const streamRequiredResponse = `{"error":{"code":"non_stream_not_allowed","message":"This account only accepts streaming conversation requests; set stream: true.","type":"invalid_request_error"}}`
const successfulProbeStream = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\",\"model\":\"gpt-test\"}\n\ndata: [DONE]\n\n"

func TestDefaultProbeRetriesExplicitStreamRequirementWithSameTarget(t *testing.T) {
	for _, mode := range []string{"", "default"} {
		t.Run("mode="+mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := calls.Add(1)
				var body struct {
					Model  string `json:"model"`
					Stream bool   `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" || body.Model != "gpt-test" || r.Header.Get("Authorization") != "Bearer "+testKey {
					t.Error("probe changed its target, model or credential")
				}
				if attempt == 1 {
					if body.Stream {
						t.Error("regular probe skipped the requested mode")
					}
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(streamRequiredResponse))
					return
				}
				if !body.Stream || r.Header.Get("Accept") != "text/event-stream" {
					t.Error("retry did not request streaming")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(successfulProbeStream))
			}))
			t.Cleanup(server.Close)
			service := onboarding.New(probeRepository{}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)

			result, err := service.Probe(t.Context(), "upstream.test", "6", "gpt-test", mode)

			if err != nil || result.Status != "passed" || result.HTTPStatus != http.StatusOK || result.ActualModel != "gpt-test" || result.ResponseText != "ok" || calls.Load() != 2 {
				t.Fatalf("stream fallback failed: result=%+v calls=%d err=%v", result, calls.Load(), err)
			}
		})
	}
}

func TestProbeDoesNotRetryWithoutExplicitNonStreamRejection(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		body, mode string
	}{
		{name: "other validation error", status: 400, body: `{"error":{"code":"invalid_model"}}`},
		{name: "message only", status: 400, body: `{"error":{"message":"non_stream_not_allowed"}}`},
		{name: "malformed response", status: 400, body: streamRequiredResponse + " trailing"},
		{name: "authentication failure", status: 401, body: streamRequiredResponse},
		{name: "permission failure", status: 403, body: streamRequiredResponse},
		{name: "rate limit", status: 429, body: streamRequiredResponse},
		{name: "server failure", status: 500, body: streamRequiredResponse},
		{name: "business failure after acceptance", status: 200, body: streamRequiredResponse},
		{name: "already streaming", status: 400, body: streamRequiredResponse, mode: "stream"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)
			service := onboarding.New(probeRepository{}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)

			result, err := service.Probe(t.Context(), "upstream.test", "6", "gpt-test", test.mode)

			if err == nil || result.Status != "failed" || calls.Load() != 1 {
				t.Fatalf("unexpected retry or success: result=%+v calls=%d err=%v", result, calls.Load(), err)
			}
		})
	}
}

func TestStreamFallbackPreservesFailureWithoutFurtherRetries(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		body, want string
	}{
		{name: "rejected again", status: 400, body: streamRequiredResponse, want: "non_stream_not_allowed"},
		{name: "failed after partial text", status: 200, body: successfulProbeStream + "data: {\"type\":\"error\",\"error\":{\"message\":\"generation failed\"}}\n\n", want: "generation failed"},
		{name: "empty stream", status: 200, body: "data: {\"type\":\"response.completed\"}\n\n", want: "未返回有效文本"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(streamRequiredResponse))
					return
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)
			service := onboarding.New(probeRepository{}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)

			result, err := service.Probe(t.Context(), "upstream.test", "6", "gpt-test", "default")

			if err == nil || !strings.Contains(err.Error(), test.want) || result.Status != "failed" || result.HTTPStatus != test.status || calls.Load() != 2 {
				t.Fatalf("fallback failure was lost: result=%+v calls=%d err=%v", result, calls.Load(), err)
			}
		})
	}
}
