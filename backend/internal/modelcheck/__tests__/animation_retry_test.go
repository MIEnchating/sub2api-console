package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAnimationRetriesTransientResponseAndRecordsAttemptCount(t *testing.T) {
	calls := 0
	requestIDs := []string{}
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls++
		requestIDs = append(requestIDs, r.Header.Get("X-Request-ID"))
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"error":{"message":"temporary overload"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", fixtureSVG)
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
	if calls != 2 || row.Status != "succeeded" || row.RetryCount != 1 || row.SVG == "" {
		t.Fatalf("transient generation did not retry: calls=%d row=%#v", calls, row)
	}
	if task.Result["started_at"] == nil || task.Result["completed_at"] == nil || task.Result["duration_ms"] == nil {
		t.Fatalf("task runtime timing was not persisted: %#v", task.Result)
	}
	if len(requestIDs) != 2 || requestIDs[0] == requestIDs[1] || !strings.HasSuffix(requestIDs[1], "-retry-1") {
		t.Fatalf("retry request IDs=%#v", requestIDs)
	}
}

func TestAnimationDoesNotRetryAuthorizationFailure(t *testing.T) {
	calls := 0
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":{"message":"invalid API key"}}`)
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if calls != 1 || row.Status != "failed" || row.RetryCount != 0 || !strings.Contains(row.Error, "HTTP 401") {
		t.Fatalf("authorization failure was retried: calls=%d row=%#v", calls, row)
	}
}

func TestAnimationRetryStopsAfterThreeAttemptsAndKeepsLastReason(t *testing.T) {
	calls := 0
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"error":{"message":"gateway failure %d %s"}}`, calls, fixtureSecret)
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if calls != 3 || row.RetryCount != 2 || row.Status != "failed" || !strings.Contains(row.Error, "gateway failure 3") || strings.Contains(row.Error, fixtureSecret) {
		t.Fatalf("retry limit or safe error lost: calls=%d row=%#v", calls, row)
	}
}

func TestAnimationRetriesDisconnectedStreamWithoutReplayingPartialOutput(t *testing.T) {
	for _, platform := range []string{"openai", "anthropic"} {
		t.Run(platform, func(t *testing.T) {
			calls := 0
			var firstBody string
			f := setup(t, 1, platform, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if calls == 1 {
					firstBody = string(body)
				} else if firstBody != string(body) {
					t.Error("retry changed original conversation")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if calls == 1 {
					if platform == "openai" {
						_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"<svg>discard\"}\n\n")
					} else {
						_, _ = fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"<svg>discard\"}}\n\n")
					}
					return
				}
				if platform == "openai" {
					_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":%q}}\n\n", fixtureSVG)
				} else {
					_, _ = fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":%q}}\n\ndata: {\"type\":\"message_stop\"}\n\n", fixtureSVG)
				}
			})
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if calls != 2 || row.Status != "succeeded" || strings.Contains(row.SVG, "discard") {
				t.Fatalf("stream retry: calls=%d row=%#v", calls, row)
			}
		})
	}
}

func TestAnimationDoesNotRetryLongRetryAfterOrInvalidSVG(t *testing.T) {
	for _, mode := range []string{"retry-after", "SVG", "truncated", "quota", "quota-429"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch mode {
				case "retry-after":
					w.Header().Set("Retry-After", "3600")
					w.WriteHeader(429)
					_, _ = fmt.Fprint(w, `{"error":{"message":"wait one hour"}}`)
				case "SVG":
					_, _ = fmt.Fprint(w, `{"status":"completed","output_text":"<svg><script/></svg>"}`)
				case "truncated":
					_, _ = fmt.Fprint(w, `{"status":"incomplete","output_text":"<svg>"}`)
				case "quota":
					_, _ = fmt.Fprint(w, `{"status":"failed","error":{"code":"insufficient_quota"}}`)
				case "quota-429":
					w.WriteHeader(429)
					_, _ = fmt.Fprint(w, `{"error":{"code":"insufficient_quota","message":"quota exhausted"}}`)
				}
			})
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if calls != 1 || row.Status != "failed" || row.RetryCount != 0 {
				t.Fatalf("permanent failure retried: %d %#v", calls, row)
			}
		})
	}
}

func TestOAuthAnimationRetriesTimeoutWithSameInputAndAccount(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	calls := 0
	var firstBody string
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if calls == 1 {
			firstBody = string(body)
			return nil, context.DeadlineExceeded
		}
		if string(body) != firstBody || r.Header.Get("ChatGPT-Account-Id") != "isolated-workspace" {
			t.Error("retry changed conversation or account")
		}
		return oauthResponse(200, "application/json", fmt.Sprintf(`{"status":"completed","output_text":%q}`, fixtureSVG)), nil
	}))
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if calls != 2 || row.Status != "succeeded" || row.RetryCount != 1 {
		t.Fatalf("OAuth retry: %d %#v", calls, row)
	}
}

func TestAnimationCancellationDuringRetryDelayDoesNotSendAgain(t *testing.T) {
	started := make(chan struct{}, 1)
	calls := 0
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "15")
		w.WriteHeader(503)
		_, _ = fmt.Fprint(w, `{"error":{"message":"temporary overload"}}`)
		started <- struct{}{}
	})
	queued, err := f.service.EnqueueAnimation(context.Background(), request("1"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request not started")
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("task not cancellable")
	}
	final := finished(t, f)
	row := final.Result["animations"].([]modelcheck.AnimationResult)[0]
	if calls != 1 || final.Status != "cancelled" || row.RetryCount != 0 {
		t.Fatalf("cancelled retry sent: %d %#v", calls, final)
	}
}

func TestPrecheckDoesNotUseAnimationAutomaticRetry(t *testing.T) {
	calls := 0
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		_, _ = fmt.Fprint(w, `{"error":{"message":"temporary overload"}}`)
	})
	input := request("1")
	input.Mode, input.PrecheckQuestions = "precheck", []string{"candy"}
	if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if calls != 1 || row.RetryCount != 0 || row.Precheck == nil {
		t.Fatalf("precheck retried: %d %#v", calls, row)
	}
}

func TestCustomAnimationRetryKeepsCredentialsOnlyInMemory(t *testing.T) {
	calls := 0
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+fixtureSecret {
			t.Error("custom retry lost key")
		}
		if calls == 1 {
			w.WriteHeader(503)
			_, _ = fmt.Fprint(w, `{"error":{"message":"temporary overload"}}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"status":"completed","output_text":%q}`, fixtureSVG)
	})
	input := modelcheck.AnimationRequest{TimeoutSeconds: 5, Custom: &modelcheck.AnimationCustomEndpoint{
		BaseURL: *f.catalog.details["1"].BaseURL, APIKey: fixtureSecret, Platform: "openai", Model: "test-model",
	}}
	queued, err := f.service.EnqueueAnimation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	final := finished(t, f)
	row := final.Result["animations"].([]modelcheck.AnimationResult)[0]
	if calls != 2 || row.Status != "succeeded" || row.RetryCount != 1 || !strings.HasPrefix(row.AccountID, "custom-") {
		t.Fatalf("custom retry=%#v calls=%d", row, calls)
	}
	stored, err := f.tasks.Get(context.Background(), queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), fixtureSecret) || strings.Contains(string(raw), "api_key") {
		t.Fatal("custom retry persisted credentials")
	}
	if row.Endpoint != input.Custom.BaseURL || row.Platform != "openai" {
		t.Fatal("custom retry lost endpoint metadata")
	}
}
