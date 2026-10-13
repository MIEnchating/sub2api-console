package modelcheck_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOAuthCandyCompletesOnTerminalEventWhileConnectionStaysOpen(t *testing.T) {
	service := oauthService(t, func(r *http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"gpt-6-astra\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: &openStreamBody{data: []byte(body), ctx: r.Context()}}, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	report, err := service.CheckOAuthCandyWithProxy(ctx, "41", "测试账号", map[string]any{"access_token": oauthFixtureToken}, "gpt-6-astra", 120, "")
	if err != nil || report["verdict"] != "MATCH" || report["response_model"] != "gpt-6-astra" {
		t.Fatalf("completed candy result waited for EOF: %v %v", report, err)
	}
}

func TestOAuthPrecheckCompletesOnTerminalEventWithoutWaitingForConnectionClose(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"21\"}}\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: &openStreamBody{data: []byte(body), ctx: r.Context()}}, nil
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	input := request("1")
	input.Mode = "precheck"
	if _, err := f.service.EnqueueAnimation(ctx, input); err != nil {
		t.Fatal(err)
	}
	runner.runs[0](ctx)
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "succeeded" || row.Precheck == nil || row.Precheck.Verdict != "passed" {
		t.Fatalf("precheck waited for EOF: %+v", row)
	}
}

func TestOAuthCandyRejectsIncompleteOrFailedStreams(t *testing.T) {
	for _, body := range []string{
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n\ndata: [DONE]\n\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\",\"output_text\":\"21\"}}\n\n",
	} {
		service := oauthService(t, func(*http.Request) (*http.Response, error) { return oauthResponse(200, "text/event-stream", body), nil })
		report, err := service.CheckOAuthCandyWithProxy(t.Context(), "41", "测试", map[string]any{"access_token": oauthFixtureToken}, "gpt-6-astra", 120, "")
		if err != nil || report["verdict"] != "ERROR" || strings.TrimSpace(report["error"].(string)) == "" {
			t.Fatalf("partial stream accepted: %v %v", report, err)
		}
	}
}
