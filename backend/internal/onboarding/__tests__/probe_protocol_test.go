package onboarding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

const messagesRequiredResponse = `{"error":{"message":"field messages is required (request id: isolated-request)","type":"new_api_error","param":"","code":null}}`
const conversionUnsupportedResponse = `{"error":{"message":"not implemented (request id: isolated-request)","type":"new_api_error","param":"","code":"convert_request_failed"}}`

type protocolProbeRepository struct {
	probeRepository
	platform *string
}

func (repo protocolProbeRepository) OnboardingCandidates(ctx context.Context, host string) ([]business.OnboardingCandidate, error) {
	candidates, err := repo.probeRepository.OnboardingCandidates(ctx, host)
	for index := range candidates {
		candidates[index].Platform = repo.platform
	}
	return candidates, err
}

func TestProbeRetriesConfirmedProtocolRejectionWithChatMessages(t *testing.T) {
	anthropic := "anthropic"
	for _, test := range []struct {
		name, path, rejection string
		platform              *string
		status                int
	}{
		{name: "missing platform requires messages", path: "/v1/responses", rejection: messagesRequiredResponse, status: 400},
		{name: "Anthropic conversion is not implemented", path: "/v1/messages", rejection: conversionUnsupportedResponse, status: 500, platform: &anthropic},
	} {
		for _, mode := range []string{"default", "stream"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempt := calls.Add(1)
					var body struct {
						Model    string                           `json:"model"`
						Messages []struct{ Role, Content string } `json:"messages"`
						Stream   bool                             `json:"stream"`
						Input    *string                          `json:"input"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					if r.Method != http.MethodPost || body.Model != "claude-sonnet-5" || body.Stream != (mode == "stream") || r.Header.Get("Authorization") != "Bearer "+testKey {
						t.Error("probe changed its model, mode or credential")
					}
					if attempt == 1 {
						if r.URL.Path != test.path {
							t.Errorf("first request path=%s want=%s", r.URL.Path, test.path)
						}
						w.WriteHeader(test.status)
						_, _ = w.Write([]byte(test.rejection))
						return
					}
					if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Anthropic-Version") != "" || body.Input != nil || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "hi" {
						t.Errorf("fallback must send Chat Completions user messages: path=%s body=%+v", r.URL.Path, body)
					}
					if mode == "stream" {
						if r.Header.Get("Accept") != "text/event-stream" {
							t.Error("fallback lost streaming Accept header")
						}
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = w.Write([]byte("data: {\"model\":\"claude-sonnet-5\",\"choices\":[{\"delta\":{\"content\":\"pong\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
						return
					}
					_, _ = w.Write([]byte(`{"model":"claude-sonnet-5","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`))
				}))
				t.Cleanup(server.Close)
				service := onboarding.New(protocolProbeRepository{platform: test.platform}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)

				result, err := service.Probe(t.Context(), "upstream.test", "6", "claude-sonnet-5", mode)

				if err != nil || result.Status != "passed" || result.ResponseText != "pong" || result.ActualModel != "claude-sonnet-5" || calls.Load() != 2 {
					t.Fatalf("protocol fallback failed: result=%+v calls=%d err=%v", result, calls.Load(), err)
				}
			})
		}
	}
}

func TestProbeDoesNotSwitchProtocolWithoutConfirmedPreGenerationRejection(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{name: "ordinary server error", status: 500, body: `{"error":{"type":"new_api_error","message":"not implemented"}}`},
		{name: "different conversion failure", status: 500, body: `{"error":{"type":"new_api_error","code":"convert_request_failed","message":"invalid model"}}`},
		{name: "unrecognized error type", status: 500, body: `{"error":{"type":"upstream_error","code":"convert_request_failed","message":"not implemented"}}`},
		{name: "unrelated messages error", status: 400, body: `{"error":{"type":"new_api_error","message":"field messages is required for another operation"}}`},
		{name: "different validation code", status: 400, body: `{"error":{"type":"new_api_error","code":"invalid_model","message":"field messages is required"}}`},
		{name: "trailing JSON", status: 400, body: messagesRequiredResponse + " trailing"},
		{name: "authentication", status: 401, body: messagesRequiredResponse},
		{name: "rate limit", status: 429, body: messagesRequiredResponse},
		{name: "accepted generation", status: 200, body: conversionUnsupportedResponse},
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

			result, err := service.Probe(t.Context(), "upstream.test", "6", "claude-sonnet-5")

			if err == nil || result.Status != "failed" || calls.Load() != 1 {
				t.Fatalf("unconfirmed rejection was retried: result=%+v calls=%d err=%v", result, calls.Load(), err)
			}
		})
	}
}

func TestProtocolFallbackPreservesFinalFailureWithoutFurtherProtocolRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(messagesRequiredResponse))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(conversionUnsupportedResponse))
	}))
	t.Cleanup(server.Close)
	service := onboarding.New(probeRepository{}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)

	result, err := service.Probe(t.Context(), "upstream.test", "6", "claude-sonnet-5")

	if err == nil || result.Status != "failed" || result.HTTPStatus != 500 || !strings.Contains(result.Message, "convert_request_failed") || calls.Load() != 2 {
		t.Fatalf("final failure lost or retried: result=%+v calls=%d err=%v", result, calls.Load(), err)
	}
}

func TestProtocolAndStreamCompatibilityCanEachRetryOnceInEitherOrder(t *testing.T) {
	for _, order := range []string{"chat first", "stream first"} {
		t.Run(order, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := calls.Add(1)
				var body struct {
					Stream bool `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if attempt < 3 {
					w.WriteHeader(http.StatusBadRequest)
					chatRejected := (attempt == 1 && order == "chat first") || (attempt == 2 && order == "stream first")
					if chatRejected {
						_, _ = w.Write([]byte(messagesRequiredResponse))
					} else {
						_, _ = w.Write([]byte(streamRequiredResponse))
					}
					return
				}
				if r.URL.Path != "/v1/chat/completions" || !body.Stream || r.Header.Get("Accept") != "text/event-stream" {
					t.Error("combined compatibility request must use streaming Chat Completions")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
			}))
			t.Cleanup(server.Close)
			service := onboarding.New(probeRepository{}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)

			result, err := service.Probe(t.Context(), "upstream.test", "6", "claude-sonnet-5")

			if err != nil || result.Status != "passed" || result.ResponseText != "pong" || calls.Load() != 3 {
				t.Fatalf("combined compatibility failed: result=%+v calls=%d err=%v", result, calls.Load(), err)
			}
		})
	}
}
